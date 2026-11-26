package site

import (
	"crypto/sha512"
	"encoding/base64"
	"html/template"
	"strings"
)

// ProductURL is the Stageproof product site. The index header is its header.
const ProductURL = "https://stageproof.github.io/stageproof/"

// NavLink is one link in a header row.
type NavLink struct {
	Label string
	Href  string
}

// ChartIndexLabel is the product header entry that points at this index.
const ChartIndexLabel = "Chart Index"

// ProductNav is the primary header of the product site, in its order.
// web/*.html in stageproof/stageproof pins the same labels and pages.
func ProductNav() []NavLink {
	return []NavLink{
		{Label: "Overview", Href: ProductURL},
		{Label: ChartIndexLabel, Href: Href(Root)},
		{Label: "Docs", Href: ProductURL + "docs.html"},
		{Label: "Architecture", Href: ProductURL + "architecture.html"},
		{Label: "Security", Href: ProductURL + "security.html"},
		{Label: "Roadmap", Href: ProductURL + "roadmap.html"},
	}
}

// IndexNav is the second header row on index pages.
func IndexNav() []NavLink {
	return []NavLink{
		{Label: "Home", Href: Href(Root)},
		{Label: "Charts", Href: Href(Charts)},
		{Label: "Your values", Href: Href(Values)},
		{Label: "API", Href: Href(API)},
	}
}

// indexSection names the IndexNav entry for a page kind.
func indexSection(kind string) string {
	switch kind {
	case "home":
		return "Home"
	case "charts", "chart", "upgrade":
		return "Charts"
	case "values":
		return "Your values"
	case "api":
		return "API"
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

// headerHTML is the header every index page shows.
func headerHTML(kind string) template.HTML {
	esc := template.HTMLEscapeString
	var b strings.Builder
	b.WriteString(`<a class="skip" href="#main">Skip to content</a>` + "\n")
	b.WriteString(`<header class="nav">` + "\n" + `<div class="nav-inner">` + "\n")
	b.WriteString(`<a class="brand" href="` + esc(ProductURL) + `">` + "\n" + brandMark + "\n")
	b.WriteString(`<span class="brand-word">stageproof</span>` + "\n</a>\n")
	b.WriteString(`<nav class="nav-links" id="nav-links" aria-label="Primary">` + "\n")
	for _, link := range ProductNav() {
		b.WriteString(`<a href="` + esc(link.Href) + `"`)
		if link.Label == ChartIndexLabel {
			b.WriteString(` class="active" aria-current="page"`)
		}
		b.WriteString(`>` + esc(link.Label) + "</a>\n")
	}
	b.WriteString("</nav>\n" + `<div class="nav-cta">` + "\n" + themeToggle + "\n")
	b.WriteString(`<a class="btn btn-primary btn-sm" href="` + esc(ProductURL+"docs.html#first-check") + `">Quickstart</a>` + "\n</div>\n")
	b.WriteString(`<button class="nav-toggle" id="nav-toggle" type="button" aria-expanded="false" aria-controls="nav-links" aria-label="Toggle navigation">Menu</button>` + "\n")
	b.WriteString("</div>\n" + `<nav class="subnav" aria-label="Chart Index">` + "\n" + `<div class="subnav-inner">` + "\n")
	b.WriteString(`<span class="subnav-title">` + ChartIndexLabel + "</span>\n")
	active := indexSection(kind)
	for _, link := range IndexNav() {
		b.WriteString(`<a href="` + esc(link.Href) + `"`)
		if link.Label == active {
			b.WriteString(` class="active" aria-current="page"`)
		}
		b.WriteString(`>` + esc(link.Label) + "</a>\n")
	}
	b.WriteString("</div>\n</nav>\n</header>")
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
