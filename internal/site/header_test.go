package site

import (
	"crypto/sha512"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var (
	primaryNav  = regexp.MustCompile(`(?s)<nav class="nav-links" id="nav-links" aria-label="Primary">(.*?)</nav>`)
	footerBlock = regexp.MustCompile(`(?s)<footer class="footer">(.*?)</footer>`)
	navAnchor   = regexp.MustCompile(`<a href="([^"]+)"([^>]*)>([^<]+)</a>`)
)

type navEntry struct {
	label  string
	href   string
	active bool
}

func navEntries(block string) []navEntry {
	var out []navEntry
	for _, m := range navAnchor.FindAllStringSubmatch(block, -1) {
		out = append(out, navEntry{label: m[3], href: m[1], active: strings.Contains(m[2], `class="active"`)})
	}
	return out
}

func footerLinks() []NavLink {
	var out []NavLink
	for _, group := range FooterNav() {
		out = append(out, group.Links...)
	}
	return out
}

func sameLinks(t *testing.T, where string, got []navEntry, want []NavLink, href func(string) string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s links = %+v, want %+v", where, got, want)
	}
	for i := range want {
		if h := href(got[i].href); got[i].label != want[i].Label || h != want[i].Href {
			t.Fatalf("%s link %d = %s %s, want %s %s", where, i, got[i].label, h, want[i].Label, want[i].Href)
		}
	}
}

func TestEveryPageHasTheProductHeader(t *testing.T) {
	data := t.TempDir()
	writeFixture(t, data)
	out := filepath.Join(t.TempDir(), "site")
	if err := Generate(data, out); err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{
		"index.html":                                  "Charts",
		"charts/index.html":                           "Charts",
		"chart/fixtures/web/index.html":               "Charts",
		"values/index.html":                           "Check values",
		"api/index.html":                              "",
		"upgrade/fixtures/web/0.2.0/0.3.0/index.html": "Charts",
	}
	siteJS := readPage(t, out, "assets/site.js")
	sum := sha512.Sum384([]byte(siteJS))
	sri := "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
	same := func(h string) string { return h }
	for page, active := range pages {
		html := readPage(t, out, page)
		block := primaryNav.FindStringSubmatch(html)
		if block == nil {
			t.Fatalf("%s has no primary header", page)
		}
		got := navEntries(block[1])
		sameLinks(t, page+" header", got, ProductNav(), same)
		for _, entry := range got {
			if entry.active != (entry.label == active) {
				t.Fatalf("%s marks %s active=%v, want only %q", page, entry.label, entry.active, active)
			}
		}
		foot := footerBlock.FindStringSubmatch(html)
		if foot == nil {
			t.Fatalf("%s has no product footer", page)
		}
		sameLinks(t, page+" footer", navEntries(foot[1]), footerLinks(), same)
		if strings.Contains(html, "subnav") {
			t.Fatalf("%s still has a second header row", page)
		}
		for _, need := range []string{`id="theme-toggle"`, `id="nav-toggle"`, `id="main"`, `class="sub-hero"`, sri, "/assets/fonts/space-grotesk-latin.woff2"} {
			if !strings.Contains(html, need) {
				t.Fatalf("%s is missing %s", page, need)
			}
		}
	}
	css := readPage(t, out, "assets/site.css")
	for _, font := range []string{"inter-latin.woff2", "space-grotesk-latin.woff2", "jetbrains-mono-latin.woff2"} {
		if !strings.Contains(css, "fonts/"+font) {
			t.Fatalf("site.css does not load %s", font)
		}
		info, err := os.Stat(filepath.Join(out, "assets", "fonts", font))
		if err != nil || info.Size() == 0 {
			t.Fatalf("font %s was not written: %v", font, err)
		}
	}
	if !strings.Contains(readPage(t, out, "assets/fonts/LICENSE.txt"), "SIL OPEN FONT LICENSE") {
		t.Fatal("font license missing")
	}
	if !strings.Contains(css, `[data-theme="light"]`) {
		t.Fatal("light theme missing")
	}
}

// The live product site is the source of the header and the footer links.
// CI sets STAGEPROOF_CHECK_PRODUCT_NAV so a pull request cannot drift from it.
func TestHeaderMatchesTheLiveProductSite(t *testing.T) {
	if os.Getenv("STAGEPROOF_CHECK_PRODUCT_NAV") == "" {
		t.Skip("set STAGEPROOF_CHECK_PRODUCT_NAV=1 to compare with " + ProductURL)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	base, err := url.Parse(ProductURL)
	if err != nil {
		t.Fatal(err)
	}
	site, err := url.Parse(BaseURL)
	if err != nil {
		t.Fatal(err)
	}
	// Index links are site-relative here and absolute on the product site.
	local := func(h string) string {
		ref, err := url.Parse(h)
		if err != nil {
			t.Fatal(err)
		}
		abs := base.ResolveReference(ref)
		if abs.Host == site.Host && strings.HasPrefix(abs.Path, BasePath()+"/") {
			return abs.Path
		}
		s := abs.String()
		if i := strings.Index(s, "index.html"); i >= 0 && strings.HasPrefix(s, ProductURL) {
			s = s[:i] + s[i+len("index.html"):]
		}
		return s
	}
	for _, page := range []string{"", "docs.html", "architecture.html", "security.html", "roadmap.html"} {
		resp, err := client.Get(ProductURL + page)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		if cerr := resp.Body.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s%s: HTTP %d", ProductURL, page, resp.StatusCode)
		}
		block := primaryNav.FindSubmatch(body)
		if block == nil {
			t.Fatalf("%s%s has no primary header", ProductURL, page)
		}
		sameLinks(t, ProductURL+page+" header", navEntries(string(block[1])), ProductNav(), local)
		foot := footerBlock.FindSubmatch(body)
		if foot == nil {
			t.Fatalf("%s%s has no footer", ProductURL, page)
		}
		sameLinks(t, ProductURL+page+" footer", navEntries(string(foot[1])), footerLinks(), local)
	}
}
