package site

import (
	"crypto/sha512"
	"encoding/base64"
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
	js := readPage(t, out, "values/values.js")
	if !strings.Contains(page, "default-src 'self'; connect-src 'self'; form-action 'none'") {
		t.Fatal("csp missing")
	}
	if strings.Contains(page, "<script>") || strings.Contains(page, "onclick=") {
		t.Fatal("inline script")
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
