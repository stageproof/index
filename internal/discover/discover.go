// Package discover reads the public Helm catalog on Artifact Hub.
package discover

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stageproof/index/internal/charts"
)

const (
	defaultBase = "https://artifacthub.io"
	userAgent   = "stageproof-index (https://github.com/stageproof/index)"
	pageSize    = 60
)

// Options controls one discovery run.
type Options struct {
	BaseURL string
	Client  *http.Client
	Limit   int
}

// Run returns the top charts by star count after the catalog filters.
func Run(opt Options) (charts.File, error) {
	if opt.Limit <= 0 {
		opt.Limit = 300
	}
	if opt.BaseURL == "" {
		opt.BaseURL = defaultBase
	}
	client := opt.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second, Transport: pace(http.DefaultTransport)}
	}
	var kept []charts.Chart
	seen := map[string]struct{}{}
	for offset := 0; len(kept) < opt.Limit; offset += pageSize {
		page, err := search(client, opt.BaseURL, offset)
		if err != nil {
			return charts.File{}, err
		}
		if len(page) == 0 {
			break
		}
		fresh := 0
		for _, item := range page {
			key := item.Repository.URL + "\x00" + item.Name
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			fresh++
			chart, apiRepo, apiChart, ok := item.chart()
			if !ok {
				continue
			}
			detail, ok, err := fetchDetail(client, opt.BaseURL, apiRepo, apiChart)
			if err != nil {
				return charts.File{}, err
			}
			if !ok {
				continue
			}
			chart.Homepage = detail.HomeURL
			chart.ReleaseNotesURL = releaseNotes(detail.Links)
			if err := chart.Validate(); err != nil {
				continue
			}
			kept = append(kept, chart)
			if len(kept) == opt.Limit {
				break
			}
		}
		if fresh == 0 || len(page) < pageSize {
			break
		}
	}
	sort.Slice(kept, func(i, j int) bool {
		if kept[i].Stars != kept[j].Stars {
			return kept[i].Stars > kept[j].Stars
		}
		return kept[i].ID < kept[j].ID
	})
	return charts.File{Charts: kept}, nil
}

type searchItem struct {
	Name       string `json:"name"`
	Deprecated bool   `json:"deprecated"`
	Stars      int    `json:"stars"`
	Repository struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"repository"`
}

func (s searchItem) chart() (charts.Chart, string, string, bool) {
	if s.Deprecated || s.Name == "" || s.Repository.URL == "" || s.Repository.Name == "" {
		return charts.Chart{}, "", "", false
	}
	if archivedBitnamiMirror(s.Repository.Name, s.Repository.URL) {
		return charts.Chart{}, "", "", false
	}
	if !safeSegment(s.Repository.Name) || !safeSegment(s.Name) {
		return charts.Chart{}, "", "", false
	}
	return charts.Chart{
		ID:       s.Repository.Name + "/" + s.Name,
		Repo:     s.Repository.URL,
		RepoName: s.Repository.Name,
		Chart:    s.Name,
		Stars:    s.Stars,
	}, s.Repository.Name, s.Name, true
}

type detail struct {
	HomeURL string `json:"home_url"`
	Links   []link `json:"links"`
}

type link struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

func releaseNotes(links []link) string {
	sort.Slice(links, func(i, j int) bool { return links[i].Name < links[j].Name })
	for _, l := range links {
		if strings.Contains(strings.ToLower(l.Name), "release") && l.URL != "" {
			return l.URL
		}
	}
	return ""
}

func archivedBitnamiMirror(name, repoURL string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	u := strings.ToLower(repoURL)
	switch n {
	case "bitnami", "bitnamicharts":
		return true
	}
	return strings.Contains(u, "charts.bitnami.com") || strings.Contains(u, "/bitnamicharts")
}

func safeSegment(s string) bool {
	if s == "" || s == "." || s == ".." || strings.Contains(s, "..") {
		return false
	}
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

func search(client *http.Client, base string, offset int) ([]searchItem, error) {
	u, err := url.Parse(strings.TrimRight(base, "/") + "/api/v1/packages/search")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("kind", "0")
	q.Set("sort", "stars")
	q.Set("limit", strconv.Itoa(pageSize))
	q.Set("offset", strconv.Itoa(offset))
	u.RawQuery = q.Encode()
	var body struct {
		Packages []searchItem `json:"packages"`
	}
	if _, err := getJSON(client, u.String(), &body); err != nil {
		return nil, err
	}
	return body.Packages, nil
}

func fetchDetail(client *http.Client, base, repo, chart string) (detail, bool, error) {
	u := strings.TrimRight(base, "/") + "/api/v1/packages/helm/" + url.PathEscape(repo) + "/" + url.PathEscape(chart)
	var out detail
	status, err := getJSON(client, u, &out)
	if err != nil {
		return detail{}, false, err
	}
	if status == http.StatusNotFound {
		return detail{}, false, nil
	}
	return out, true, nil
}

func getJSON(client *http.Client, raw string, dest any) (int, error) {
	var last error
	wait := 5 * time.Second
	for attempt := 0; attempt < 6; attempt++ {
		req, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			return 0, err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			last = err
			time.Sleep(wait)
			wait *= 2
			continue
		}
		buf, readErr := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return 0, readErr
		}
		if resp.StatusCode == http.StatusNotFound {
			return resp.StatusCode, nil
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			last = fmt.Errorf("%s: %s", raw, resp.Status)
			pause := wait
			if retry := resp.Header.Get("Retry-After"); retry != "" {
				if seconds, convErr := strconv.Atoi(retry); convErr == nil && seconds > 0 {
					pause = time.Duration(seconds) * time.Second
				}
			}
			if pause > time.Minute {
				pause = time.Minute
			}
			time.Sleep(pause)
			if wait < 30*time.Second {
				wait *= 2
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return resp.StatusCode, fmt.Errorf("%s: %s", raw, resp.Status)
		}
		if err := json.Unmarshal(buf, dest); err != nil {
			return resp.StatusCode, fmt.Errorf("parse %s: %w", raw, err)
		}
		return resp.StatusCode, nil
	}
	return 0, last
}

func pace(next http.RoundTripper) http.RoundTripper {
	if next == nil {
		next = http.DefaultTransport
	}
	return &pacer{next: next, gap: time.Second}
}

type pacer struct {
	next http.RoundTripper
	gap  time.Duration
	last time.Time
}

func (p *pacer) RoundTrip(req *http.Request) (*http.Response, error) {
	if wait := p.gap - time.Since(p.last); wait > 0 {
		time.Sleep(wait)
	}
	p.last = time.Now()
	return p.next.RoundTrip(req)
}
