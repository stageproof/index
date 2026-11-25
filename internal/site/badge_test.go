package site

import (
	"bytes"
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBadgeURLSchemeFrozen(t *testing.T) {
	file, err := BadgeFile("fixtures", "web", "0.1.0", "0.2.0")
	if err != nil || file != "/badge/fixtures/web/0.1.0/0.2.0.svg" {
		t.Fatalf("file %s %v", file, err)
	}
	query, err := BadgeQuery("https://charts.example/fixtures/", "web", "0.1.0", "0.2.0")
	const frozenQuery = "/badge.svg?chart=web&from=0.1.0&registry=https%3A%2F%2Fcharts.example%2Ffixtures%2F&to=0.2.0"
	if err != nil || query != frozenQuery {
		t.Fatalf("query %s %v", query, err)
	}
	if _, err := BadgeQuery("https://charts.example/fixtures", "../web", "0.1.0", "0.2.0"); err == nil {
		t.Fatal("accepted a parent chart segment")
	}

	cases := []struct {
		name string
		doc  envelope
		text string
		kind string
		svg  string
	}{
		{
			name: "clear",
			doc:  envelope{Results: map[string]k8sReport{"1.35": {Verdict: "PASSED"}}},
			text: "no blocking finding (defaults)",
			kind: "clear",
			svg: `<svg xmlns="http://www.w3.org/2000/svg" width="304" height="20" role="img" aria-label="stageproof: no blocking finding (defaults)">
<title>stageproof: no blocking finding (defaults)</title>
<rect width="82" height="20" fill="#555"/>
<rect x="82" width="222" height="20" fill="#1f6feb"/>
<g fill="#fff" text-anchor="middle" font-family="Verdana,DejaVu Sans,sans-serif" font-size="11">
<text x="41" y="14">stageproof</text>
<text x="193" y="14">no blocking finding (defaults)</text>
</g>
</svg>
`,
		},
		{
			name: "blocked",
			doc: envelope{Results: map[string]k8sReport{"1.35": {
				Verdict: "BLOCKED",
				Findings: []finding{{
					RuleID: "k8s.api.removed", Severity: "blocker", Message: "API batch/v1beta1 is not served",
				}},
			}}},
			text: "removed K8s API",
			kind: "blocked",
			svg: `<svg xmlns="http://www.w3.org/2000/svg" width="199" height="20" role="img" aria-label="stageproof: removed K8s API">
<title>stageproof: removed K8s API</title>
<rect width="82" height="20" fill="#555"/>
<rect x="82" width="117" height="20" fill="#c92a2a"/>
<g fill="#fff" text-anchor="middle" font-family="Verdana,DejaVu Sans,sans-serif" font-size="11">
<text x="41" y="14">stageproof</text>
<text x="140" y="14">removed K8s API</text>
</g>
</svg>
`,
		},
		{
			name: "unchecked",
			doc:  envelope{Results: map[string]k8sReport{"1.35": {Verdict: "NOT CHECKED"}}},
			text: "not checked",
			kind: "unchecked",
			svg: `<svg xmlns="http://www.w3.org/2000/svg" width="171" height="20" role="img" aria-label="stageproof: not checked">
<title>stageproof: not checked</title>
<rect width="82" height="20" fill="#555"/>
<rect x="82" width="89" height="20" fill="#6c757d"/>
<g fill="#fff" text-anchor="middle" font-family="Verdana,DejaVu Sans,sans-serif" font-size="11">
<text x="41" y="14">stageproof</text>
<text x="126" y="14">not checked</text>
</g>
</svg>
`,
		},
	}
	for _, tc := range cases {
		text, kind := BadgeOf(tc.doc)
		if text != tc.text || kind != tc.kind {
			t.Fatalf("%s label %q %s", tc.name, text, kind)
		}
		if got := string(RenderBadge(text, kind)); got != tc.svg {
			t.Fatalf("%s svg\n%s", tc.name, got)
		}
		if forbiddenBadge(text) {
			t.Fatalf("%s uses a forbidden word", tc.name)
		}
	}
	unknown := `<svg xmlns="http://www.w3.org/2000/svg" width="171" height="20" role="img" aria-label="stageproof: not indexed">
<title>stageproof: not indexed</title>
<rect width="82" height="20" fill="#555"/>
<rect x="82" width="89" height="20" fill="#6c757d"/>
<g fill="#fff" text-anchor="middle" font-family="Verdana,DejaVu Sans,sans-serif" font-size="11">
<text x="41" y="14">stageproof</text>
<text x="126" y="14">not indexed</text>
</g>
</svg>
`
	if got := string(UnknownSVG()); got != unknown {
		t.Fatalf("unknown svg\n%s", got)
	}
	worker, err := os.ReadFile(filepath.Join("..", "..", "worker", "src", "index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(worker), unknown) {
		t.Fatal("worker fallback SVG drifted from UnknownSVG")
	}
	if !strings.Contains(string(worker), string(MissingPairSVG())) {
		t.Fatal("worker missing-pair SVG drifted from MissingPairSVG")
	}

	crd, kind := BadgeOf(envelope{Results: map[string]k8sReport{"1.35": {
		Verdict: "BLOCKED",
		Findings: []finding{{
			RuleID: "crd.schema.incompatible", Severity: "blocker", Message: "field removed",
		}},
	}}})
	if crd != "CRD schema break" || kind != "blocked" {
		t.Fatalf("crd %q %s", crd, kind)
	}
	values, kind := BadgeOf(envelope{Results: map[string]k8sReport{"1.35": {
		Verdict: "BLOCKED",
		Findings: []finding{
			{RuleID: "helm.values.silently_dropped", Severity: "blocker", Message: "key size dropped"},
			{RuleID: "helm.values.silently_dropped", Severity: "blocker", Message: "key port dropped"},
			{RuleID: "helm.values.silently_dropped", Severity: "blocker", Message: "key image dropped"},
		},
	}}})
	if values != "3 value keys removed" || kind != "blocked" {
		t.Fatalf("values %q %s", values, kind)
	}
	claim, kind := BadgeOf(envelope{Results: map[string]k8sReport{"1.35": {
		Verdict: "BLOCKED",
		Findings: []finding{{
			RuleID: "helm.template.render_failed", Severity: "blocker", Message: "This upgrade is safe and 100% verified.",
		}},
	}}})
	if claim != "blocking finding" || kind != "blocked" || forbiddenBadge(claim) {
		t.Fatalf("claim leaked %q %s", claim, kind)
	}

	index, err := RegistryIndex([]chart{
		{Repo: "https://charts.example/fixtures/", RepoName: "fixtures", Chart: "web"},
		{Repo: "https://charts.example/fixtures", RepoName: "fixtures", Chart: "empty"},
	})
	if err != nil {
		t.Fatal(err)
	}
	repo, ok := LookupRepo(index, "https://charts.example/fixtures/", "web")
	if !ok || repo != "fixtures" {
		t.Fatalf("lookup %s %v", repo, ok)
	}
	if _, ok := LookupRepo(index, "oci://ghcr.io/stefanprodan/charts", "podinfo"); ok {
		t.Fatal("oci registry matched an http catalog")
	}
	if _, ok := LookupRepo(index, "https://charts.example/fixtures", ".."); ok {
		t.Fatal("lookup accepted a parent segment")
	}
	raw, err := RegistryJSON([]chart{
		{Repo: "https://charts.example/fixtures/", RepoName: "fixtures", Chart: "web"},
	})
	const frozenRegistry = `{
  "https://charts.example/fixtures": {
    "web": "fixtures"
  }
}
`
	if err != nil || string(raw) != frozenRegistry {
		t.Fatalf("registry json\n%s%v", raw, err)
	}
	if _, err := RegistryIndex([]chart{
		{Repo: "https://charts.example/fixtures", RepoName: "fixtures", Chart: "web"},
		{Repo: "https://charts.example/fixtures/", RepoName: "other", Chart: "web"},
	}); err == nil {
		t.Fatal("accepted two repo names for one registry and chart")
	}
}

func TestBadgeWorkerURL(t *testing.T) {
	const want = "https://stageproof-index-badges.stageproof.workers.dev/badge.svg"
	if BadgeWorkerURL != want {
		t.Fatal(BadgeWorkerURL)
	}
	tpl, err := template.New("page.html").Funcs(template.FuncMap{"href": Href}).ParseFS(templateFS, "templates/page.html")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "page", view{
		Kind: "api", Title: "JSON API",
		IndexPath:      "/index/api/v1/index.json",
		UpgradePattern: "/index/api/v1/upgrade/<repo>/<chart>/<from>/<to>.json",
		BadgeURL:       BadgeWorkerURL,
	}); err != nil {
		t.Fatal(err)
	}
	page := buf.String()
	if !strings.Contains(page, want+"?chart=&amp;from=&amp;registry=&amp;to=") {
		t.Fatalf("API page does not print the worker URL\n%s", page)
	}
	if !strings.Contains(page, "not indexed") || !strings.Contains(page, "pair not indexed") {
		t.Fatal("API page does not name both badge texts")
	}
	if strings.Contains(page, "github.io/index/badge.svg") {
		t.Fatal("API page points the query at GitHub Pages")
	}
}

func TestGeneratedBadgesMatchTheReport(t *testing.T) {
	data := t.TempDir()
	writeFixture(t, data)
	out := filepath.Join(t.TempDir(), "site")
	if err := Generate(data, out); err != nil {
		t.Fatal(err)
	}
	clean := readPage(t, out, "badge/fixtures/web/0.1.0/0.2.0.svg")
	if clean != string(RenderBadge("no blocking finding (defaults)", "clear")) {
		t.Fatalf("clean badge\n%s", clean)
	}
	blocked := readPage(t, out, "badge/fixtures/web/0.2.0/0.3.0.svg")
	if blocked != string(RenderBadge("port is required", "blocked")) {
		t.Fatalf("blocked badge\n%s", blocked)
	}
	if !strings.Contains(readPage(t, out, "badge/registries.json"), `"https://charts.example/fixtures"`) {
		t.Fatal("registry map missing")
	}
	if readPage(t, out, "badge/not-indexed.svg") != string(UnknownSVG()) {
		t.Fatal("unknown badge file drifted")
	}
	for _, word := range []string{"safe", "verified", "PROVEN", "100% pass", "stageproof.dev"} {
		if strings.Contains(clean, word) || strings.Contains(blocked, word) {
			t.Fatalf("badge contains %s", word)
		}
	}
}
