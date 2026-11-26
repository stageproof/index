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
	primaryNav = regexp.MustCompile(`(?s)<nav class="nav-links" id="nav-links" aria-label="Primary">(.*?)</nav>`)
	secondNav  = regexp.MustCompile(`(?s)<nav class="subnav" aria-label="Chart Index">(.*?)</nav>`)
	navAnchor  = regexp.MustCompile(`<a href="([^"]+)"([^>]*)>([^<]+)</a>`)
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

func TestEveryPageHasTheProductHeader(t *testing.T) {
	data := t.TempDir()
	writeFixture(t, data)
	out := filepath.Join(t.TempDir(), "site")
	if err := Generate(data, out); err != nil {
		t.Fatal(err)
	}
	pages := map[string]string{
		"index.html":        "Home",
		"charts/index.html": "Charts",
		"values/index.html": "Your values",
		"api/index.html":    "API",
		"upgrade/fixtures/web/0.2.0/0.3.0/index.html": "Charts",
	}
	siteJS := readPage(t, out, "assets/site.js")
	sum := sha512.Sum384([]byte(siteJS))
	sri := "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
	for page, section := range pages {
		html := readPage(t, out, page)
		block := primaryNav.FindStringSubmatch(html)
		if block == nil {
			t.Fatalf("%s has no primary header", page)
		}
		got := navEntries(block[1])
		want := ProductNav()
		if len(got) != len(want) {
			t.Fatalf("%s header = %+v, want %+v", page, got, want)
		}
		for i := range want {
			if got[i].label != want[i].Label || got[i].href != want[i].Href {
				t.Fatalf("%s header link %d = %s %s, want %s %s", page, i, got[i].label, got[i].href, want[i].Label, want[i].Href)
			}
			if got[i].active != (want[i].Label == ChartIndexLabel) {
				t.Fatalf("%s marks %s active=%v", page, got[i].label, got[i].active)
			}
		}
		sub := secondNav.FindStringSubmatch(html)
		if sub == nil {
			t.Fatalf("%s has no Chart Index row", page)
		}
		active := ""
		for _, entry := range navEntries(sub[1]) {
			if entry.active {
				if active != "" {
					t.Fatalf("%s marks two Chart Index pages active", page)
				}
				active = entry.label
			}
		}
		if active != section {
			t.Fatalf("%s active section = %q, want %q", page, active, section)
		}
		for _, need := range []string{`id="theme-toggle"`, `id="nav-toggle"`, `id="main"`, sri, "/assets/fonts/space-grotesk-latin.woff2"} {
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

// The live product site is the source of the header. CI sets
// STAGEPROOF_CHECK_PRODUCT_NAV so a pull request cannot drift from it.
func TestHeaderMatchesTheLiveProductSite(t *testing.T) {
	if os.Getenv("STAGEPROOF_CHECK_PRODUCT_NAV") == "" {
		t.Skip("set STAGEPROOF_CHECK_PRODUCT_NAV=1 to compare with " + ProductURL)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	base, err := url.Parse(ProductURL)
	if err != nil {
		t.Fatal(err)
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
		got := navEntries(string(block[1]))
		want := ProductNav()
		if len(got) != len(want) {
			t.Fatalf("%s%s header = %+v, want %+v", ProductURL, page, got, want)
		}
		for i := range want {
			ref, err := url.Parse(got[i].href)
			if err != nil {
				t.Fatal(err)
			}
			live := strings.TrimSuffix(base.ResolveReference(ref).String(), "index.html")
			wantHref := want[i].Href
			if want[i].Label == ChartIndexLabel {
				wantHref = BaseURL
			}
			if got[i].label != want[i].Label || live != wantHref {
				t.Fatalf("%s%s link %d = %s %s, want %s %s", ProductURL, page, i, got[i].label, live, want[i].Label, wantHref)
			}
		}
	}
}
