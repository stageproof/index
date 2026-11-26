package site

import (
	"bytes"
	"embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

//go:embed templates/page.html
var templateFS embed.FS

// Disclaimer is the sentence on every upgrade page.
const Disclaimer = "Checked with chart defaults only. Your own values can change the result."

// Generate reads dataDir/v1 and replaces outDir with the static site.
func Generate(dataDir, outDir string) error {
	if outDir == "" || outDir == "." || outDir == "/" {
		return fmt.Errorf("refusing to replace %s", outDir)
	}
	abs, err := filepath.Abs(outDir)
	if err != nil {
		return err
	}
	root := filepath.Join(dataDir, "v1")
	cat, err := loadCatalog(filepath.Join(root, "index.json"))
	if err != nil {
		return err
	}
	pages, err := collect(root, cat)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(abs); err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	tpl, err := pageTemplate()
	if err != nil {
		return err
	}
	for _, page := range pages.html {
		if err := writeHTML(tpl, abs, page); err != nil {
			return err
		}
	}
	for _, file := range pages.raw {
		if err := writeRaw(abs, file.route, file.body); err != nil {
			return err
		}
	}
	return writeRaw(abs, Sitemap, pages.sitemap)
}

func pageTemplate() (*template.Template, error) {
	siteJS, err := valueAssets.ReadFile("assets/site.js")
	if err != nil {
		return nil, err
	}
	return template.New("page.html").Funcs(template.FuncMap{
		"href":       Href,
		"header":     headerHTML,
		"footer":     footerHTML,
		"disclaimer": func() string { return Disclaimer },
		"head":       func() template.HTML { return headHTML(siteJS) },
	}).ParseFS(templateFS, "templates/page.html")
}

type rawFile struct {
	route string
	body  []byte
}

type built struct {
	html    []view
	raw     []rawFile
	sitemap []byte
}

type view struct {
	Kind           string
	Title          string
	NoIndex        bool
	Root           string
	ChartsPath     string
	APIPath        string
	Notice         string
	Rows           []chartRow
	ID             string
	Latest         string
	RepoText       string
	RepoURL        string
	Homepage       string
	ReleaseNotes   string
	Error          string
	VersionLine    string
	Upgrades       []upgradeLink
	ChartName      string
	From           string
	To             string
	Minors         []minorView
	Blockers       []line
	Warnings       []line
	Info           []line
	Verdict        string
	VerdictClass   string
	MinorText      string
	Command        string
	Bug            string
	APIHref        string
	ValuesHref     string
	ChartHref      string
	IndexPath      string
	UpgradePattern string
	BadgeURL       string
	ChartCount     int
	ReportedCount  int
	PairCount      int
	BlockedCount   int
	Version        string
	Notable        []notableView
	HasLatest      bool
	LatestUpgrade  upgradeLink
	Steps          []upgradeLink
	Jumps          []upgradeLink
	ShowStepFilter bool
	NotChecked     bool
	Checks         []checkView
	Versions       []string
	Pairs          int
	Blocked        int
	Description    string
	route          string
}

type chartRow struct {
	ID           string
	Latest       string
	Pairs        int
	Blocked      int
	LatestStatus string
	LatestClass  string
	Search       string
	Href         string
}

type upgradeLink struct {
	From     string
	To       string
	Href     string
	Status   string
	Class    string
	Adjacent bool
	Reason   string
}

type notableView struct {
	ID    string
	From  string
	To    string
	Href  string
	Stars int
}

type checkView struct {
	Minor   string
	Name    string
	Status  string
	Class   string
	Summary string
}

type minorView struct {
	Minor   string
	Verdict string
	Class   string
	Exit    int
}

type line struct {
	Severity string
	RuleID   string
	Message  template.HTML
}

func collect(root string, cat catalog) (built, error) {
	var pages built
	nav := view{Root: Root, ChartsPath: Charts, APIPath: API, Version: cat.StageproofVersion}
	chartRows := make([]chartRow, 0, len(cat.Charts))
	var notable []notableView
	var pairTotal, blockedTotal, reported int
	var locs []string
	locs = append(locs, Charts)
	for _, item := range cat.Charts {
		if err := segment(item.RepoName); err != nil {
			return built{}, fmt.Errorf("%s: %w", item.ID, err)
		}
		if err := segment(item.Chart); err != nil {
			return built{}, fmt.Errorf("%s: %w", item.ID, err)
		}
		route, err := Chart(item.RepoName, item.Chart)
		if err != nil {
			return built{}, err
		}
		versions, err := readVersions(filepath.Join(root, "charts", item.RepoName, item.Chart, "meta.json"))
		if err != nil {
			return built{}, err
		}
		pairs, err := loadPairs(filepath.Join(root, "charts", item.RepoName, item.Chart), versions)
		if err != nil {
			return built{}, fmt.Errorf("%s: %w", item.ID, err)
		}
		pairTotal += item.Pairs
		blockedTotal += item.Blocked
		if item.Pairs > 0 {
			reported++
		}
		links := make([]upgradeLink, 0, len(pairs))
		for _, one := range pairs {
			up, err := Upgrade(item.RepoName, item.Chart, one.From, one.To)
			if err != nil {
				return built{}, err
			}
			api, err := APIUpgrade(item.RepoName, item.Chart, one.From, one.To)
			if err != nil {
				return built{}, err
			}
			status, class := linkStatus(one.Blocked, one.NotChecked)
			reason := ""
			if one.NotChecked {
				reason = notCheckedReason(one.Doc)
			}
			links = append(links, upgradeLink{
				From: one.From, To: one.To, Href: Href(up),
				Status: status, Class: class, Adjacent: one.Adjacent, Reason: reason,
			})
			page := upgradeView(nav, item, one, up, api)
			pages.html = append(pages.html, page)
			pages.raw = append(pages.raw, rawFile{route: api, body: one.Raw})
			badgeRoute, err := BadgeFile(item.RepoName, item.Chart, one.From, one.To)
			if err != nil {
				return built{}, err
			}
			text, kind := BadgeOf(one.Doc)
			pages.raw = append(pages.raw, rawFile{route: badgeRoute, body: RenderBadge(text, kind)})
			if one.Adjacent {
				locs = append(locs, up)
			}
		}
		page := chartView(nav, item, route, versions, links)
		pages.html = append(pages.html, page)
		if len(pairs) > 0 {
			locs = append(locs, route)
			chartAPI, err := APIChart(item.RepoName, item.Chart)
			if err != nil {
				return built{}, err
			}
			chartJSON, err := chartPairsJSON(item, pairs)
			if err != nil {
				return built{}, err
			}
			pages.raw = append(pages.raw, rawFile{route: chartAPI, body: chartJSON})
		}
		latestStatus, latestClass := "none", ""
		if page.HasLatest {
			latestStatus, latestClass = page.LatestUpgrade.Status, page.LatestUpgrade.Class
			if page.LatestUpgrade.Class == "blocked" {
				notable = append(notable, notableView{
					ID: item.ID, From: page.LatestUpgrade.From, To: page.LatestUpgrade.To,
					Href: page.LatestUpgrade.Href, Stars: item.Stars,
				})
			}
		}
		chartRows = append(chartRows, chartRow{
			ID: item.ID, Latest: item.Latest, Pairs: item.Pairs, Blocked: item.Blocked,
			LatestStatus: latestStatus, LatestClass: latestClass,
			Search: strings.ToLower(item.ID + " " + item.RepoName + " " + item.Chart + " " + item.Repo),
			Href:   Href(route),
		})
	}
	sort.Slice(notable, func(i, j int) bool {
		if notable[i].Stars != notable[j].Stars {
			return notable[i].Stars > notable[j].Stars
		}
		return notable[i].ID < notable[j].ID
	})
	if len(notable) > 5 {
		notable = notable[:5]
	}
	pages.html = append(pages.html, view{
		Kind: "home", Title: "Chart index - Stageproof", route: Root,
		Description: "Upgrade reports for popular public Helm charts. See which version jumps Stageproof blocks, then check your own values.yaml in your browser.",
		Root:        Root, ChartsPath: Charts, APIPath: API,
		ChartCount: len(cat.Charts), ReportedCount: reported, PairCount: pairTotal, BlockedCount: blockedTotal,
		Version: cat.StageproofVersion, Notable: notable,
	})
	pages.html = append(pages.html, view{
		Kind: "charts", Title: "All charts - Stageproof", Notice: cat.Notice, Rows: chartRows, route: Charts,
		Root: Root, ChartsPath: Charts, APIPath: API, Version: cat.StageproofVersion, ChartCount: len(cat.Charts),
		Description: "Every Helm chart in the Stageproof chart index, with its newest version and the result of its newest upgrade.",
	})
	pages.html = append(pages.html, view{
		Kind: "api", Title: "JSON API - Stageproof", route: API,
		Root: Root, ChartsPath: Charts, APIPath: API, Version: cat.StageproofVersion,
		Description: "Every Stageproof chart index report as a static JSON file, plus the badge files Renovate pull requests show.",
		IndexPath:   Href(APIIndex), UpgradePattern: Href("/api/v1/upgrade/<repo>/<chart>/<from>/<to>.json"),
		BadgeURL: BadgeWorkerURL,
	})
	indexPath := filepath.Join(root, "index.json")
	buf, err := os.ReadFile(indexPath)
	if err != nil {
		return built{}, err
	}
	pages.raw = append(pages.raw, rawFile{route: APIIndex, body: buf})
	registry, err := RegistryJSON(cat.Charts)
	if err != nil {
		return built{}, err
	}
	pages.raw = append(pages.raw, rawFile{route: BadgeRegistry, body: registry})
	pages.raw = append(pages.raw, rawFile{route: BadgeUnknown, body: UnknownSVG()})
	if err := addValuesPage(&pages, cat.StageproofVersion); err != nil {
		return built{}, err
	}
	locs = append(locs, Values)
	pages.sitemap, err = sitemap(locs)
	if err != nil {
		return built{}, err
	}
	return pages, nil
}

func chartView(nav view, item chart, route string, versions []string, links []upgradeLink) view {
	latest, steps, jumps, ok := splitUpgrades(links)
	return view{
		Kind: "chart", Title: item.ID + " upgrades - Stageproof", route: route, NoIndex: len(links) == 0,
		Root: nav.Root, ChartsPath: nav.ChartsPath, APIPath: nav.APIPath, Version: nav.Version,
		Description: "Stageproof upgrade reports for the " + item.ID + " Helm chart: each version jump, whether it is blocked, and why.",
		Versions:    versions, Pairs: item.Pairs, Blocked: item.Blocked,
		ID: item.ID, Latest: item.Latest, RepoText: item.Repo, RepoURL: httpLink(item.Repo),
		Homepage: httpLink(item.Homepage), ReleaseNotes: httpLink(item.ReleaseNotesURL),
		Error: item.Error, VersionLine: strings.Join(versions, ", "),
		HasLatest: ok, LatestUpgrade: latest, Steps: steps, Jumps: jumps,
		ShowStepFilter: len(steps) > 20,
	}
}

func splitUpgrades(links []upgradeLink) (upgradeLink, []upgradeLink, []upgradeLink, bool) {
	var steps, jumps []upgradeLink
	for _, link := range links {
		if link.Adjacent {
			steps = append(steps, link)
		} else {
			jumps = append(jumps, link)
		}
	}
	if len(steps) == 0 {
		return upgradeLink{}, steps, jumps, false
	}
	return steps[0], steps, jumps, true
}

func notCheckedReason(doc envelope) string {
	for _, minor := range minorKeys(doc.Results) {
		for _, check := range doc.Results[minor].Checks {
			if check.Status != "fail" {
				continue
			}
			if text := chartFailureReason(check.Summary); text != "" {
				return text
			}
		}
	}
	return ""
}

func chartFailureReason(summary string) string {
	var kept []string
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || skipFailureLine(line) {
			continue
		}
		kept = append(kept, line)
		if len(kept) == 3 {
			break
		}
	}
	text := strings.Join(kept, " ")
	if len(text) > 280 {
		text = text[:277] + "..."
	}
	return text
}

