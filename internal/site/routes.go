package site

import (
	"fmt"
	"net/url"
	"strings"
)

// Paths are relative to the site root. GitHub Pages serves this repo at
// BaseURL, so HTML links use Href, which adds that path prefix.
const (
	Root          = "/"
	Charts        = "/charts/"
	API           = "/api/"
	APIIndex      = "/api/v1/index.json"
	Sitemap       = "/sitemap.xml"
	BadgeRegistry = "/badge/registries.json"
	BadgeUnknown  = "/badge/not-indexed.svg"
)

// Chart is the overview page for one chart.
func Chart(repo, name string) (string, error) {
	if err := segment(repo); err != nil {
		return "", err
	}
	if err := segment(name); err != nil {
		return "", err
	}
	return "/chart/" + repo + "/" + name + "/", nil
}

// Upgrade is one from/to page.
func Upgrade(repo, name, from, to string) (string, error) {
	if err := pair(repo, name, from, to); err != nil {
		return "", err
	}
	return "/upgrade/" + repo + "/" + name + "/" + from + "/" + to + "/", nil
}

// APIUpgrade is the static JSON document for one pair.
func APIUpgrade(repo, name, from, to string) (string, error) {
	if err := pair(repo, name, from, to); err != nil {
		return "", err
	}
	return "/api/v1/upgrade/" + repo + "/" + name + "/" + from + "/" + to + ".json", nil
}

// Href is the path a browser on GitHub Pages requests.
func Href(route string) string {
	if route == "" {
		return BasePath() + "/"
	}
	return BasePath() + route
}

// Absolute is the sitemap URL for a route.
func Absolute(route string) string {
	return strings.TrimSuffix(BaseURL, "/") + route
}

// BasePath is the URL path of BaseURL, without a trailing slash.
func BasePath() string {
	u, err := url.Parse(BaseURL)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(u.Path, "/")
}

// BugURL opens a prefilled Stageproof bug report for one indexed pair.
func BugURL(repo, chart, from, to, version string) string {
	q := url.Values{}
	q.Set("template", "bug.yml")
	q.Set("title", fmt.Sprintf("Wrong index result: %s/%s %s to %s (%s)", repo, chart, from, to, version))
	q.Set("version", version)
	q.Set("what", fmt.Sprintf("Repo: %s\nChart: %s\nFrom: %s\nTo: %s\nStageproof: %s\n", repo, chart, from, to, version))
	return "https://github.com/stageproof/stageproof/issues/new?" + q.Encode()
}

func pair(repo, name, from, to string) error {
	for _, s := range []string{repo, name, from, to} {
		if err := segment(s); err != nil {
			return err
		}
	}
	return nil
}

func segment(s string) error {
	if s == "" || s == "." || s == ".." || strings.Contains(s, "..") || strings.ContainsAny(s, "/\\?#") {
		return fmt.Errorf("invalid path segment %q", s)
	}
	return nil
}
