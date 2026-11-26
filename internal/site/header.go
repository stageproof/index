package site

import (
	"crypto/sha512"
	"encoding/base64"
	"html/template"
	"strings"
)

// ProductURL is the Stageproof product site. Every index page uses its
// header and footer, so the two read as one site.
const ProductURL = "https://stageproof.github.io/stageproof/"

// NavLink is one link in the header or the footer.
type NavLink struct {
	Label string
	Href  string
}

// FooterGroup is one footer column.
type FooterGroup struct {
	Title string
	Links []NavLink
}

const (
	chartsLabel = "Charts"
	valuesLabel = "Check values"
)

// ProductNav is the header of the product site, in its order.
// web/*.html in stageproof/stageproof pins the same labels and pages.
func ProductNav() []NavLink {
	return []NavLink{
		{Label: "Overview", Href: ProductURL},
		{Label: "Docs", Href: ProductURL + "docs.html"},
		{Label: chartsLabel, Href: Href(Root)},
		{Label: valuesLabel, Href: Href(Values)},
		{Label: "Security", Href: ProductURL + "security.html"},
		{Label: "Roadmap", Href: ProductURL + "roadmap.html"},
	}
}

// FooterNav is the footer of the product site, in its order.
func FooterNav() []FooterGroup {
	return []FooterGroup{
		{Title: "Product", Links: []NavLink{
			{Label: "Quickstart", Href: ProductURL + "docs.html#first-check"},
			{Label: "Chart index", Href: Href(Root)},
			{Label: "Check your values", Href: Href(Values)},
			{Label: "Roadmap", Href: ProductURL + "roadmap.html"},
		}},
		{Title: "Docs", Links: []NavLink{
			{Label: "Documentation", Href: ProductURL + "docs.html"},
			{Label: "Architecture", Href: ProductURL + "architecture.html"},
			{Label: "Security", Href: ProductURL + "security.html"},
			{Label: "JSON API", Href: Href(API)},
		}},
		{Title: "Code", Links: []NavLink{
			{Label: "GitHub", Href: "https://github.com/stageproof/stageproof"},
			{Label: "GitHub Action", Href: "https://github.com/stageproof/action"},
			{Label: "GitLab component", Href: "https://gitlab.com/zarovvvv/stageproof"},
			{Label: "Renovate preset", Href: "https://github.com/stageproof/renovate-presets"},
		}},
	}
}

// activeLabel names the header entry for a page kind.
func activeLabel(kind string) string {
	switch kind {
	case "home", "charts", "chart", "upgrade":
		return chartsLabel
	case "values":
		return valuesLabel
	}
	return ""
}

const brandMark = `<svg class="brand-mark" viewBox="0 0 32 32" fill="none" aria-hidden="true">
<rect x="2" y="2" width="28" height="28" rx="7" fill="#0B0F17" stroke="#38BDF8" stroke-width="1.8"/>
<path d="M8.5 11.5h15M8.5 16h9" stroke="#64748B" stroke-width="1.8" stroke-linecap="round"/>
<path d="M18 21.5l2.6 2.6 4.9-5.3" stroke="#2DD4BF" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`

const themeToggle = `<button class="theme-toggle" id="theme-toggle" type="button" aria-pressed="false" aria-label="Switch to light theme">
<span class="theme-toggle-thumb" aria-hidden="true">
<svg class="theme-icon-moon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 14.5A8.5 8.5 0 1 1 9.5 3 7 7 0 0 0 21 14.5z"/></svg>
<svg class="theme-icon-sun" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4"/></svg>
</span>
</button>`

func brandLink() string {
	return `<a class="brand" href="` + template.HTMLEscapeString(ProductURL) + `">` + "\n" + brandMark + "\n" +
		`<span class="brand-word">stageproof</span>` + "\n</a>\n"
}

// headerHTML is the header every index page shows.
func headerHTML(kind string) template.HTML {
	esc := template.HTMLEscapeString
	var b strings.Builder
	b.WriteString(`<a class="skip" href="#main">Skip to content</a>` + "\n")
	b.WriteString(`<header class="nav">` + "\n" + `<div class="wrap nav-inner">` + "\n")
	b.WriteString(brandLink())
	b.WriteString(`<nav class="nav-links" id="nav-links" aria-label="Primary">` + "\n")
	active := activeLabel(kind)
	for _, link := range ProductNav() {
		b.WriteString(`<a href="` + esc(link.Href) + `"`)
		if link.Label == active {
			b.WriteString(` class="active" aria-current="page"`)
		}
		b.WriteString(`>` + esc(link.Label) + "</a>\n")
	}
	b.WriteString("</nav>\n" + `<div class="nav-cta">` + "\n" + themeToggle + "\n")
	b.WriteString(`<a class="btn btn-primary btn-sm" href="` + esc(ProductURL+"docs.html#first-check") + `">Quickstart</a>` + "\n</div>\n")
	b.WriteString(`<button class="nav-toggle" id="nav-toggle" type="button" aria-expanded="false" aria-controls="nav-links" aria-label="Toggle navigation">Menu</button>` + "\n")
	b.WriteString("</div>\n</header>")
	return template.HTML(b.String())
}

// footerHTML is the product footer. version is the Stageproof release that
// made the reports.
func footerHTML(version string) template.HTML {
	esc := template.HTMLEscapeString
	var b strings.Builder
	b.WriteString(`<footer class="footer">` + "\n" + `<div class="wrap">` + "\n" + `<div class="footer-top">` + "\n")
	b.WriteString(`<div class="footer-brand">` + "\n" + brandLink())
	b.WriteString("<p>Checks a Kubernetes upgrade before you merge it. Free and open source (Apache-2.0).</p>\n</div>\n")
	b.WriteString(`<div class="footer-cols">` + "\n")
	for _, group := range FooterNav() {
		b.WriteString(`<div class="footer-col">` + "\n" + `<span class="footer-h">` + esc(group.Title) + "</span>\n")
		for _, link := range group.Links {
			b.WriteString(`<a href="` + esc(link.Href) + `">` + esc(link.Label) + "</a>\n")
		}
		b.WriteString("</div>\n")
	}
	b.WriteString("</div>\n</div>\n" + `<div class="footer-bottom">` + "\n")
	b.WriteString("<span>© 2026 Stageproof · Apache-2.0</span>\n")
	if version != "" {
		b.WriteString(`<span class="status-pill"><span class="dot-live"></span>Reports made with ` + esc(version) + "</span>\n")
	}
	b.WriteString("</div>\n</div>\n</footer>")
	return template.HTML(b.String())
}

// headHTML loads the shared fonts, stylesheet, and theme script.
// site.js runs before the body so a stored light theme paints first.
func headHTML(siteJS []byte) template.HTML {
	esc := template.HTMLEscapeString
	sum := sha512.Sum384(siteJS)
	sri := "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
	var b strings.Builder
	for _, font := range []string{"space-grotesk-latin", "inter-latin"} {
		b.WriteString(`<link rel="preload" href="` + esc(Href("/assets/fonts/"+font+".woff2")) + `" as="font" type="font/woff2" crossorigin>` + "\n")
	}
	b.WriteString(`<link rel="icon" href="` + esc(Href("/assets/favicon.svg")) + `" type="image/svg+xml">` + "\n")
	b.WriteString(`<link rel="stylesheet" href="` + esc(Href("/assets/site.css")) + `">` + "\n")
	b.WriteString(`<script src="` + esc(Href("/assets/site.js")) + `" integrity="` + sri + `"></script>`)
	return template.HTML(b.String())
}