func skipFailureLine(line string) bool {
	switch {
	case strings.HasPrefix(line, "helm template "),
		strings.HasPrefix(line, "Error: execution error at"),
		strings.HasPrefix(line, "Use --debug"),
		strings.HasPrefix(line, "coalesce.go:"):
		return true
	default:
		return false
	}
}

func linkStatus(blocked, notChecked bool) (string, string) {
	switch {
	case blocked:
		return "blocked", "blocked"
	case notChecked:
		return "not checked", "unchecked"
	default:
		return "no blocking finding", "ok"
	}
}

func upgradeView(nav view, item chart, one pairFile, route, api string) view {
	var blockers, warnings, info []line
	var minors []minorView
	for _, minor := range minorKeys(one.Doc.Results) {
		rep := one.Doc.Results[minor]
		minors = append(minors, minorView{Minor: minor, Verdict: rep.Verdict, Class: verdictClass(rep.Verdict), Exit: rep.ExitCode})
		findings := append([]finding(nil), rep.Findings...)
		sort.SliceStable(findings, func(i, j int) bool {
			return rank(findings[i].Severity) < rank(findings[j].Severity)
		})
		for _, f := range findings {
			if f.Ignored || f.RuleID == "" {
				continue
			}
			row := line{Severity: f.Severity, RuleID: f.RuleID, Message: ticks(f.Message)}
			switch f.Severity {
			case "blocker":
				blockers = append(blockers, row)
			case "warning":
				warnings = append(warnings, row)
			case "info":
				info = append(info, row)
			}
		}
	}
	names := make([]string, 0, len(minors))
	for _, m := range minors {
		names = append(names, m.Minor)
	}
	verdict, class := "No blocking finding", "ok"
	switch {
	case one.Blocked:
		verdict, class = "Blocked", "blocked"
	case one.NotChecked:
		verdict, class = "Not checked", "unchecked"
	}
	return view{
		Kind: "upgrade", Title: item.Chart + " " + one.From + " to " + one.To + " - Stageproof",
		Description: "Stageproof report for upgrading the " + item.ID + " Helm chart from " + one.From + " to " + one.To + ": " + strings.ToLower(verdict) + " with chart defaults.",
		route:       route, NoIndex: !one.Adjacent,
		Root: nav.Root, ChartsPath: nav.ChartsPath, APIPath: nav.APIPath, Version: nav.Version,
		ID: item.ID, ChartHref: Href(mustChart(item)), ChartName: item.Chart, From: one.From, To: one.To, Minors: minors,
		Blockers: blockers, Warnings: warnings, Info: info, Verdict: verdict, VerdictClass: class, MinorText: strings.Join(names, ", "),
		Command: shellLines(one.Doc.ReproduceCommand), ReleaseNotes: httpLink(item.ReleaseNotesURL),
		Bug:     BugURL(item.Repo, item.Chart, one.From, one.To, one.Doc.StageproofVersion),
		APIHref: Href(api), ValuesHref: valuesLink(item.RepoName, item.Chart, one.From, one.To),
		NotChecked: one.NotChecked, Checks: checksToShow(one.Doc, one.NotChecked),
	}
}

