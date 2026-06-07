# k8s-nyx-chart

Helm chart for the [k8s-nyx](https://github.com/cedricfarinazzo/k8s-nyx)
operator. It installs the `SleepSchedule` CRD, the operator `Deployment`, RBAC,
and (optionally) the validating admission webhook.

The chart is published to GHCR as an OCI artifact; its `version` and
`appVersion` (the operator image tag) move together with each release.

## Install

```sh
helm install k8s-nyx oci://ghcr.io/cedricfarinazzo/k8s-nyx-chart \
  --namespace k8s-nyx-system --create-namespace
```

Pin a version with `--version X.Y.Z`. See all values with:

```sh
helm show values oci://ghcr.io/cedricfarinazzo/k8s-nyx-chart
```

## What it installs

- `CustomResourceDefinition` — `sleepschedules.nyx.dev` (carries
  `helm.sh/resource-policy: keep`, so it survives `helm uninstall`).
- `Deployment` — the operator/controller-manager.
- `ServiceAccount` + `ClusterRole`/`ClusterRoleBinding` — least-privilege,
  cluster-wide: watch + patch the targeted workload kinds (Deployments,
  StatefulSets, DaemonSets, CronJobs, Jobs, HPAs); manage the per-schedule
  checkpoint Secret and wake ConfigMap; emit Events. See the
  [Operator Guide → RBAC footprint](../../docs/operator-guide.md#rbac-footprint).
- `Role`/`RoleBinding` — leader election (when enabled).
- Optional: `Service` + `ValidatingWebhookConfiguration` (+ cert-manager
  `Issuer`/`Certificate`) when `webhook.enabled=true`.

## Values

| Key | Default | Description |
|-----|---------|-------------|
| `image.repository` | `ghcr.io/cedricfarinazzo/k8s-nyx` | Operator image. |
| `image.tag` | `""` | Image tag; empty → chart `appVersion`. |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy. |
| `imagePullSecrets` | `[]` | Pull secrets for private registries. |
| `replicaCount` | `1` | Operator replicas (leader election picks one active). |
| `leaderElection.enabled` | `true` | Run leader election (keep on for HA). |
| `serviceAccount.create` | `true` | Create the ServiceAccount. |
| `serviceAccount.name` | `""` | Name to use; empty → generated. |
| `serviceAccount.annotations` | `{}` | Extra SA annotations. |
| `crds.install` | `true` | Install/update the `SleepSchedule` CRD. |
| `webhook.enabled` | `false` | Enable the validating webhook (needs TLS certs). |
| `webhook.failurePolicy` | `Fail` | Webhook failure policy. |
| `webhook.certManager.enabled` | `true` | When the webhook is on, wire a cert-manager `Issuer`/`Certificate`. |
| `podSecurityContext` | non-root, seccomp RuntimeDefault | Pod security context. |
| `securityContext` | drop ALL caps, no privilege escalation | Container security context. |
| `resources` | cpu 10m–500m / mem 64–128Mi | Operator resource requests/limits. |
| `podAnnotations` / `podLabels` | `{}` | Extra pod metadata. |
| `nodeSelector` / `tolerations` / `affinity` | `{}` / `[]` / `{}` | Scheduling controls. |

## The validating webhook

The webhook is **off by default** because it needs TLS. The CRD's OpenAPI schema
still enforces structural validation without it; the webhook only adds checks
OpenAPI can't express (valid IANA timezone, window ordering, target/mode
consistency).

To enable it:

```sh
helm upgrade k8s-nyx oci://ghcr.io/cedricfarinazzo/k8s-nyx-chart \
  --set webhook.enabled=true
```

With `webhook.certManager.enabled=true` (default) this creates a self-signed
`Issuer` and a `Certificate`, and injects the CA into the webhook configuration —
so [cert-manager](https://cert-manager.io) must be installed in the cluster. If
you set `certManager.enabled=false`, you must supply the serving cert in the
`<release>-k8s-nyx-chart-webhook-cert` Secret and the webhook `caBundle`
yourself.

## Verify

```sh
kubectl -n k8s-nyx-system rollout status deploy/<release>-k8s-nyx-chart
helm test <release> -n k8s-nyx-system   # lists SleepSchedules via the operator SA
```

## Uninstall

```sh
helm uninstall k8s-nyx -n k8s-nyx-system
```

The CRD is intentionally **kept** (resource-policy `keep`) so existing
`SleepSchedule` resources are not deleted. Remove it manually if you really mean
to: `kubectl delete crd sleepschedules.nyx.dev`.
