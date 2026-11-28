# Stageproof index report

These numbers are chart defaults only. They are not a check of any cluster's values files. Regenerate with `go run ./cmd/report -data data`.

The stored files were checked against Kubernetes 1.35. A removed API is a finding about that minor.

Newest adjacent upgrade: 10 charts blocked, 4 not checked, 280 passed, 0 with no adjacent step. That is 294 charts with a stored pair.

300 charts are listed. 294 have a stored pair.

Catalog charts with no stored pair:

- `bitnami-labs/sealed-secrets`: https://bitnami-labs.github.io/sealed-secrets/index.yaml: 404 Not Found
- `k8s-dashboard/kubernetes-dashboard`: https://kubernetes.github.io/dashboard/index.yaml: 404 Not Found
- `minio/minio`: chart minio is not in https://helm.min.io/
- `mysql-operator/mysql-operator`
- `twuni/docker-registry`: Get "https://helm.twun.io/index.yaml": dial tcp: lookup helm.twun.io on 127.0.0.53:53: no such host
- `wso2/mysql`

Upgrade pairs: 21859. Blocked: 1857. Not checked: 818. Passed: 19184.

A pair is blocked when any Kubernetes minor in the file is BLOCKED. Otherwise a pair with any NOT CHECKED minor is not checked. The rest are passed.

## Chart versions absent from the upstream index

A stored pair names a version that is not in that chart's `meta.json` versions list. The upstream index dropped or renamed that version string. The file is kept.

- `rook/rook-ceph` version `v1.21.2` is named by 1 stored pair
- `rook/rook-ceph-cluster` version `v1.21.2` is named by 1 stored pair

## Why blocked pairs are blocked

A chart-defaults render problem is `helm.template.render_failed`, `helm.values.new_required_unset`, or `k8s.apply.invalid`. An upgrade hazard is any other blocker, including a delete, an immutable field, a CRD change, or a missing image. Mixed means the file has blockers from more than one of those groups.

| Cause | Pairs |
|---|---:|
| only a removed API | 647 |
| only a chart-defaults render problem | 167 |
| an upgrade hazard | 935 |
| more than one of those | 108 |

## Stageproof version

- `v0.4.0`: 19366
- `v0.5.1`: 1656
- `v0.5.2`: 666
- `v0.5.0`: 171

## Blocker rules

Pairs is the number of upgrade files that contain the rule. Findings counts each blocker in those files, so one upgrade can add more than one.

| Rule | Pairs | Findings |
|---|---:|---:|
| `k8s.api.removed` | 688 | 3284 |
| `crd.not_upgraded_by_helm` | 378 | 1406 |
| `k8s.apply.immutable_field` | 337 | 423 |
| `supply.image.not_found` | 258 | 400 |
| `k8s.upgrade.immutable_selector_changed` | 115 | 174 |
| `helm.template.render_failed` | 108 | 108 |
| `k8s.apply.invalid` | 93 | 396 |
| `impact.delete.crd` | 61 | 351 |
| `helm.values.new_required_unset` | 34 | 42 |
| `impact.delete.pvc` | 27 | 29 |
| `crd.existing_object_invalid` | 9 | 32 |
| `impact.delete.namespace` | 5 | 9 |
| `impact.delete.statefulset` | 1 | 1 |

## One stored command per rule

Each command is the `reproduce_command` stored in that index file. This report copies it. It does not run it again. The pair is the one with the newest target version for that rule. The message is the first blocker finding for that rule in the file.

### `k8s.api.removed`

`teleport-agent-kube/teleport-cluster` 10.2.0 to 10.2.1, report `v0.4.0`.

API `policy/v1beta1` (PodSecurityPolicy) is not served by Kubernetes 1.35.0.

```
stageproof check --repo https://charts.releases.teleport.dev --chart teleport-cluster --from 10.2.0 --to 10.2.1 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `crd.not_upgraded_by_helm`

`prometheus-community/kube-prometheus-stack` 92.22.0 to 93.9.0, report `v0.4.0`.

The CRD alertmanagers.monitoring.coreos.com under crds/ changed (spec.clusterTLS.server.clientAuthType enum, spec.web.tlsConfig.clientAuthType enum). Plain Helm does not upgrade CRDs in that directory. helm upgrade leaves the CRD already in the cluster as it is, and CRDs are not installed on upgrade or rollback.

```
stageproof check --repo https://prometheus-community.github.io/helm-charts --chart kube-prometheus-stack --from 92.22.0 --to 93.9.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `k8s.apply.immutable_field`

`jfrog/artifactory` 107.104.17 to 107.111.4, report `v0.5.1`.

Kubernetes rejected the update. `spec` is immutable.

```
stageproof check --repo https://charts.jfrog.io --chart artifactory --from 107.104.17 --to 107.111.4 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `supply.image.not_found`

`sonarqube/sonarqube` 2025.1.8 to 2025.2.0, report `v0.4.0`.

Image `docker.io/bitnami/postgresql:11.14.0-debian-10-r22` was not found.

```
stageproof check --repo https://SonarSource.github.io/helm-chart-sonarqube --chart sonarqube --from 2025.1.8 --to 2025.2.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `k8s.upgrade.immutable_selector_changed`

`jfrog/artifactory` 107.104.17 to 107.111.4, report `v0.5.1`.

