# stageproof/index

Nightly upgrade reports for public Helm charts. Each report is produced by the pinned Stageproof release, `v0.4.0`, checked against the chart's default values.

These reports are chart defaults only. Run Stageproof with your values files before you treat a result as a check of your cluster. Nothing here is a verdict on a cluster the index has not seen.

The site is `https://stageproof.github.io/index/`. That address is `site.BaseURL` in the Go module. Badge images that need a query string are served by the worker at `https://stageproof-index-badges.stageproof.workers.dev/badge.svg`, because GitHub Pages cannot route that query.

## Layout

`charts.yaml` is the curated chart list. `cmd/discover` writes a candidate list from Artifact Hub. `cmd/build` reads that list, chooses stable version pairs, and writes `data/v1/`.

`data/` is CC BY 4.0. See [DATA-LICENSE](DATA-LICENSE). The Go code is Apache-2.0.

## Commands

```bash
go run ./cmd/discover -n 300 -o charts.yaml
go run ./cmd/build -charts charts.yaml -o data
```

`cmd/build` downloads `stageproof` `v0.4.0` from the GitHub release and checks its SHA-256. It does not build the Stageproof `main` branch. Chart archives stay in a local cache and are not committed. A chart whose repository cannot be read is recorded in `data/v1/index.json` and the build continues with the next chart.

The curated list currently expands to about 20,000 stable pairs. That history is committed under `data/`. The nightly workflow (`.github/workflows/build.yml`, `17 4 * * *` UTC, plus `workflow_dispatch`) builds only the newest pair of each chart on Kubernetes 1.35, with the API server on. It commits new and updated JSON files under `data/v1/` back to `main`, and it rewrites `REPORT.md` from those files in the same commit. It does not commit chart archives. Older report files stay, and the catalog counts every report in that chart directory. Leave `-max-pairs` unset to build every pair the current window lists.

`-recheck` also runs pair files already stored for that chart when `stageproof_version` is not the pinned release, including files the window no longer lists. A file already at the pin, with the same chart hashes and the requested minors, is left as it is. `-shard i/n` keeps charts whose place in the id-sorted list modulo n is i. `-list-shard i/n` prints those `repo_name/chart` paths and exits. `-catalog` rewrites `data/v1/index.json` from the pair files on disk and does not run Stageproof.

`.github/workflows/rebuild.yml` is `workflow_dispatch` only. It runs 64 shards on Kubernetes 1.35, each shard uploads only its own chart directories, and one later job checks that every file is `data/v1/**/*.json`, rewrites the catalog, regenerates `REPORT.md`, and commits. It does not publish the site. The nightly schedule is unchanged. A repository that stamps one `created` day on every version stays inside the 24-month window, and files already stored are kept.

`REPORT.md` is the chart-defaults count. Regenerate it with:

```bash
go run ./cmd/report -data data
```

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
- `/badge/<repo>/<chart>/<from>/<to>.svg` is one factual image for that indexed pair. The text is `no blocking finding (defaults)`, `not checked`, or the blocking finding (`removed K8s API`, `CRD schema break`, `3 value keys removed`, or the finding's first sentence). These reports are chart defaults only.
- `/badge/registries.json` maps a Helm repository URL, without a trailing slash, and a chart name to `repo_name`. `/badge/not-indexed.svg` is the image for a pair this catalog does not have. Its text is `not indexed`.

A Renovate pull request requests `/badge.svg?chart=&from=&registry=&to=`. `chart` is the Helm chart name. For an HTTP Helm pull request that name is `depName`, and `registry` is the repository URL passed through `prBodyDefinitions`. GitHub Pages cannot choose a file from that query, and a missing file is HTTP 404 HTML. `worker/` answers the query at `https://stageproof-index-badges.stageproof.workers.dev/badge.svg`. It serves the generated SVG when the registry and chart are in `registries.json` and that pair file exists, and it returns HTTP 200 `image/svg+xml` with `not indexed` otherwise. The SVG files are published with the site. A pair that is not in the catalog answers `not indexed`.

HTML links start with `/index` because GitHub Pages serves this repository at `https://stageproof.github.io/index/`.
