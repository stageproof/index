# Stageproof index report

These numbers are chart defaults only. They are not a check of any cluster's values files. Regenerate with `go run ./cmd/report -data data`.

Catalog charts: 300. Charts with a stored pair: 294. Upgrade pairs: 20488. Blocked: 2115. Not checked: 763. Passed: 17610.

Catalog charts with no stored pair:

- `bitnami-labs/sealed-secrets`: https://bitnami-labs.github.io/sealed-secrets/index.yaml: 404 Not Found
- `k8s-dashboard/kubernetes-dashboard`: https://kubernetes.github.io/dashboard/index.yaml: 404 Not Found
- `minio/minio`: chart minio is not in https://helm.min.io/
- `mysql-operator/mysql-operator`
- `twuni/docker-registry`: Get "https://helm.twun.io/index.yaml": EOF
- `wso2/mysql`

A pair is blocked when any Kubernetes minor in the file is BLOCKED. Otherwise a pair with any NOT CHECKED minor is not checked. The rest are passed.

## Stageproof version

- `v0.3.0`: 20182
- `v0.3.1`: 306

## Blocker rules

Pairs is the number of upgrade files that contain the rule. Findings counts each blocker in those files, so one upgrade can add more than one.

| Rule | Pairs | Findings |
|---|---:|---:|
| `k8s.api.removed` | 983 | 4830 |
| `k8s.apply.immutable_field` | 545 | 712 |
| `crd.not_upgraded_by_helm` | 308 | 1057 |
| `helm.template.render_failed` | 136 | 136 |
| `k8s.upgrade.immutable_selector_changed` | 114 | 170 |
| `k8s.apply.invalid` | 93 | 396 |
| `impact.delete.crd` | 59 | 347 |
| `supply.image.not_found` | 51 | 65 |
| `helm.values.new_required_unset` | 33 | 41 |
| `impact.delete.pvc` | 27 | 29 |
| `crd.existing_object_invalid` | 9 | 32 |
| `impact.delete.namespace` | 5 | 9 |
| `impact.delete.statefulset` | 1 | 1 |

## One stored command per rule

Each command is the `reproduce_command` stored in that index file. This report copies it. It does not run it again. The message is the first blocker finding for that rule in the file.

### `k8s.api.removed`

`artifact-hub/artifact-hub` 1.10.0 to 1.24.0, report `v0.3.0`.

API `batch/v1beta1` (CronJob) is not served by Kubernetes 1.35.0.

```
stageproof check --repo https://artifacthub.github.io/helm-charts/ --chart artifact-hub --from 1.10.0 --to 1.24.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `k8s.apply.immutable_field`

`apache/pulsar` 3.3.0 to 4.8.0, report `v0.3.0`.

Kubernetes rejected the update. `spec` is immutable.

```
stageproof check --repo https://pulsar.apache.org/charts --chart pulsar --from 3.3.0 --to 4.8.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `crd.not_upgraded_by_helm`

`actions-runner-controller/actions-runner-controller` 0.17.1 to 0.23.7, report `v0.3.0`.

The CRD runnerdeployments.actions.summerwind.dev under crds/ changed (spec.template.spec.dnsConfig type, spec.template.spec.dnsConfig.items removed, spec.template.spec.dnsConfig.items type, spec.template.spec.dnsConfig.items.nameservers removed, spec.template.spec.dnsConfig.items.nameservers type, spec.template.spec.dnsConfig.items.nameservers.items removed, spec.template.spec.dnsConfig.items.nameservers.items type, spec.template.spec.dnsConfig.items.options removed, spec.template.spec.dnsConfig.items.options type, spec.template.spec.dnsConfig.items.options.items removed, spec.template.spec.dnsConfig.items.options.items type, spec.template.spec.dnsConfig.items.options.items.name removed, spec.template.spec.dnsConfig.items.options.items.name type, spec.template.spec.dnsConfig.items.options.items.value removed, spec.template.spec.dnsConfig.items.options.items.value type, spec.template.spec.dnsConfig.items.searches removed, spec.template.spec.dnsConfig.items.searches type, spec.template.spec.dnsConfig.items.searches.items removed, spec.template.spec.dnsConfig.items.searches.items type, spec.template.spec.topologySpreadConstraint removed, spec.template.spec.topologySpreadConstraint type, spec.template.spec.topologySpreadConstraint.items removed, spec.template.spec.topologySpreadConstraint.items type, spec.template.spec.topologySpreadConstraint.items.labelSelector removed, spec.template.spec.topologySpreadConstraint.items.labelSelector type, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchExpressions removed, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchExpressions type, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchExpressions.items removed, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchExpressions.items type, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchExpressions.items.key removed, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchExpressions.items.key type, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchExpressions.items.operator removed, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchExpressions.items.operator type, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchExpressions.items.values removed, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchExpressions.items.values type, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchExpressions.items.values.items removed, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchExpressions.items.values.items type, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchLabels removed, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchLabels type, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchLabels.additionalProperties removed, spec.template.spec.topologySpreadConstraint.items.labelSelector.matchLabels.additionalProperties type, spec.template.spec.topologySpreadConstraint.items.maxSkew removed, spec.template.spec.topologySpreadConstraint.items.maxSkew type, spec.template.spec.topologySpreadConstraint.items.topologyKey removed, spec.template.spec.topologySpreadConstraint.items.topologyKey type, spec.template.spec.topologySpreadConstraint.items.whenUnsatisfiable removed, spec.template.spec.topologySpreadConstraint.items.whenUnsatisfiable type). Plain Helm does not upgrade CRDs in that directory. helm upgrade leaves the CRD already in the cluster as it is, and CRDs are not installed on upgrade or rollback.

