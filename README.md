# stageproof/index

Nightly upgrade reports for public Helm charts. Each report is produced by the pinned Stageproof release, `v0.2.0`, checked against the chart's default values.

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

`cmd/build` downloads `stageproof` `v0.2.0` from the GitHub release and checks its SHA-256. It does not build the Stageproof `main` branch. Chart archives stay in a local cache and are not committed. A chart whose repository cannot be read is recorded in `data/v1/index.json` and the build continues with the next chart.

The curated list currently expands to about 20,000 stable pairs. The nightly workflow (`.github/workflows/build.yml`, `17 4 * * *` UTC, plus `workflow_dispatch`) builds the newest pair of each chart on Kubernetes 1.35, with the API server on, and keeps finished reports for the next run. Leave `-max-pairs` unset to build every pair.
