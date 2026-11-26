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

//go:embed assets/values.js assets/site.css assets/site.js assets/favicon.svg assets/fonts
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

func addValuesPage(pages *built, version string) error {
	js, err := valueAssets.ReadFile("assets/values.js")
	if err != nil {
		return err
	}
	css, err := valueAssets.ReadFile("assets/site.css")
	if err != nil {
		return err
	}
	icon, err := valueAssets.ReadFile("assets/favicon.svg")
	if err != nil {
		return err
	}
	siteJS, err := valueAssets.ReadFile("assets/site.js")
	if err != nil {
		return err
	}
	fonts, err := valueAssets.ReadDir("assets/fonts")
	if err != nil {
		return err
	}
	for _, font := range fonts {
		body, err := valueAssets.ReadFile("assets/fonts/" + font.Name())
		if err != nil {
			return err
		}
		pages.raw = append(pages.raw, rawFile{route: "/assets/fonts/" + font.Name(), body: body})
	}
	sum := sha512.Sum384(js)
	sri := "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
	pages.raw = append(pages.raw,
		rawFile{route: "/assets/site.css", body: css},
		rawFile{route: "/assets/site.js", body: siteJS},
		rawFile{route: "/assets/favicon.svg", body: icon},
		rawFile{route: "/values/values.js", body: js},
		rawFile{route: Values, body: []byte(valuesHTML(sri, siteJS, version))},
	)
	return nil
}
func valuesHTML(sri string, siteJS []byte, version string) string {
	js := Href("/values/values.js")
	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="theme-color" content="#06080D">
<meta http-equiv="Content-Security-Policy" content="default-src 'self'; connect-src 'self'; form-action 'none'; base-uri 'self'; object-src 'none'">
<title>Check your values.yaml - Stageproof</title>
<meta name="description" content="Paste your values.yaml and see which of your settings a Helm chart upgrade changes, removes, or now requires. Runs in your browser.">
%s
</head>
<body data-root="%s">
%s
<main id="main">
<section class="sub-hero">
<div class="wrap">
<p class="kicker"><span class="kicker-num">VALUES</span> Chart index</p>
<h1>Check your values.yaml</h1>
<p class="sec-lede">Pick a chart and the upgrade you plan, then paste your <code>values.yaml</code>. You see which of your settings the upgrade changes, removes, or now requires.</p>
<p class="privacy-pill">Runs in your browser. Nothing is uploaded, and Helm does not run.</p>
</div>
</section>
<div class="wrap page">
<section class="panel" aria-labelledby="step-upgrade">
<h2 class="step-title" id="step-upgrade"><span class="step-n">1</span> Choose the upgrade</h2>
<div class="field">
<div class="label-row">
<label for="chart">Chart</label>
<span id="chart-count" class="muted" aria-live="polite">Loading charts...</span>
</div>
<div class="combo">
<input id="chart" type="text" role="combobox" aria-autocomplete="list" aria-expanded="false" aria-controls="chart-list" placeholder="Type a chart name, for example traefik or cert-manager" autocomplete="off" spellcheck="false">
<ul id="chart-list" class="combo-list" role="listbox" aria-label="Charts" hidden></ul>
</div>
</div>
<div class="version-grid">
<div class="field">
<label for="from">Version you run now</label>
<select id="from" disabled><option>Pick a chart first</option></select>
</div>
<div class="arrow" aria-hidden="true">&rarr;</div>
<div class="field">
<label for="to">Version you upgrade to</label>
<select id="to" disabled><option>Pick a chart first</option></select>
</div>
</div>
<p class="muted field-help">Only upgrades this index has checked are listed. <strong>newest</strong> marks the chart's newest version, and <strong>skips versions</strong> marks a jump over the versions in between.</p>
<div id="pair-meta" class="pair-meta" hidden></div>
</section>

<section class="panel" aria-labelledby="step-values">
<div class="label-row">
<h2 class="step-title" id="step-values"><span class="step-n">2</span> <label for="yaml">Paste your values.yaml</label></h2>
<div class="toolbar">
<input id="yaml-file" type="file" accept=".yaml,.yml,text/yaml,text/plain" hidden>
<button type="button" id="load-file" class="btn btn-secondary btn-sm">Open a file</button>
<button type="button" id="clear-yaml" class="btn btn-secondary btn-sm">Clear</button>
</div>
</div>
<textarea id="yaml" spellcheck="false" placeholder="# Paste your values.yaml here, or drop a .yaml file on this box.&#10;replicaCount: 3"></textarea>
<div class="action-row">
<button type="button" id="run" class="btn btn-primary">Check</button>
<span id="key-count" class="muted">0 keys in paste</span>
<span class="hint muted">Ctrl+Enter to check</span>
</div>
<p id="status" class="status" role="status"></p>
</section>

<div id="out" class="results"></div>
</div>
</main>
%s
<script src="%s" integrity="%s" defer></script>
</body>
</html>
`, headHTML(siteJS), BasePath(), headerHTML("values"), footerHTML(version), js, sri)
}
