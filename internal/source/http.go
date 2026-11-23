package source

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/stageproof/index/internal/charts"
	"github.com/stageproof/index/internal/pairs"
	"gopkg.in/yaml.v3"
)

const userAgent = "stageproof-index (https://github.com/stageproof/index)"

// HTTP fetches charts from a Helm HTTP repository or an oci:// registry.
type HTTP struct {
	Client *http.Client
	// Scheme overrides the registry scheme. Production OCI registries use https.
	Scheme string
	// CacheDir stores verified chart archives. Empty disables the cache.
	CacheDir string
	hosts    hostLimit
}

func (h *HTTP) client() *http.Client {
	if h.Client != nil {
		return h.Client
	}
	return &http.Client{Timeout: 2 * time.Minute}
}

func (h *HTTP) Versions(ctx context.Context, chart charts.Chart) ([]pairs.Version, error) {
	if strings.HasPrefix(chart.Repo, "oci://") {
		return h.ociVersions(ctx, chart)
	}
	return h.helmVersions(ctx, chart)
}

func (h *HTTP) Fetch(ctx context.Context, chart charts.Chart, version string) (string, string, func() error, error) {
	if strings.HasPrefix(chart.Repo, "oci://") {
		return h.ociFetch(ctx, chart, version)
	}
	return h.helmFetch(ctx, chart, version)
}

func (h *HTTP) helmVersions(ctx context.Context, chart charts.Chart) ([]pairs.Version, error) {
	raw, err := h.get(ctx, strings.TrimRight(chart.Repo, "/")+"/index.yaml", "")
	if err != nil {
		return nil, err
	}
	var idx struct {
		Entries map[string][]struct {
			Version string    `yaml:"version"`
			Created time.Time `yaml:"created"`
		} `yaml:"entries"`
	}
	if err := yaml.Unmarshal(raw, &idx); err != nil {
		return nil, fmt.Errorf("parse index %s: %w", chart.Repo, err)
	}
	entries, ok := idx.Entries[chart.Chart]
	if !ok || len(entries) == 0 {
		return nil, fmt.Errorf("chart %s is not in %s", chart.Chart, chart.Repo)
	}
	out := make([]pairs.Version, 0, len(entries))
	for _, e := range entries {
		out = append(out, pairs.Version{Name: e.Version, Created: e.Created})
	}
	return out, nil
}

func (h *HTTP) helmFetch(ctx context.Context, chart charts.Chart, version string) (string, string, func() error, error) {
	raw, err := h.get(ctx, strings.TrimRight(chart.Repo, "/")+"/index.yaml", "")
	if err != nil {
		return "", "", nil, err
	}
	var idx struct {
		Entries map[string][]struct {
			Version string   `yaml:"version"`
			Digest  string   `yaml:"digest"`
			URLs    []string `yaml:"urls"`
		} `yaml:"entries"`
	}
	if err := yaml.Unmarshal(raw, &idx); err != nil {
		return "", "", nil, err
	}
	var digest, chartURL string
	for _, e := range idx.Entries[chart.Chart] {
		if e.Version == version && len(e.URLs) > 0 {
			digest = e.Digest
			if strings.HasPrefix(e.URLs[0], "oci://") {
				chartURL = e.URLs[0]
			} else {
				chartURL, err = resolveChartURL(chart.Repo, e.URLs[0])
				if err != nil {
					return "", "", nil, err
				}
			}
			break
		}
	}
	if chartURL == "" {
		return "", "", nil, fmt.Errorf("chart %s version %s is not in the index", chart.Chart, version)
	}
	want := strings.TrimPrefix(strings.ToLower(digest), "sha256:")
	var body []byte
	if cached, ok := h.readCache(want); ok {
		body = cached
	} else if strings.HasPrefix(chartURL, "oci://") {
		var err error
		body, err = h.pullOCIReference(ctx, chartURL)
		if err != nil {
			return "", "", nil, err
		}
	} else {
		var err error
		body, err = h.get(ctx, chartURL, "")
		if err != nil {
			return "", "", nil, err
		}
	}
	sum := sha256.Sum256(body)
	got := hex.EncodeToString(sum[:])
	if len(want) == 64 && want != got {
		return "", "", nil, fmt.Errorf("digest mismatch for %s@%s", chart.Chart, version)
	}
	if err := h.writeCache(got, body); err != nil {
		return "", "", nil, err
	}
	dir, err := os.MkdirTemp("", "stageproof-index-")
	if err != nil {
		return "", "", nil, err
	}
	chartDir, err := untarChart(body, dir)
	if err != nil {
		return "", "", nil, errors.Join(err, os.RemoveAll(dir))
	}
	return chartDir, got, func() error { return os.RemoveAll(dir) }, nil
}

