package site

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGenerateLinksVersionsAndSitemap(t *testing.T) {
	data := t.TempDir()
	writeFixture(t, data)
	out := filepath.Join(t.TempDir(), "site")
	if err := Generate(data, out); err != nil {
		t.Fatal(err)
	}
	clean := readPage(t, out, "upgrade/fixtures/web/0.1.0/0.2.0/index.html")
	broken := readPage(t, out, "upgrade/fixtures/web/0.2.0/0.3.0/index.html")
	jump := readPage(t, out, "upgrade/fixtures/web/0.1.0/0.3.0/index.html")
	empty := readPage(t, out, "chart/fixtures/empty/index.html")
	if strings.Contains(clean, `content="noindex"`) || !strings.Contains(clean, "No blocking finding") {
		t.Fatalf("clean page\n%s", clean)
	}
	if err := banned(out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(broken, "port is required") || !strings.Contains(broken, "crd.schema.incompatible") || !strings.Contains(broken, "supply.image.not_found") {
		t.Fatalf("broken page missing findings\n%s", broken)
	}
	banner := strings.Index(broken, "Checked with chart defaults only.")
	block := strings.Index(broken, "port is required")
	if block < 0 || banner < 0 || block > banner {
		t.Fatalf("breaks should lead the page, banner %d block %d", banner, block)
	}
	if !strings.Contains(broken, Disclaimer) {
		t.Fatal("disclaimer missing")
	}
	if !strings.Contains(broken, "stageproof check --repo https://charts.example/fixtures --chart web --from 0.2.0 --to 0.3.0") {
		t.Fatal("reproduce command missing")
	}
	if !strings.Contains(broken, "Report a wrong result") || !strings.Contains(broken, "template=bug.yml") || !strings.Contains(broken, "v0.3.0") {
		t.Fatal("bug link missing")
	}
	if !strings.Contains(broken, "https://example.com/notes") {
		t.Fatal("release notes missing")
	}
	if !strings.Contains(jump, `content="noindex"`) || !strings.Contains(empty, `content="noindex"`) {
		t.Fatal("jump or empty page is indexable")
	}
	siteMap := string(readPage(t, out, "sitemap.xml"))
	for _, want := range []string{
		"https://stageproof.github.io/index/charts/",
		"https://stageproof.github.io/index/chart/fixtures/web/",
		"https://stageproof.github.io/index/upgrade/fixtures/web/0.1.0/0.2.0/",
		"https://stageproof.github.io/index/upgrade/fixtures/web/0.2.0/0.3.0/",
	} {
		if !strings.Contains(siteMap, want) {
			t.Fatalf("sitemap missing %s\n%s", want, siteMap)
		}
	}
	for _, skip := range []string{
		"upgrade/fixtures/web/0.1.0/0.3.0",
		"chart/fixtures/empty",
		"stageproof.dev",
	} {
		if strings.Contains(siteMap, skip) {
			t.Fatalf("sitemap includes %s\n%s", skip, siteMap)
		}
	}
	api := readPage(t, out, "api/v1/upgrade/fixtures/web/0.2.0/0.3.0.json")
	src := readPage(t, data, "v1/charts/fixtures/web/0.2.0..0.3.0.json")
	if api != src {
		t.Fatal("upgrade JSON is not a byte copy")
	}
	if readPage(t, out, "api/v1/index.json") != readPage(t, data, "v1/index.json") {
		t.Fatal("catalog JSON is not a byte copy")
	}
	bad, err := BrokenLinks(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 0 {
		t.Fatalf("broken links %v", bad)
	}
	charts := readPage(t, out, "charts/index.html")
	if !strings.Contains(charts, "fixtures/web") || !strings.Contains(charts, `id="q"`) || !strings.Contains(charts, "Latest upgrade") {
		t.Fatal("catalog search missing")
	}
	if !strings.Contains(charts, ">none<") || !strings.Contains(charts, ">blocked<") {
		t.Fatal("latest upgrade column missing")
	}
	home := readPage(t, out, "index.html")
	for _, want := range []string{"2 listed, 1 with a report.", "3 upgrade pairs.", "1 blocked.", "Stageproof v0.3.0.", "fixtures/web 0.2.0 to 0.3.0"} {
		if !strings.Contains(home, want) {
			t.Fatalf("home missing %s\n%s", want, home)
		}
	}
	chart := readPage(t, out, "chart/fixtures/web/index.html")
	if strings.Contains(chart, `content="noindex"`) || !strings.Contains(chart, "https://charts.example/fixtures") {
		t.Fatal("chart page should be indexed and link the repository")
	}
	latestH := strings.Index(chart, "Latest upgrade")
	stepH := strings.Index(chart, "Step-by-step upgrades")
	jumpH := strings.Index(chart, "Jumps to latest")
	newest := strings.Index(chart, "0.2.0 to 0.3.0")
	older := strings.Index(chart, "0.1.0 to 0.2.0")
	jumpAt := strings.Index(chart, "0.1.0 to 0.3.0")
	if latestH < 0 || stepH < latestH || jumpH < stepH || newest < latestH || newest > stepH || older < stepH || older > jumpH || jumpAt < jumpH {
		t.Fatal("newest adjacent upgrade should lead, then steps, then jumps")
	}
	for _, page := range []string{clean, broken, jump, empty, charts, chart, readPage(t, out, "index.html")} {
		if !strings.Contains(page, "<!DOCTYPE html>") || !strings.Contains(page, "</html>") {
			t.Fatal("page is not a complete HTML document")
		}
	}
	if strings.Contains(siteMap, "<loc>https://stageproof.github.io/index/</loc>") {
		t.Fatal("sitemap includes the home page")
	}
}

func TestChartFailureReasonSkipsHelmNoise(t *testing.T) {
	summary := "helm template 10.6.1 failed: exit status 1\nError: execution error at (gitlab/templates/NOTES.txt:248:3):\nCONFIGURATION CHECKS:\n\nThe chart no longer provides a bundled object storage solution."
	got := chartFailureReason(summary)
	if !strings.Contains(got, "CONFIGURATION CHECKS") || !strings.Contains(got, "bundled object storage") {
		t.Fatal(got)
	}
	if strings.Contains(got, "exit status") || strings.Contains(got, "execution error") {
		t.Fatal(got)
	}
}

func TestNotCheckedPairIsNotDescribedAsClean(t *testing.T) {
	data := t.TempDir()
	writeFile(t, filepath.Join(data, "v1", "index.json"), `{
	  "schema_version": 1,
	  "stageproof_version": "v0.3.0",
	  "notice": "chart defaults only; run with your values files",
	  "charts": [
	    {"id": "fixtures/held", "repo": "https://charts.example/fixtures", "repo_name": "fixtures", "chart": "held", "latest": "1.0.1", "pairs": 1, "blocked": 0}
	  ]
	}`)
	dir := filepath.Join(data, "v1", "charts", "fixtures", "held")
	writeFile(t, filepath.Join(dir, "meta.json"), `{"versions":["1.0.0","1.0.1"]}`)
	writeFile(t, filepath.Join(dir, "1.0.0..1.0.1.json"), `{
	  "stageproof_version": "v0.3.0",
	  "scope": "chart-defaults-only",
	  "reproduce_command": "stageproof check --repo https://charts.example/fixtures --chart held --from 1.0.0 --to 1.0.1",
	  "k8s_results": {"1.35": {"verdict": "NOT CHECKED", "exit_code": 2, "checks": [
	    {"name": "baseline helm template", "status": "fail", "summary": "chart requires kubeVersion >= 1.21.0-0 < 1.31.0-0\nsecond line"}
	  ], "findings": []}}
	}`)
	out := filepath.Join(t.TempDir(), "site")
	if err := Generate(data, out); err != nil {
		t.Fatal(err)
	}
	chart := readPage(t, out, "chart/fixtures/held/index.html")
	if !strings.Contains(chart, `class="unchecked">not checked`) || strings.Contains(chart, "no blocking finding") || strings.Contains(chart, `class="ok"`) {
		t.Fatalf("chart page treated an unchecked pair as a pass\n%s", chart)
	}
	if !strings.Contains(chart, "chart requires kubeVersion &gt;= 1.21.0-0 &lt; 1.31.0-0") {
		t.Fatalf("chart page should quote the required-values failure\n%s", chart)
	}
	upgrade := readPage(t, out, "upgrade/fixtures/held/1.0.0/1.0.1/index.html")
	for _, want := range []string{
		"This pair could not be checked with chart defaults.",
		"baseline helm template",
		"chart requires kubeVersion &gt;= 1.21.0-0 &lt; 1.31.0-0 second line",
	} {
		if !strings.Contains(upgrade, want) {
			t.Fatalf("upgrade page missing %s\n%s", want, upgrade)
		}
	}
	if strings.Contains(upgrade, "No blocking finding") {
		t.Fatal("unchecked upgrade claimed there was no blocking finding")
	}
}

func TestLongChartGroupsStepsAndFilters(t *testing.T) {
	data := t.TempDir()
	writeFile(t, filepath.Join(data, "v1", "index.json"), `{
	  "schema_version": 1,
	  "stageproof_version": "v0.3.0",
	  "charts": [
	    {"id": "fixtures/long", "repo": "https://charts.example/fixtures", "repo_name": "fixtures", "chart": "long", "latest": "0.22.0", "pairs": 22, "blocked": 0}
	  ]
	}`)
	var versions []string
	for i := 1; i <= 22; i++ {
		versions = append(versions, fmt.Sprintf("0.%d.0", i))
	}
	dir := filepath.Join(data, "v1", "charts", "fixtures", "long")
	writeFile(t, filepath.Join(dir, "meta.json"), `{"versions":["`+strings.Join(versions, `","`)+`"]}`)
	body := `{"stageproof_version":"v0.3.0","scope":"chart-defaults-only","reproduce_command":"stageproof check","k8s_results":{"1.35":{"verdict":"PASSED","exit_code":0,"findings":[]}}}`
	for i := 0; i+1 < len(versions); i++ {
		writeFile(t, filepath.Join(dir, versions[i]+".."+versions[i+1]+".json"), body)
	}
	writeFile(t, filepath.Join(dir, "0.1.0..0.22.0.json"), body)
	out := filepath.Join(t.TempDir(), "site")
	if err := Generate(data, out); err != nil {
		t.Fatal(err)
	}
	chart := readPage(t, out, "chart/fixtures/long/index.html")
	if !strings.Contains(chart, `id="steps"`) || !strings.Contains(chart, "<details>") || !strings.Contains(chart, "0.1.0 to 0.22.0") {
		t.Fatal("long chart is missing the filter or the jump list")
	}
	latest := strings.Index(chart, "0.21.0 to 0.22.0")
	jumpAt := strings.Index(chart, "0.1.0 to 0.22.0")
	older := strings.Index(chart, "0.1.0 to 0.2.0")
	if latest < 0 || older < 0 || jumpAt < 0 || latest > older || older > jumpAt {
		t.Fatal("long chart should list the newest step before older steps and jumps")
	}
	bad, err := BrokenLinks(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 0 {
		t.Fatalf("broken links %v", bad)
	}
}

func TestGenerateRefusesTheWorkingDirectory(t *testing.T) {
	for _, out := range []string{".", "/"} {
		if err := Generate(t.TempDir(), out); err == nil {
			t.Fatalf("accepted %s", out)
		}
	}
}

func TestHTTPLinkAllowsBrowserURLsOnly(t *testing.T) {
	if httpLink("https://example.com/web") != "https://example.com/web" || httpLink("http://example.com") != "http://example.com" {
		t.Fatal("http links were dropped")
	}
	if httpLink("oci://example.com/web") != "" || httpLink("javascript:alert(1)") != "" {
		t.Fatal("non-browser URL became a link")
	}
}

func banned(dir string) error {
	return filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		buf, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(buf)
		if strings.HasSuffix(path, ".woff2") {
			return nil
		}
		for _, word := range []string{"—", "stageproof.dev", "google-analytics", "plausible", "googletagmanager", "fonts.googleapis.com", "fonts.gstatic.com"} {
			if strings.Contains(text, word) {
				return fmt.Errorf("%s contains %s", path, word)
			}
		}
		return nil
	})
}

func TestGenerateRejectsAParentSegment(t *testing.T) {
	data := t.TempDir()
	writeFile(t, filepath.Join(data, "v1", "index.json"), `{
	  "schema_version": 1,
	  "charts": [{"id": "bad", "repo": "https://example.com", "repo_name": "..", "chart": "web"}]
	}`)
	err := Generate(data, filepath.Join(t.TempDir(), "site"))
	if err == nil || !strings.Contains(err.Error(), "invalid path segment") {
		t.Fatal(err)
	}
}

func TestGenerateThreeHundredChartsWithinThirtySeconds(t *testing.T) {
	data := t.TempDir()
	var b strings.Builder
	b.WriteString(`{"schema_version":1,"stageproof_version":"v0.3.0","notice":"chart defaults only; run with your values files","charts":[`)
	for i := 0; i < 300; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		name := fmt.Sprintf("chart-%03d", i)
		fmtPair(&b, name)
	}
	b.WriteString("]}")
	writeFile(t, filepath.Join(data, "v1", "index.json"), b.String())
	for i := 0; i < 300; i++ {
		name := fmt.Sprintf("chart-%03d", i)
		dir := filepath.Join(data, "v1", "charts", "fixtures", name)
		writeFile(t, filepath.Join(dir, "meta.json"), `{"versions":["0.1.0","0.2.0"]}`)
		writeFile(t, filepath.Join(dir, "0.1.0..0.2.0.json"), `{"stageproof_version":"v0.3.0","scope":"chart-defaults-only","reproduce_command":"stageproof check --repo https://charts.example/fixtures --chart `+name+` --from 0.1.0 --to 0.2.0 --values-proof full --current-k8s 1.35 --api-server off --format json","k8s_results":{"1.35":{"verdict":"PASSED","exit_code":0,"findings":[]}}}`)
	}
	out := filepath.Join(t.TempDir(), "site")
	start := time.Now()
	if err := Generate(data, out); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 30*time.Second {
		t.Fatalf("generate took %s", time.Since(start))
	}
	bad, err := BrokenLinks(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(bad) != 0 {
		t.Fatalf("broken links %d, first %s", len(bad), bad[0])
	}
}

func writeFixture(t *testing.T, data string) {
	t.Helper()
	writeFile(t, filepath.Join(data, "v1", "index.json"), `{
	  "schema_version": 1,
	  "stageproof_version": "v0.3.0",
	  "notice": "chart defaults only; run with your values files",
	  "charts": [
	    {"id": "fixtures/empty", "repo": "https://charts.example/fixtures", "repo_name": "fixtures", "chart": "empty", "latest": "0.1.0", "error": "chart empty is not in https://charts.example/fixtures"},
	    {"id": "fixtures/web", "repo": "https://charts.example/fixtures", "repo_name": "fixtures", "chart": "web", "homepage": "https://example.com/web", "release_notes_url": "https://example.com/notes", "latest": "0.3.0", "pairs": 3, "blocked": 1}
	  ]
	}`)
	dir := filepath.Join(data, "v1", "charts", "fixtures", "web")
	writeFile(t, filepath.Join(dir, "meta.json"), `{"versions":["0.1.0","0.2.0","0.3.0"]}`)
	writeFile(t, filepath.Join(dir, "0.1.0..0.2.0.json"), `{
	  "stageproof_version": "v0.3.0",
	  "scope": "chart-defaults-only",
	  "reproduce_command": "stageproof check --repo https://charts.example/fixtures --chart web --from 0.1.0 --to 0.2.0 --values-proof full --current-k8s 1.35 --api-server off --format json",
	  "k8s_results": {"1.35": {"verdict": "PASSED", "exit_code": 0, "findings": []}}
	}`)
	writeFile(t, filepath.Join(dir, "0.2.0..0.3.0.json"), `{
	  "stageproof_version": "v0.3.0",
	  "scope": "chart-defaults-only",
	  "reproduce_command": "stageproof check --repo https://charts.example/fixtures --chart web --from 0.2.0 --to 0.3.0 --values-proof full --current-k8s 1.35 --api-server off --format json",
	  "k8s_results": {"1.35": {"verdict": "BLOCKED", "exit_code": 1, "findings": [
	    {"rule_id": "helm.values.new_required_unset", "severity": "blocker", "message": "port is required"},
	    {"rule_id": "crd.schema.incompatible", "severity": "blocker", "message": "CRD schema dropped a field"},
	    {"rule_id": "supply.image.not_found", "severity": "warning", "message": "image tag is missing"}
	  ]}}
	}`)
	writeFile(t, filepath.Join(dir, "0.1.0..0.3.0.json"), `{
	  "stageproof_version": "v0.3.0",
	  "scope": "chart-defaults-only",
	  "reproduce_command": "stageproof check --repo https://charts.example/fixtures --chart web --from 0.1.0 --to 0.3.0 --values-proof full --current-k8s 1.35 --api-server off --format json",
	  "k8s_results": {"1.35": {"verdict": "PASSED", "exit_code": 0, "findings": []}}
	}`)
	writeFile(t, filepath.Join(data, "v1", "charts", "fixtures", "empty", "meta.json"), `{"versions":["0.1.0"]}`)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readPage(t *testing.T, root, rel string) string {
	t.Helper()
	buf, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(buf)
}

func fmtPair(b *strings.Builder, name string) {
	b.WriteString(`{"id":"fixtures/` + name + `","repo":"https://charts.example/fixtures","repo_name":"fixtures","chart":"` + name + `","latest":"0.2.0","pairs":1,"blocked":0}`)
}
