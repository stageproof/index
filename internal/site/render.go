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
const Disclaimer = "Checked with chart defaults only. Run the command below with your -f values.yaml to check your cluster."

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
	tpl, err := template.New("page.html").Funcs(template.FuncMap{
		"href": Href,
	}).ParseFS(templateFS, "templates/page.html")
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
	Defaults       []line
	CRDs           []line
	Images         []line
	Command        string
	Bug            string
	APIHref        string
	IndexPath      string
	UpgradePattern string
	route          string
}

type chartRow struct {
	ID      string
	Latest  string
	Blocked int
	Search  string
	Href    string
}

type upgradeLink struct {
	From     string
	To       string
	Href     string
	Blocked  bool
	Adjacent bool
}

type minorView struct {
	Minor   string
	Verdict string
	Exit    int
}

type line struct {
	Severity string
	RuleID   string
	Category string
	Message  string
}

func collect(root string, cat catalog) (built, error) {
	var pages built
	nav := view{Root: Root, ChartsPath: Charts, APIPath: API}
	pages.html = append(pages.html, view{
		Kind: "home", Title: "Stageproof index", route: Root,
		Root: Root, ChartsPath: Charts, APIPath: API,
	})
	chartRows := make([]chartRow, 0, len(cat.Charts))
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
			links = append(links, upgradeLink{
				From: one.From, To: one.To, Href: Href(up), Blocked: one.Blocked, Adjacent: one.Adjacent,
			})
			page := upgradeView(nav, item, one, up, api)
			pages.html = append(pages.html, page)
			pages.raw = append(pages.raw, rawFile{route: api, body: one.Raw})
			if one.Adjacent {
				locs = append(locs, up)
			}
		}
		page := chartView(nav, item, route, versions, links)
		pages.html = append(pages.html, page)
		if len(pairs) > 0 {
			locs = append(locs, route)
		}
		chartRows = append(chartRows, chartRow{
			ID: item.ID, Latest: item.Latest, Blocked: item.Blocked,
			Search: strings.ToLower(item.ID + " " + item.RepoName + " " + item.Chart + " " + item.Repo),
			Href:   Href(route),
		})
	}
	pages.html = append(pages.html, view{
		Kind: "charts", Title: "Charts", Notice: cat.Notice, Rows: chartRows, route: Charts,
		Root: Root, ChartsPath: Charts, APIPath: API,
	})
	pages.html = append(pages.html, view{
		Kind: "api", Title: "JSON API", route: API,
		Root: Root, ChartsPath: Charts, APIPath: API,
		IndexPath: Href(APIIndex), UpgradePattern: Href("/api/v1/upgrade/<repo>/<chart>/<from>/<to>.json"),
	})
	indexPath := filepath.Join(root, "index.json")
	buf, err := os.ReadFile(indexPath)
	if err != nil {
		return built{}, err
	}
	pages.raw = append(pages.raw, rawFile{route: APIIndex, body: buf})
	pages.sitemap, err = sitemap(locs)
	if err != nil {
		return built{}, err
	}
	return pages, nil
}

func chartView(nav view, item chart, route string, versions []string, links []upgradeLink) view {
	return view{
		Kind: "chart", Title: item.ID, route: route, NoIndex: len(links) == 0,
		Root: nav.Root, ChartsPath: nav.ChartsPath, APIPath: nav.APIPath,
		ID: item.ID, Latest: item.Latest, RepoText: item.Repo, RepoURL: httpLink(item.Repo),
		Homepage: httpLink(item.Homepage), ReleaseNotes: httpLink(item.ReleaseNotesURL),
		Error: item.Error, VersionLine: strings.Join(versions, ", "), Upgrades: links,
	}
}

func upgradeView(nav view, item chart, one pairFile, route, api string) view {
	var blockers, warnings, info, defaults, crds, images []line
	var minors []minorView
	for _, minor := range minorKeys(one.Doc.Results) {
		rep := one.Doc.Results[minor]
		minors = append(minors, minorView{Minor: minor, Verdict: rep.Verdict, Exit: rep.ExitCode})
		findings := append([]finding(nil), rep.Findings...)
		sort.SliceStable(findings, func(i, j int) bool {
			return rank(findings[i].Severity) < rank(findings[j].Severity)
		})
		for _, f := range findings {
			if f.Ignored || f.RuleID == "" {
				continue
			}
			row := line{Severity: f.Severity, RuleID: f.RuleID, Category: category(f.RuleID), Message: f.Message}
			switch f.Severity {
			case "blocker":
				blockers = append(blockers, row)
			case "warning":
				warnings = append(warnings, row)
			case "info":
				info = append(info, row)
			}
			switch row.Category {
			case "defaults":
				defaults = append(defaults, row)
			case "CRD":
				crds = append(crds, row)
			case "image":
				images = append(images, row)
			}
		}
	}
	return view{
		Kind: "upgrade", Title: item.Chart + " " + one.From + " to " + one.To,
		route: route, NoIndex: !one.Adjacent,
		Root: nav.Root, ChartsPath: nav.ChartsPath, APIPath: nav.APIPath,
		ChartName: item.Chart, From: one.From, To: one.To, Minors: minors,
		Blockers: blockers, Warnings: warnings, Info: info, Defaults: defaults, CRDs: crds, Images: images,
		Command: one.Doc.ReproduceCommand, ReleaseNotes: httpLink(item.ReleaseNotesURL),
		Bug:     BugURL(item.Repo, item.Chart, one.From, one.To, one.Doc.StageproofVersion),
		APIHref: Href(api),
	}
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

func category(rule string) string {
	switch {
	case strings.Contains(rule, "values"):
		return "defaults"
	case strings.HasPrefix(rule, "crd."):
		return "CRD"
	case strings.Contains(rule, "image"):
		return "image"
	default:
		return ""
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
	case strings.HasSuffix(route, ".json") || strings.HasSuffix(route, ".xml"):
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
