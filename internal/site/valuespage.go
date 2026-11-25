package site

import (
	"crypto/sha512"
	"embed"
	"encoding/base64"
	"fmt"
	"net/url"
)

//go:embed assets/values.js assets/values.css
var valueAssets embed.FS

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
<a href="%s">Your values</a>
</header>
<main>
<h1>Check your values file against a stored upgrade</h1>
<p class="notice">The YAML you paste stays in this browser. This page does not upload it, and it does not run Helm.</p>
<p>A stored report is that chart's own defaults. It records a values key only when the check found one. Dropped, renamed, and changed-default keys are listed when the report has those findings. A required key the defaults leave unset is listed too, and this page says whether your paste sets it. A key that only your file sets, and that this report did not record, is not compared.</p>
<p>The reader handles block YAML, quoted keys, anchors, and lists of maps. A key it cannot read is treated as not set. Run Stageproof on the file before you rely on the result.</p>
<label for="chart-search">Find a chart</label>
<input id="chart-search" type="search" autocomplete="off">
<label for="chart">Chart</label>
<select id="chart"></select>
<label for="from">From</label>
<input id="from" autocomplete="off">
<label for="to">To</label>
<input id="to" autocomplete="off">
<label for="yaml">Values YAML</label>
<textarea id="yaml" spellcheck="false"></textarea>
<p><button type="button" id="run">Compare</button></p>
<p id="status"></p>
<div id="out"></div>
</main>
<footer>
<p>Chart defaults only. Run Stageproof with your values files before you treat a result as a check of your cluster.</p>
</footer>
<script src="%s" integrity="%s" defer></script>
</body>
</html>
`, css, BasePath(), Href(Root), Href(Charts), Href(API), Href(Values), js, sri)
}
