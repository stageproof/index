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

`cmd/build` downloads `stageproof` `v0.2.0` from the GitHub release and checks its SHA-256. It does not build the Stageproof `main` branch. Chart archives stay in a local cache and are not committed. `-max-pairs 1` builds only the newest pair of each chart. The nightly run leaves that flag unset and builds every pair.

The nightly schedule is not turned on until the chart list in `charts.yaml` has been reviewed. Until then, `.github/workflows/build.yml` starts only with `workflow_dispatch`.
