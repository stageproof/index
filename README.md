# stageproof/index

Nightly upgrade reports for public Helm charts. Each report is produced by the pinned Stageproof release, `v0.3.0`, checked against the chart's default values.

These reports are chart defaults only. Run Stageproof with your values files before you treat a result as a check of your cluster. Nothing here is a verdict on a cluster the index has not seen.

The site stays at `https://stageproof.github.io/index/` until the `stageproof.dev` domain is confirmed. That address is `site.BaseURL` in the Go module.

## Layout

`charts.yaml` is the curated chart list. `cmd/discover` writes a candidate list from Artifact Hub. `cmd/build` reads that list, chooses stable version pairs, and writes `data/v1/`.

`data/` is CC BY 4.0. See [DATA-LICENSE](DATA-LICENSE). The Go code is Apache-2.0.

## Commands

```bash
go run ./cmd/discover -n 300 -o charts.yaml
go run ./cmd/build -charts charts.yaml -o data
```

`cmd/build` downloads `stageproof` `v0.3.0` from the GitHub release and checks its SHA-256. It does not build the Stageproof `main` branch. Chart archives stay in a local cache and are not committed. A chart whose repository cannot be read is recorded in `data/v1/index.json` and the build continues with the next chart.

The curated list currently expands to about 20,000 stable pairs. That history is committed under `data/`. The nightly workflow (`.github/workflows/build.yml`, `17 4 * * *` UTC, plus `workflow_dispatch`) builds only the newest pair of each chart on Kubernetes 1.35, with the API server on. Older report files stay, and the catalog counts every report in that chart directory. Leave `-max-pairs` unset to build every pair.

## Site

`cmd/site` reads `data/v1/` and writes the HTML pages and the JSON API. The nightly workflow runs it after `cmd/build` and uploads the files for GitHub Pages. A push to `main` does not publish a site, and the generated files are not committed.

```bash
go run ./cmd/site -data data -o site
```

`-o` defaults to `site` and replaces that directory. `.` and `/` are refused.

Pages, relative to the site root:

- `/` shows the catalog totals and links to charts whose newest step is blocked. It also links to the catalog and the API.
- `/charts/` lists every chart in the HTML. Columns are the newest version, the newest step (`blocked`, `not checked`, `no blocking finding`, or `none`), the pair count, and the historical blocked count. A script on that page filters the list. There is no search server and no analytics.
- `/chart/<repo>/<chart>/` shows the versions, the newest adjacent upgrade, the step-by-step upgrades, and jumps to latest in a collapsed list. A version filter appears when there are more than 20 steps. `not checked` is not shown as a pass. The upstream repository, homepage, and release notes are links when the URL starts with `http://` or `https://`.
- `/upgrade/<repo>/<chart>/<from>/<to>/` leads with the result for each Kubernetes minor. When a check failed, or the result is `NOT CHECKED`, the page lists those checks. A pair that was not checked says "This pair could not be checked with chart defaults." It does not say there was no blocking finding. Blocking findings, warnings, defaults, CRD changes, and image changes follow. It then prints: "Checked with chart defaults only. Run the command below with your `-f values.yaml` to check your cluster." The `reproduce_command` follows, then the release notes, then a link that opens a Stageproof bug report with the repository, chart, versions, and Stageproof version filled in.
- `/api/` documents the files. `/api/v1/index.json` is a byte copy of the catalog. `/api/v1/upgrade/<repo>/<chart>/<from>/<to>.json` is a byte copy of that pair.
- `/sitemap.xml` lists `/charts/`, chart pages that have an upgrade report, and adjacent upgrades. A page that skips a version, and a chart page with no report, are marked `noindex` and left out of the sitemap.

HTML links start with `/index` because GitHub Pages serves this repository at `https://stageproof.github.io/index/`.