```
stageproof check --repo https://actions-runner-controller.github.io/actions-runner-controller --chart actions-runner-controller --from 0.17.1 --to 0.23.7 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `helm.template.render_failed`

`adwerx/awx` 2.0.1 to 3.4.3, report `v0.3.0`.

Candidate chart (`--to`) failed `helm template` rendering.

```
stageproof check --repo https://adwerx.github.io/charts/ --chart awx --from 2.0.1 --to 3.4.3 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `k8s.upgrade.immutable_selector_changed`

`apache-airflow/airflow` 1.4.0 to 1.22.0, report `v0.3.0`.

Immutable `spec.selector.matchLabels` changed from map[app.kubernetes.io/instance:stageproof-rel app.kubernetes.io/name:postgresql role:primary] to map[app.kubernetes.io/component:primary app.kubernetes.io/instance:stageproof-rel app.kubernetes.io/name:postgresql]. An in-place `helm upgrade` will fail.

```
stageproof check --repo https://airflow.apache.org/ --chart airflow --from 1.4.0 --to 1.22.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `k8s.apply.invalid`

`apache/pulsar` 3.3.0 to 4.8.0, report `v0.3.0`.

Kubernetes rejected `metadata.name` on Service/kube-system/stageproof-rel-victoria-metrics-k8s-stack-kube-controller-manager.

```
stageproof check --repo https://pulsar.apache.org/charts --chart pulsar --from 3.3.0 --to 4.8.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `impact.delete.crd`

`apache/pulsar` 3.3.0 to 4.8.0, report `v0.3.0`.

Helm will delete CustomResourceDefinition/default/alertmanagerconfigs.monitoring.coreos.com. A templated custom resource definition that the new chart does not render is removed, and Kubernetes then deletes every object of that kind.

```
stageproof check --repo https://pulsar.apache.org/charts --chart pulsar --from 3.3.0 --to 4.8.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `supply.image.not_found`

`airflow-helm/airflow` 8.8.0 to 8.9.0, report `v0.3.1`.

Image `docker.io/bitnami/redis:6.2.14-debian-12-r17` was not found.

```
stageproof check --repo https://airflow-helm.github.io/charts --chart airflow --from 8.8.0 --to 8.9.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `helm.values.new_required_unset`

`gitlab/gitlab` 9.11.16 to 10.0.0, report `v0.3.0`.

The new chart does not render because a required value is unset.

```
stageproof check --repo http://charts.gitlab.io/ --chart gitlab --from 9.11.16 --to 10.0.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `impact.delete.pvc`

`datawire/ambassador` 6.4.3 to 6.4.4, report `v0.3.0`.

Helm will delete PersistentVolumeClaim/stageproof-check/stageproof-rel-ambassador-registry-data. The data on that claim goes with it.

```
stageproof check --repo https://getambassador.io/ --chart ambassador --from 6.4.3 --to 6.4.4 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `crd.existing_object_invalid`

`fluxcd-community/flux2` 2.13.0 to 2.20.0, report `v0.3.0`.

CustomResourceDefinition/default/buckets.source.toolkit.fluxcd.io no longer includes a storage version (stored versions removed : [v1beta2]). Objects stored as that version cannot be read.

```
stageproof check --repo https://fluxcd-community.github.io/helm-charts --chart flux2 --from 2.13.0 --to 2.20.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `impact.delete.namespace`

`eclipse-che/eclipse-che` 7.49.0 to 7.50.0, report `v0.3.0`.

Helm will delete Namespace/default/eclipse-che. Every object in that namespace is deleted with it.

```
stageproof check --repo https://eclipse-che.github.io/che-operator/charts --chart eclipse-che --from 7.49.0 --to 7.50.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

### `impact.delete.statefulset`

`grafana-community/loki` 11.7.0 to 12.0.0, report `v0.3.0`.

Helm will delete StatefulSet/stageproof-check/loki-backend. persistentVolumeClaimRetentionPolicy.whenDeleted is Delete, so the claims created for it are deleted too.

```
stageproof check --repo https://grafana-community.github.io/helm-charts/ --chart loki --from 11.7.0 --to 12.0.0 --values-proof full --current-k8s 1.35 --api-server auto --format json
```

## Accuracy corpus

This command does not re-run the accuracy corpus. The measurement below is from `docs/accuracy.md` in `stageproof/stageproof`, dated 2026-11-22, commit `042579b`, workflow `45065690496`, on `ubuntu-latest` with Go 1.27.4, Helm 3.22.1, and Kubernetes 1.35.0.

- 12 of 12 incidents caught, 12 of 12 controls passed.
- 316 unique pairs across 105 CNCF projects. 281 were labeled clean and 35 were labeled breaks.
- Labeled-clean wrong blocks: 0 of 281 with `--api-server off`, and 0 of 281 with `--api-server auto`.
- File check latency: p50 0.74s, p95 4.13s, max 8.07s. API server proof: p50 5.19s, p95 15.41s.
- Flux replay: 70 of 70 across 5 public repositories, 62 passed, 8 blocked, 0 not checked, 0 errors. p50 3.53s, p95 25.04s.

Reproduce that measurement from the Stageproof repository:

```
git checkout 042579b
go run ./cmd/corpus run --set incidents --out incidents.json
go run ./cmd/corpus run --set pairs --api-server off --out pairs-files.json
go run ./cmd/corpus run --set pairs --api-server auto --out pairs.json
go run ./cmd/corpus compare incidents.json corpus/baseline.json
go run ./cmd/corpus compare pairs-files.json corpus/baseline.json
go run ./cmd/corpus compare pairs.json corpus/baseline.json
go run ./cmd/corpus run --set replay --out replay.json
```