Immutable `spec.selector.matchLabels` changed from map[app.kubernetes.io/instance:stageproof-rel app.kubernetes.io/name:postgresql role:primary] to map[app.kubernetes.io/component:primary app.kubernetes.io/instance:stageproof-rel app.kubernetes.io/name:postgresql]. An in-place `helm upgrade` will fail.

```
stageproof check --repo https://charts.jfrog.io --chart artifactory --from 107.104.17 --to 107.111.4 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `helm.template.render_failed`

`sonarqube/sonarqube` 10.8.1 to 2025.1.0, report `v0.4.0`.

Candidate chart (`--to`) failed `helm template` rendering.

```
stageproof check --repo https://SonarSource.github.io/helm-chart-sonarqube --chart sonarqube --from 10.8.1 --to 2025.1.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `k8s.apply.invalid`

`sonatype/nxrm-ha` 70.1.0 to 71.0.0, report `v0.4.0`.

Kubernetes rejected `spec.updateStrategy.rollingUpdate` on StatefulSet/nexusrepo/stageproof-rel-nxrm-ha.

```
stageproof check --repo https://sonatype.github.io/helm3-charts/ --chart nxrm-ha --from 70.1.0 --to 71.0.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `impact.delete.crd`

`minio-operator/operator` 6.0.4 to 7.1.1, report `v0.4.0`.

Helm will delete CustomResourceDefinition/default/miniojobs.job.min.io. A templated custom resource definition that the new chart does not render is removed, and Kubernetes then deletes every object of that kind.

```
stageproof check --repo https://operator.min.io --chart operator --from 6.0.4 --to 7.1.1 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `helm.values.new_required_unset`

`teleport-agent-kube/teleport-cluster` 11.3.27 to 12.0.1, report `v0.4.0`.

Upgraded chart introduces a NEW `required` value `auth.teleportConfig` that is unset in your values.

```
stageproof check --repo https://charts.releases.teleport.dev --chart teleport-cluster --from 11.3.27 --to 12.0.1 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `impact.delete.pvc`

`open-webui/open-webui` 12.0.0 to 12.0.1, report `v0.4.0`.

replaced: delete then create. Helm will delete PersistentVolumeClaim/stageproof-check/stageproof-rel. The data on that claim goes with it. PersistentVolumeClaim/stageproof-check/stageproof-rel-pipelines is created in its place.

```
stageproof check --repo https://helm.openwebui.com --chart open-webui --from 12.0.0 --to 12.0.1 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `crd.existing_object_invalid`

`fluxcd-community/flux2` 2.16.4 to 2.20.0, report `v0.4.0`.

CustomResourceDefinition/default/imagepolicies.image.toolkit.fluxcd.io no longer includes a storage version (stored versions removed : [v1beta2]). Objects stored as that version cannot be read.

```
stageproof check --repo https://fluxcd-community.github.io/helm-charts --chart flux2 --from 2.16.4 --to 2.20.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `impact.delete.namespace`

`eclipse-che/eclipse-che` 7.49.0 to 7.50.0, report `v0.4.0`.

Helm will delete Namespace/default/eclipse-che. Every object in that namespace is deleted with it.

```
stageproof check --repo https://eclipse-che.github.io/che-operator/charts --chart eclipse-che --from 7.49.0 --to 7.50.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `impact.delete.statefulset`

`grafana-community/loki` 11.7.0 to 12.0.0, report `v0.4.0`.

Helm will delete StatefulSet/stageproof-check/loki-backend. persistentVolumeClaimRetentionPolicy.whenDeleted is Delete, so the claims created for it are deleted too.

```
stageproof check --repo https://grafana-community.github.io/helm-charts/ --chart loki --from 11.7.0 --to 12.0.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

## Accuracy corpus

This command does not re-run the accuracy corpus. The lines below are copied from `docs/accuracy.md` in `stageproof/stageproof`, dated 2026-11-25, commit `c3ec5b051d568f1a41081628d2c591bed54c39be`, workflow `45519144615`, on ubuntu-latest, Go 1.27.4, Helm v3.22.1, Kubernetes 1.35.0.

- 18 of 18 incidents caught, 18 of 18 controls passed.
- 316 unique pairs across 105 CNCF projects. 283 were labeled clean and 33 were labeled breaks.
- Labeled-clean wrong blocks: 0 of 283 with --api-server off, and 0 of 283 with --api-server auto.
- File check latency: p50 0.92s, p95 3.47s, max 6.47s. API server proof: p50 5.42s, p95 13.71s, max 66.23s.
- This run did not execute the Flux replay. The last local replay, on 042579b, checked 70 of 70 and is not part of workflow run 45519144615.

Reproduce that measurement from the Stageproof repository:

```
git checkout c3ec5b051d568f1a41081628d2c591bed54c39be
go run ./cmd/corpus run --set incidents --out incidents.json
go run ./cmd/corpus run --set pairs --api-server off --out pairs-files.json
go run ./cmd/corpus run --set pairs --api-server auto --out pairs.json
go run ./cmd/corpus compare incidents.json corpus/baseline.json
go run ./cmd/corpus compare pairs-files.json corpus/baseline.json
go run ./cmd/corpus compare pairs.json corpus/baseline.json
```