func mustChart(item chart) string {
	route, err := Chart(item.RepoName, item.Chart)
	if err != nil {
		return Root
	}
	return route
}

func verdictClass(verdict string) string {
	switch verdict {
	case "BLOCKED":
		return "blocked"
	case "PASSED", "PROVEN":
		return "ok"
	default:
		return "unchecked"
	}
}

// ticks shows `code` spans in a finding message as code.
func ticks(msg string) template.HTML {
	parts := strings.Split(msg, "`")
	if len(parts)%2 == 0 {
		return template.HTML(template.HTMLEscapeString(msg))
	}
	var b strings.Builder
	for i, part := range parts {
		if i%2 == 1 {
			b.WriteString("<code>" + template.HTMLEscapeString(part) + "</code>")
		} else {
			b.WriteString(template.HTMLEscapeString(part))
		}
	}
	return template.HTML(b.String())
}

// shellLines puts each flag of a one-line command on its own line. A command
// with quotes is left alone, because a flag could sit inside a quoted value.
func shellLines(cmd string) string {
	if strings.ContainsAny(cmd, `'"\`) {
		return cmd
	}
	fields := strings.Fields(cmd)
	var b strings.Builder
	for i, f := range fields {
		switch {
		case i == 0:
			b.WriteString(f)
		case strings.HasPrefix(f, "-"):
			b.WriteString(" \\\n  " + f)
		default:
			b.WriteString(" " + f)
		}
	}
	return b.String()
}

func checksToShow(doc envelope, notChecked bool) []checkView {
	var failed, all []checkView
	for _, minor := range minorKeys(doc.Results) {
		for _, item := range doc.Results[minor].Checks {
			row := checkView{
				Minor: minor, Name: item.Name, Status: item.Status, Class: checkClass(item.Status),
				Summary: oneLine(item.Summary),
			}
			all = append(all, row)
			if checkFailed(item.Status) {
				failed = append(failed, row)
			}
		}
	}
	if len(failed) > 0 {
		return failed
	}
	if notChecked {
		return all
	}
	return nil
}

func checkClass(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "fail", "error":
		return "blocked"
	case "pass", "ok":
		return "ok"
	default:
		return "unchecked"
	}
}

func checkFailed(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "fail", "error":
		return true
	default:
		return false
	}
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func rank(severity string) int {
	switch severity {
	case "blocker":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

func httpLink(raw string) string {
	if strings.HasPrefix(raw, "https://") || strings.HasPrefix(raw, "http://") {
		return raw
	}
	return ""
}

func readVersions(path string) ([]string, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var doc meta
	if err := jsonUnmarshal(buf, &doc); err != nil {
		return nil, err
	}
	return doc.Versions, nil
}

func jsonUnmarshal(buf []byte, v any) error {
	return json.Unmarshal(buf, v)
}

func writeHTML(tpl *template.Template, root string, page view) error {
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "page", page); err != nil {
		return err
	}
	return writeRaw(root, page.route, buf.Bytes())
}

func writeRaw(root, route string, body []byte) error {
	rel := strings.TrimPrefix(route, "/")
	var path string
	switch {
	case strings.HasSuffix(route, ".json") || strings.HasSuffix(route, ".xml") || strings.HasSuffix(route, ".svg") || strings.HasSuffix(route, ".js") || strings.HasSuffix(route, ".css") || strings.HasSuffix(route, ".woff2") || strings.HasSuffix(route, ".txt"):
		path = filepath.Join(root, filepath.FromSlash(rel))
	case route == "/":
		path = filepath.Join(root, "index.html")
	default:
		path = filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(rel, "/")), "index.html")
	}
	if err := within(root, path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func within(root, path string) error {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes the site directory")
	}
	return nil
}

func sitemap(routes []string) ([]byte, error) {
	sort.Strings(routes)
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	buf.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	for _, route := range routes {
		buf.WriteString("  <url><loc>")
		if err := xml.EscapeText(&buf, []byte(Absolute(route))); err != nil {
			return nil, err
		}
		buf.WriteString("</loc></url>\n")
	}
	buf.WriteString("</urlset>\n")
	return buf.Bytes(), nil
}

var hrefRE = regexp.MustCompile(`(?:href|src)="([^"]*)"`)

