package site

import (
	"crypto/sha512"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

//go:embed assets/values.js assets/values.css
var valueAssets embed.FS

type chartPairsDoc struct {
	ID          string          `json:"id"`
	RepoName    string          `json:"repo_name"`
	Chart       string          `json:"chart"`
	DefaultFrom string          `json:"default_from"`
	DefaultTo   string          `json:"default_to"`
	Pairs       []chartPairItem `json:"pairs"`
}

type chartPairItem struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Adjacent   bool   `json:"adjacent,omitempty"`
	Status     string `json:"status"`
	ValuesKeys int    `json:"values_keys,omitempty"`
}

func chartPairsJSON(item chart, pairs []pairFile) ([]byte, error) {
	doc := chartPairsDoc{
		ID:       item.ID,
		RepoName: item.RepoName,
		Chart:    item.Chart,
		Pairs:    make([]chartPairItem, 0, len(pairs)),
	}
	for _, p := range pairs {
		_, class := linkStatus(p.Blocked, p.NotChecked)
		doc.Pairs = append(doc.Pairs, chartPairItem{
			From:       p.From,
			To:         p.To,
			Adjacent:   p.Adjacent,
			Status:     class,
			ValuesKeys: countValuesKeys(p.Doc),
		})
		if doc.DefaultFrom == "" && p.Adjacent {
			doc.DefaultFrom = p.From
			doc.DefaultTo = p.To
		}
	}
	if doc.DefaultFrom == "" && len(pairs) > 0 {
		doc.DefaultFrom = pairs[0].From
		doc.DefaultTo = pairs[0].To
	}
	return json.Marshal(doc)
}

func countValuesKeys(doc envelope) int {
	seen := make(map[string]struct{})
	for _, rep := range doc.Results {
		for _, f := range rep.Findings {
			if f.Ignored || f.Resource == "" {
				continue
			}
			if strings.HasPrefix(f.RuleID, "helm.values.") {
				seen[f.RuleID+"\x00"+f.Resource] = struct{}{}
			}
		}
	}
	return len(seen)
}

func valuesLink(repo, chart, from, to string) string {
	q := url.Values{}
	q.Set("repo", repo)
	q.Set("chart", chart)
	q.Set("from", from)
	q.Set("to", to)
	return Href(Values) + "?" + q.Encode()
}

func addValuesPage(pages *built) error {
	js, err := valueAssets.ReadFile("assets/values.js")
	if err != nil {
		return err
	}
	css, err := valueAssets.ReadFile("assets/values.css")
	if err != nil {
		return err
	}
	sum := sha512.Sum384(js)
	sri := "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
	pages.raw = append(pages.raw,
		rawFile{route: "/values/values.js", body: js},
		rawFile{route: "/values/values.css", body: css},
		rawFile{route: Values, body: []byte(valuesHTML(sri))},
	)
	return nil
}

func valuesHTML(sri string) string {
	css := Href("/values/values.css")
	js := Href("/values/values.js")
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="Content-Security-Policy" content="default-src 'self'; connect-src 'self'; form-action 'none'; base-uri 'self'; object-src 'none'">
<title>Your values - Stageproof index</title>
<link rel="stylesheet" href="%s">
</head>
<body data-root="%s">
<header>
<a href="%s">Stageproof index</a>
<a href="%s">Charts</a>
<a href="%s">API</a>
<a href="%s" class="active">Your values</a>
</header>
<main>
<div class="hero">
<h1>Check your values.yaml against an indexed upgrade</h1>
<p class="privacy-pill">Runs 100%% in your browser. Zero uploads. No Helm executed.</p>
<p class="lede">Pick a chart and version jump from the index, then paste your <code>values.yaml</code> to see which changed defaults, required keys, or dropped keys affect your config.</p>
</div>

<section class="panel" aria-label="Select upgrade">
<div class="field">
<div class="label-row">
<label for="chart-search">Chart</label>
<span id="chart-count" class="muted">Loading charts...</span>
</div>
<input id="chart-search" type="search" placeholder="Filter by chart or repo name, e.g. traefik, cert-manager, argo-cd" autocomplete="off">
<select id="chart" aria-label="Chart"></select>
</div>

<div class="version-grid">
<div class="field">
<label for="from">From version</label>
<select id="from" disabled></select>
</div>
<div class="arrow" aria-hidden="true">-&gt;</div>
<div class="field">
<label for="to">To version</label>
<select id="to" disabled></select>
</div>
</div>

<div id="pair-meta" class="pair-meta" hidden></div>
</section>

<section class="panel" aria-label="Values YAML">
<div class="label-row">
<label for="yaml">Your values.yaml</label>
<div class="toolbar">
<input id="yaml-file" type="file" accept=".yaml,.yml,text/yaml,text/plain" hidden>
<button type="button" id="load-file" class="btn-secondary">Load .yaml file</button>
<button type="button" id="clear-yaml" class="btn-secondary">Clear</button>
</div>
</div>
<textarea id="yaml" spellcheck="false" placeholder="# Paste your values.yaml here, or drop a .yaml file&#10;# Only key paths are read in this browser. Nothing is uploaded.&#10;replicaCount: 3"></textarea>
<div class="action-row">
<button type="button" id="run" class="btn-primary">Check</button>
<span id="key-count" class="muted">0 keys in paste</span>
<span class="hint muted">Ctrl+Enter to check</span>
</div>
<p id="status" class="status" role="status"></p>
</section>

<div id="out" class="results"></div>
</main>
<footer>
<p>Stored reports are generated with chart defaults only. Keys that only appear in your custom values file require running the Stageproof CLI locally with <code>-f values.yaml</code>.</p>
</footer>
<script src="%s" integrity="%s" defer></script>
</body>
</html>
`, css, BasePath(), Href(Root), Href(Charts), Href(API), Href(Values), js, sri)
}
