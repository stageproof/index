package site

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"unicode"
)

// BadgeFile is the static SVG for one indexed pair.
func BadgeFile(repo, chart, from, to string) (string, error) {
	if err := pair(repo, chart, from, to); err != nil {
		return "", err
	}
	return "/badge/" + repo + "/" + chart + "/" + from + "/" + to + ".svg", nil
}

// BadgeQuery is the worker request. chart is the Helm chart name.
// registry is the repository URL, which Renovate can pass through prBodyDefinitions.
// Query keys are encoded in sorted order.
func BadgeQuery(registry, chart, from, to string) (string, error) {
	if err := segment(chart); err != nil {
		return "", err
	}
	if err := segment(from); err != nil {
		return "", err
	}
	if err := segment(to); err != nil {
		return "", err
	}
	if strings.TrimSpace(registry) == "" {
		return "", fmt.Errorf("registry is empty")
	}
	q := url.Values{}
	q.Set("chart", chart)
	q.Set("from", from)
	q.Set("registry", registry)
	q.Set("to", to)
	return "/badge.svg?" + q.Encode(), nil
}

// NormRegistry trims space and trailing slashes so a Chart.yaml URL matches the catalog URL.
func NormRegistry(registry string) string {
	registry = strings.TrimSpace(registry)
	for strings.HasSuffix(registry, "/") {
		registry = strings.TrimSuffix(registry, "/")
	}
	return registry
}

// RegistryIndex maps a Helm repository URL and chart name to repo_name.
func RegistryIndex(charts []chart) (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	for _, item := range charts {
		key := NormRegistry(item.Repo)
		if key == "" || item.Chart == "" || item.RepoName == "" {
			continue
		}
		if err := segment(item.RepoName); err != nil {
			return nil, fmt.Errorf("registry %s: %w", key, err)
		}
		if err := segment(item.Chart); err != nil {
			return nil, fmt.Errorf("registry %s: %w", key, err)
		}
		if out[key] == nil {
			out[key] = map[string]string{}
		}
		if prev, ok := out[key][item.Chart]; ok && prev != item.RepoName {
			return nil, fmt.Errorf("registry %s chart %s maps to %s and %s", key, item.Chart, prev, item.RepoName)
		}
		out[key][item.Chart] = item.RepoName
	}
	return out, nil
}

// RegistryJSON is the file the worker reads.
func RegistryJSON(charts []chart) ([]byte, error) {
	index, err := RegistryIndex(charts)
	if err != nil {
		return nil, err
	}
	buf, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(buf, '\n'), nil
}

// LookupRepo resolves a repository URL and chart name to repo_name.
// The worker uses the same rules: trim trailing slashes, then require a path-safe name.
func LookupRepo(index map[string]map[string]string, registry, chart string) (string, bool) {
	if err := segment(chart); err != nil {
		return "", false
	}
	charts := index[NormRegistry(registry)]
	repo := charts[chart]
	if err := segment(repo); err != nil {
		return "", false
	}
	return repo, true
}

// BadgeOf is the right-hand text and the color kind for one report.
// A blocked report names the first blocking finding. The words safe, verified,
// PROVEN, and 100% pass are never used.
func BadgeOf(doc envelope) (string, string) {
	blocked, notChecked := pairVerdict(doc)
	if blocked {
		text := blockerText(blockersOf(doc))
		if text == "" || forbiddenBadge(text) {
			text = "blocking finding"
		}
		return text, "blocked"
	}
	if notChecked {
		return "not checked", "unchecked"
	}
	return "no blocking finding (defaults)", "clear"
}

// UnknownSVG is the HTTP 200 image for a pair the catalog does not have.
func UnknownSVG() []byte {
	return RenderBadge("not indexed", "unknown")
}

// RenderBadge draws one flat SVG. Width is 7px per character plus 12px of padding on each side.
func RenderBadge(text, kind string) []byte {
	left := sideWidth("stageproof")
	right := sideWidth(text)
	label := xmlEscape("stageproof: " + text)
	rightX := left + right/2
	body := fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\" width=\"%d\" height=\"20\" role=\"img\" aria-label=\"%s\">\n"+
		"<title>%s</title>\n"+
		"<rect width=\"%d\" height=\"20\" fill=\"#555\"/>\n"+
		"<rect x=\"%d\" width=\"%d\" height=\"20\" fill=\"%s\"/>\n"+
		"<g fill=\"#fff\" text-anchor=\"middle\" font-family=\"Verdana,DejaVu Sans,sans-serif\" font-size=\"11\">\n"+
		"<text x=\"%d\" y=\"14\">stageproof</text>\n"+
		"<text x=\"%d\" y=\"14\">%s</text>\n"+
		"</g>\n"+
		"</svg>\n",
		left+right, label,
		label,
		left,
		left, right, badgeColor(kind),
		left/2,
		rightX, xmlEscape(text),
	)
	return []byte(body)
}

func sideWidth(text string) int {
	return len([]rune(text))*7 + 12
}

func badgeColor(kind string) string {
	switch kind {
	case "blocked":
		return "#c92a2a"
	case "clear":
		return "#1f6feb"
	default:
		return "#6c757d"
	}
}

func blockersOf(doc envelope) []finding {
	var out []finding
	for _, minor := range minorKeys(doc.Results) {
		for _, item := range doc.Results[minor].Findings {
			if item.Ignored || item.Severity != "blocker" || item.RuleID == "" {
				continue
			}
			out = append(out, item)
		}
	}
	return out
}

func blockerText(list []finding) string {
	if len(list) == 0 {
		return ""
	}
	first := list[0]
	switch {
	case strings.HasPrefix(first.RuleID, "k8s.api."):
		return "removed K8s API"
	case strings.HasPrefix(first.RuleID, "crd."):
		return "CRD schema break"
	case valueRemoval(first):
		n := 0
		for _, item := range list {
			if valueRemoval(item) {
				n++
			}
		}
		return fmt.Sprintf("%d value keys removed", n)
	default:
		text := clipBadge(first.Message)
		if text == "" {
			text = strings.ReplaceAll(first.RuleID, ".", " ")
		}
		return text
	}
}

func valueRemoval(item finding) bool {
	if !strings.Contains(item.RuleID, "values") {
		return false
	}
	id := strings.ToLower(item.RuleID)
	msg := strings.ToLower(item.Message)
	return strings.Contains(id, "drop") || strings.Contains(id, "remov") || strings.Contains(msg, "removed") || strings.Contains(msg, "dropped")
}

func clipBadge(message string) string {
	message = oneLine(message)
	if i := strings.Index(message, ". "); i >= 0 && i <= 60 {
		message = message[:i]
	}
	runes := []rune(message)
	if len(runes) > 60 {
		message = strings.TrimSpace(string(runes[:60]))
	}
	return message
}

func forbiddenBadge(text string) bool {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "100%") {
		return true
	}
	for _, word := range []string{"safe", "verified", "proven"} {
		if containsWord(lower, word) {
			return true
		}
	}
	return false
}

func containsWord(text, word string) bool {
	rest := text
	for {
		i := strings.Index(rest, word)
		if i < 0 {
			return false
		}
		before := i == 0 || !unicode.IsLetter(rune(rest[i-1]))
		end := i + len(word)
		after := end == len(rest) || !unicode.IsLetter(rune(rest[end]))
		if before && after {
			return true
		}
		rest = rest[end:]
	}
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}