// BrokenLinks returns internal links that do not point at a file in dir.
func BrokenLinks(dir string) ([]string, error) {
	var bad []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".html") {
			return nil
		}
		buf, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, match := range hrefRE.FindAllSubmatch(buf, -1) {
			ref := string(match[1])
			ok, err := resolves(dir, ref)
			if err != nil {
				return err
			}
			if !ok {
				rel, _ := filepath.Rel(dir, path)
				bad = append(bad, rel+": "+ref)
			}
		}
		return nil
	})
	return bad, err
}

func resolves(dir, ref string) (bool, error) {
	if ref == "" || strings.HasPrefix(ref, "#") || strings.HasPrefix(ref, "mailto:") {
		return true, nil
	}
	if strings.HasPrefix(ref, "https://") || strings.HasPrefix(ref, "http://") {
		return true, nil
	}
	path := ref
	if i := strings.IndexAny(path, "?#"); i >= 0 {
		path = path[:i]
	}
	base := BasePath()
	if path != base && path != base+"/" && !strings.HasPrefix(path, base+"/") {
		return false, nil
	}
	rel := strings.TrimPrefix(path, base)
	rel = strings.TrimPrefix(rel, "/")
	target := filepath.Join(dir, filepath.FromSlash(rel))
	if strings.HasSuffix(ref, "/") || rel == "" {
		target = filepath.Join(target, "index.html")
	}
	if err := within(dir, target); err != nil {
		return false, err
	}
	_, err := os.Stat(target)
	return err == nil, nil
}
