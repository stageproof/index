package site

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestValuesPageIsLocalAndHashed(t *testing.T) {
	data := t.TempDir()
	writeFixture(t, data)
	out := filepath.Join(t.TempDir(), "site")
	if err := Generate(data, out); err != nil {
		t.Fatal(err)
	}
	page := readPage(t, out, "values/index.html")
	home := readPage(t, out, "index.html")
	chartsPage := readPage(t, out, "charts/index.html")
	css := readPage(t, out, "assets/site.css")
	if !strings.Contains(css, "#06080D") {
		t.Fatal("shared dark stylesheet missing")
	}
	for name, html := range map[string]string{"values": page, "home": home, "charts": chartsPage} {
		if !strings.Contains(html, "/assets/site.css") {
			t.Fatalf("%s page missing shared stylesheet", name)
		}
		if strings.Contains(html, "<style>") {
			t.Fatalf("%s page still has inline style", name)
		}
	}
	js := readPage(t, out, "values/values.js")
	if !strings.Contains(page, "default-src 'self'; connect-src 'self'; form-action 'none'") {
		t.Fatal("csp missing")
	}
	if strings.Contains(page, "<script>") || strings.Contains(page, "onclick=") {
		t.Fatal("inline script")
	}
	if !strings.Contains(page, `>Check</button>`) {
		t.Fatal("primary button should be labeled Check")
	}
	if !strings.Contains(page, "Version you run now") || !strings.Contains(page, "Version you upgrade to") {
		t.Fatal("version labels missing")
	}
	if strings.Count(page, `id="chart"`) != 1 || !strings.Contains(page, `role="combobox"`) || strings.Contains(page, "chart-search") {
		t.Fatal("the chart should be one search field")
	}
	if strings.Contains(js, "next release") {
		t.Fatal("version labels should say newest, not next release")
	}
	sum := sha512.Sum384([]byte(js))
	sri := "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
	if !strings.Contains(page, sri) {
		t.Fatal("script hash does not match the file")
	}
	for _, banned := range []string{"https://", "http://", "POST", "PUT"} {
		if strings.Contains(js, banned) {
			t.Fatalf("values.js contains %s", banned)
		}
	}
	rawChart := readPage(t, out, "api/v1/chart/fixtures/web.json")
	var doc chartPairsDoc
	if err := json.Unmarshal([]byte(rawChart), &doc); err != nil {
		t.Fatalf("parse chart pairs json: %v", err)
	}
	if doc.DefaultFrom != "0.2.0" || doc.DefaultTo != "0.3.0" {
		t.Fatalf("default pair = %s -> %s, want 0.2.0 -> 0.3.0", doc.DefaultFrom, doc.DefaultTo)
	}
	if len(doc.Pairs) < 2 {
		t.Fatalf("expected at least 2 pairs, got %d", len(doc.Pairs))
	}
	upgrade := readPage(t, out, "upgrade/fixtures/web/0.2.0/0.3.0/index.html")
	if !strings.Contains(upgrade, "/values/?") || !strings.Contains(upgrade, "repo=fixtures") {
		t.Fatal("upgrade page does not link to the values check")
	}
	if !strings.Contains(readPage(t, out, "sitemap.xml"), "https://stageproof.github.io/index/values/") {
		t.Fatal("sitemap missing values page")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(node, "--test", "values.test.js")
	cmd.Dir = filepath.Join(wd, "assets")
	outb, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("node tests: %v\n%s", err, outb)
	}
}