func (h *HTTP) readCache(sum string) ([]byte, bool) {
	if h.CacheDir == "" || !isHexSHA256(sum) {
		return nil, false
	}
	buf, err := os.ReadFile(filepath.Join(h.CacheDir, sum+".tgz"))
	if err != nil {
		return nil, false
	}
	got := sha256.Sum256(buf)
	if hex.EncodeToString(got[:]) != sum {
		return nil, false
	}
	return buf, true
}

func (h *HTTP) writeCache(sum string, body []byte) error {
	if h.CacheDir == "" || !isHexSHA256(sum) {
		return nil
	}
	if err := os.MkdirAll(h.CacheDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(h.CacheDir, sum+".tgz")
	tmp := path + ".partial"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func isHexSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

func resolveChartURL(repo, raw string) (string, error) {
	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") {
		return raw, nil
	}
	base, err := url.Parse(strings.TrimRight(repo, "/") + "/")
	if err != nil {
		return "", err
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	resolved := base.ResolveReference(ref)
	if resolved.Scheme != "https" && resolved.Scheme != "http" {
		return "", fmt.Errorf("chart url %q has unsupported scheme %q", raw, resolved.Scheme)
	}
	return resolved.String(), nil
}

const (
	maxUntarFileBytes  = 64 << 20
	maxUntarTotalBytes = 256 << 20
	maxUntarEntries    = 10000
)

func untarChart(body []byte, dest string) (string, error) {
	var reader io.Reader = bytes.NewReader(body)
	if len(body) >= 2 && body[0] == 0x1f && body[1] == 0x8b {
		gz, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		defer func() { _ = gz.Close() }()
		reader = gz
	}
	tr := tar.NewReader(reader)
	var totalBytes int64
	var entries int
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		entries++
		if entries > maxUntarEntries {
			return "", fmt.Errorf("chart archive has more than %d entries", maxUntarEntries)
		}
		name := pathClean(hdr.Name)
		if name == "" || strings.HasPrefix(name, "../") || name == ".." {
			return "", fmt.Errorf("chart archive path %q escapes the destination", hdr.Name)
		}
		target := filepath.Join(dest, name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if st, err := os.Stat(target); err == nil && !st.IsDir() {
				if err := os.Remove(target); err != nil {
					return "", err
				}
			}
			if err := os.MkdirAll(target, 0o755); err != nil {
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			if st, err := os.Stat(target); err == nil && st.IsDir() {
				if err := os.RemoveAll(target); err != nil {
					return "", err
				}
			}
			if hdr.Size < 0 || hdr.Size > maxUntarFileBytes {
				return "", fmt.Errorf("chart file %s is larger than 64MiB", hdr.Name)
			}
			if totalBytes+hdr.Size > maxUntarTotalBytes {
				return "", fmt.Errorf("chart archive exceeds 256MiB uncompressed")
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				return "", err
			}
			written, err := io.Copy(f, io.LimitReader(tr, hdr.Size))
			if err != nil {
				return "", errors.Join(err, f.Close())
			}
			if err := f.Close(); err != nil {
				return "", err
			}
			totalBytes += written
		default:
			return "", fmt.Errorf("chart archive entry %s has unsupported type %d", hdr.Name, hdr.Typeflag)
		}
	}
	if st, err := os.Stat(filepath.Join(dest, "Chart.yaml")); err == nil && !st.IsDir() {
		return dest, nil
	}
	children, err := os.ReadDir(dest)
	if err != nil {
		return "", err
	}
	if len(children) == 1 && children[0].IsDir() {
		nested := filepath.Join(dest, children[0].Name())
		if st, err := os.Stat(filepath.Join(nested, "Chart.yaml")); err == nil && !st.IsDir() {
			return nested, nil
		}
	}
	return "", fmt.Errorf("chart archive has no Chart.yaml")
}

func pathClean(name string) string {
	name = strings.TrimPrefix(name, "./")
	name = filepath.ToSlash(name)
	return pathCleanSlash(name)
}

func pathCleanSlash(name string) string {
	parts := strings.Split(name, "/")
	var keep []string
	for _, p := range parts {
		if p == "" || p == "." {
			continue
		}
		if p == ".." {
			return ""
		}
		keep = append(keep, p)
	}
	return strings.Join(keep, "/")
}

type hostLimit struct {
	mu sync.Mutex
	ch map[string]chan struct{}
}

func (h *hostLimit) acquire(host string) func() {
	h.mu.Lock()
	if h.ch == nil {
		h.ch = map[string]chan struct{}{}
	}
	c, ok := h.ch[host]
	if !ok {
		c = make(chan struct{}, 4)
		h.ch[host] = c
	}
	h.mu.Unlock()
	c <- struct{}{}
	return func() { <-c }
}

func (h *HTTP) get(ctx context.Context, raw, accept string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	release := h.hosts.acquire(u.Host)
	defer release()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	var last error
	wait := time.Second
	for attempt := 0; attempt < 4; attempt++ {
		resp, err := h.client().Do(req)
		if err != nil {
			last = err
			time.Sleep(wait)
			wait *= 2
			continue
		}
		buf, readErr := io.ReadAll(io.LimitReader(resp.Body, 128<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			last = fmt.Errorf("%s: %s", raw, resp.Status)
			time.Sleep(wait)
			wait *= 2
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: %s", raw, resp.Status)
		}
		return buf, nil
	}
	return nil, last
}

var bearerPattern = regexp.MustCompile(`(\w+)="([^"]*)"`)

func (h *HTTP) ociVersions(ctx context.Context, chart charts.Chart) ([]pairs.Version, error) {
	base, name, err := splitOCI(chart.Repo, chart.Chart, h.scheme())
	if err != nil {
		return nil, err
	}
	raw, err := h.registryGet(ctx, base+"/v2/"+name+"/tags/list", "application/json")
	if err != nil {
		return nil, err
	}
	var body struct {
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	out := make([]pairs.Version, 0, len(body.Tags))
	for _, tag := range body.Tags {
		out = append(out, pairs.Version{Name: tag})
	}
	return out, nil
}

func (h *HTTP) ociFetch(ctx context.Context, chart charts.Chart, version string) (string, string, func() error, error) {
	base, name, err := splitOCI(chart.Repo, chart.Chart, h.scheme())
	if err != nil {
		return "", "", nil, err
	}
	body, sum, err := h.fetchOCI(ctx, base, name, version)
	if err != nil {
		return "", "", nil, err
	}
	dir, err := os.MkdirTemp("", "stageproof-index-")
	if err != nil {
		return "", "", nil, err
	}
	chartDir, err := untarChart(body, dir)
	if err != nil {
		return "", "", nil, errors.Join(err, os.RemoveAll(dir))
	}
	return chartDir, sum, func() error { return os.RemoveAll(dir) }, nil
}

const helmChartLayer = "application/vnd.cncf.helm.chart.content.v1.tar+gzip"

type ociLayer struct {
	Digest    string `json:"digest"`
	MediaType string `json:"mediaType"`
}

func chartLayerDigest(layers []ociLayer) (string, error) {
	for _, layer := range layers {
		if layer.MediaType == helmChartLayer && layer.Digest != "" {
			return layer.Digest, nil
		}
	}
	return "", fmt.Errorf("no Helm chart layer")
}

func (h *HTTP) fetchOCI(ctx context.Context, base, name, version string) ([]byte, string, error) {
	manifest, err := h.registryGet(ctx, base+"/v2/"+name+"/manifests/"+url.PathEscape(version), "application/vnd.oci.image.manifest.v1+json")
	if err != nil {
		return nil, "", err
	}
	var doc struct {
		Layers []ociLayer `json:"layers"`
	}
	if err := json.Unmarshal(manifest, &doc); err != nil {
		return nil, "", err
	}
	digest, err := chartLayerDigest(doc.Layers)
	if err != nil {
		return nil, "", fmt.Errorf("oci manifest for %s@%s: %w", name, version, err)
	}
	body, err := h.chartBlob(ctx, base, name, digest)
	if err != nil {
		return nil, "", err
	}
	sum := sha256.Sum256(body)
	return body, hex.EncodeToString(sum[:]), nil
}

func (h *HTTP) chartBlob(ctx context.Context, base, name, digest string) ([]byte, error) {
	want := strings.TrimPrefix(strings.ToLower(digest), "sha256:")
	if !strings.HasPrefix(strings.ToLower(digest), "sha256:") || !isHexSHA256(want) {
		return nil, fmt.Errorf("invalid layer digest %q", digest)
	}
	if body, ok := h.readCache(want); ok {
		return body, nil
	}
	body, err := h.registryGet(ctx, base+"/v2/"+name+"/blobs/sha256:"+want, "application/octet-stream")
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != want {
		return nil, fmt.Errorf("digest mismatch for %s", digest)
	}
	if err := h.writeCache(want, body); err != nil {
		return nil, err
	}
	return body, nil
}

func (h *HTTP) pullOCIReference(ctx context.Context, ref string) ([]byte, error) {
	base, name, tag, err := parseOCIRef(ref, h.scheme())
	if err != nil {
		return nil, err
	}
	body, _, err := h.fetchOCI(ctx, base, name, tag)
	return body, err
}

func parseOCIRef(ref, scheme string) (base, name, tag string, err error) {
	rest := strings.TrimPrefix(ref, "oci://")
	host, path, ok := strings.Cut(rest, "/")
	if !ok || host == "" || path == "" {
		return "", "", "", fmt.Errorf("oci reference %q needs a host and a repository", ref)
	}
	if i := strings.LastIndex(path, ":"); i >= 0 {
		tag = path[i+1:]
		path = path[:i]
	}
	if tag == "" || path == "" {
		return "", "", "", fmt.Errorf("oci reference %q has no tag", ref)
	}
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + host, path, tag, nil
}

func (h *HTTP) scheme() string {
	if h.Scheme != "" {
		return h.Scheme
	}
	return "https"
}

func splitOCI(repo, chart, scheme string) (base, name string, err error) {
	rest := strings.TrimPrefix(repo, "oci://")
	host, path, ok := strings.Cut(rest, "/")
	if !ok || host == "" {
		return "", "", fmt.Errorf("oci repo %q needs a host and a repository", repo)
	}
	name = strings.Trim(path, "/")
	if chart != "" && !strings.HasSuffix(name, "/"+chart) && name != chart {
		name = name + "/" + chart
	}
	if scheme == "" {
		scheme = "https"
	}
	return scheme + "://" + host, name, nil
}

func (h *HTTP) registryGet(ctx context.Context, raw, accept string) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	release := h.hosts.acquire(u.Host)
	defer release()
	var last error
	wait := time.Second
	var token string
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", accept)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := h.client().Do(req)
		if err != nil {
			last = err
			time.Sleep(wait)
			wait *= 2
			continue
		}
		if resp.StatusCode == http.StatusUnauthorized && token == "" {
			challenge := resp.Header.Get("Www-Authenticate")
			_ = resp.Body.Close()
			token, err = h.bearer(ctx, challenge)
			if err != nil {
				return nil, err
			}
			continue
		}
		buf, readErr := io.ReadAll(io.LimitReader(resp.Body, 128<<20))
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			last = fmt.Errorf("%s: %s", raw, resp.Status)
			time.Sleep(wait)
			wait *= 2
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("%s: %s", raw, resp.Status)
		}
		return buf, nil
	}
	return nil, last
}

func (h *HTTP) bearer(ctx context.Context, challenge string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return "", fmt.Errorf("registry challenge %q is not bearer", challenge)
	}
	fields := map[string]string{}
	for _, m := range bearerPattern.FindAllStringSubmatch(challenge, -1) {
		fields[m[1]] = m[2]
	}
	if fields["realm"] == "" {
		return "", fmt.Errorf("registry challenge has no realm")
	}
	u, err := url.Parse(fields["realm"])
	if err != nil {
		return "", err
	}
	if u.Scheme != "https" && u.Scheme != h.scheme() {
		return "", fmt.Errorf("registry token realm %q must use %s", fields["realm"], h.scheme())
	}
	q := u.Query()
	if fields["service"] != "" {
		q.Set("service", fields["service"])
	}
	if fields["scope"] != "" {
		q.Set("scope", fields["scope"])
	}
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := h.client().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token endpoint %s: %s", fields["realm"], resp.Status)
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return "", err
	}
	if body.Token != "" {
		return body.Token, nil
	}
	if body.AccessToken != "" {
		return body.AccessToken, nil
	}
	return "", fmt.Errorf("token response from %s has no token", fields["realm"])
}
