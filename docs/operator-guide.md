# Operator Guide

For cluster operators installing, configuring, securing, and running the k8s-nyx
controller. For authoring schedules, see the [User Guide](user-guide.md).

## Install

The Helm chart is the single supported install path, published to GHCR as an OCI
artifact.

```sh
helm install k8s-nyx oci://ghcr.io/cedricfarinazzo/k8s-nyx-chart \
  --namespace k8s-nyx-system --create-namespace
```

Pin a version with `--version X.Y.Z` (recommended for production). The chart
`version` and `appVersion` (operator image tag) are released together, so a
pinned chart pins a known-good operator image. Full values:
[chart README](../charts/k8s-nyx-chart/README.md).

## What gets deployed

| Object | Scope | Purpose |
|--------|-------|---------|
| CRD `sleepschedules.nyx.dev` | cluster | The API. Carries `resource-policy: keep`. |
| Deployment (controller-manager) | namespace | The operator. |
| ServiceAccount | namespace | Identity the operator runs as. |
| ClusterRole + binding | cluster | Watch/patch targeted workloads; manage checkpoint Secrets & wake ConfigMaps; emit Events. See [RBAC footprint](#rbac-footprint). |
| Role + binding (leader election) | namespace | Lease for HA leader election (+ Events). |
| Service + ValidatingWebhookConfiguration | cluster | Only when `webhook.enabled=true`. |

### RBAC footprint

The operator runs under a **least-privilege** ClusterRole — **no `cluster-admin`,
no wildcard (`*`) verbs, resources, or apiGroups**. Access is cluster-wide only
because targeted workloads can live in any namespace. Every granted verb is one the
controller actually calls; `list`+`watch` are kept wherever a resource is read
through the manager's informer cache (a cached `Get` is served by a `list`+`watch`
informer). The ClusterRole mirrors the generated `config/rbac/role.yaml` exactly.

| apiGroup | Resource | Verbs | Rationale |
|----------|----------|-------|-----------|
| `apps` | `deployments`, `statefulsets`, `daemonsets` | `get, list, watch, patch` | List/watch to resolve & cache targets; **patch only `/spec`** (replicas, or the `nyx.dev/asleep` nodeSelector sentinel for DaemonSets). No `update`/`create`/`delete`. |
| `batch` | `cronjobs`, `jobs` | `get, list, watch, patch` | Same; patch toggles `spec.suspend`. |
| `autoscaling` | `horizontalpodautoscalers` | `get, list, watch, patch` | Same; patch neutralizes `spec.minReplicas`/`maxReplicas`. |
| `""` (core) | `secrets` | `get, list, watch, create, update, delete` | The exact-restore checkpoint store (one Secret per schedule, operator namespace). `delete` clears a checkpoint on wake; **no `patch`**. |
| `""` (core) | `configmaps` | `get, list, watch, create, update` | The per-schedule wake-override ConfigMap. It is owner-ref garbage-collected, never operator-deleted, and never patched — so **no `delete`/`patch`**. |
| `""` (core) | `events` | `create, patch` | Lifecycle Events on workloads and SleepSchedules. Cluster-wide because an Event is written in the involved object's namespace, which can be any target namespace. |
| `nyx.dev` | `sleepschedules` | `get, list, watch` | The CR is **read-only** to the controller (reconcile watches & reads it; users author it). No `create`/`update`/`patch`/`delete`. |
| `nyx.dev` | `sleepschedules/status` | `get, update` | Status is written via the `/status` subresource only; **no `patch`**, and no `finalizers` grant (the controller sets no finalizer). |

Leader election adds a **namespaced** Role (operator namespace) granting
`coordination.k8s.io/leases` (the Lease lock) and core `events` — no ConfigMap
grant, since controller-runtime uses the Lease lock, not the legacy ConfigMap lock.

## The validating webhook

Off by default. The CRD's OpenAPI schema enforces structural validation without
it; the webhook adds checks OpenAPI can't express:

- the timezone is a real IANA zone (`time.LoadLocation`),
- `from` is strictly before `to` in every window,
- target mode and its fields are consistent (`namespaces` ⇒ a list,
  `labels` ⇒ a selector),
- `temporaryWake` durations are positive and `default ≤ max`.

### Enabling it (with cert-manager)

```sh
helm upgrade k8s-nyx oci://ghcr.io/cedricfarinazzo/k8s-nyx-chart \
  --set webhook.enabled=true
```

With `webhook.certManager.enabled=true` (default) the chart creates a self-signed
`Issuer` + `Certificate` and injects the CA into the webhook configuration, so
[cert-manager](https://cert-manager.io) must be installed. The operator reads
`ENABLE_WEBHOOKS` from this value: when the webhook is disabled it runs the
controller with `ENABLE_WEBHOOKS=false` and serves no admission endpoint.

### Enabling it without cert-manager

Set `webhook.certManager.enabled=false`, then supply the serving certificate in
the `<release>-k8s-nyx-chart-webhook-cert` Secret and set the `caBundle` on the
`ValidatingWebhookConfiguration` yourself.

## How the controller behaves

- **Evaluation** is timezone-aware and DST-correct; the binary embeds
  `time/tzdata` so `LoadLocation` works in the distroless image.
- **Requeue** happens at the soonest of the next schedule transition and the
  earliest wake-override expiry, so state changes land on time.
- **Idempotent**: a reconcile with nothing to change writes nothing (no status
  update, no patch, no event).
- **Checkpoints** live in a Secret `<schedule-name>-checkpoint` in the operator
  namespace, keyed per workload by kind + namespace + name + UID (the UID guards
  against a recreated same-named workload being restored from a stale value).
- **Leader election** (default on) ensures a single active reconciler even with
  `replicaCount > 1`.

## Observability

```sh
# operator logs
kubectl -n k8s-nyx-system logs deploy/k8s-nyx-k8s-nyx-chart -f

# schedule state
kubectl get sleepschedules -A

# scaling activity (on the workloads)
kubectl get events -A --field-selector reason=Slept
kubectl get events -A --field-selector reason=Woke
```

Health endpoints are exposed on `:8081` (`/healthz`, `/readyz`) and wired to the
pod's liveness/readiness probes.

### Audit trail

Every lifecycle action (sleep, wake, restore, wake-override, expiry) is audited
two ways:

- **Structured JSON logs** — each action logs a line with `action`, `who`
  (`k8s-nyx`), `why` (`asleep window` / `awake window` / `active wake override` /
  `wake override expired`), `when` (RFC3339), and `objectRef`
  (`Kind/namespace/name`) for correlation, plus the `sleepSchedule`. Logs are JSON
  by default (override with the `--zap-*` flags).
- **Kubernetes Events** — a corresponding Event is recorded on the
  `SleepSchedule` for each action (`kubectl describe sleepschedule <name>`), in
  addition to the per-workload `Slept`/`Woke` Events.

```sh
kubectl -n k8s-nyx-system logs deploy/k8s-nyx-k8s-nyx-chart | jq 'select(.msg=="audit")'
kubectl describe sleepschedule <name> -n <ns>   # Events: Slept / Woke / WakeEntryAccepted / WakeExpired …
```

### Prometheus metrics

The operator serves Prometheus metrics on `/metrics` (container port `8080` by
default; `--metrics-bind-address=0` disables it). Every series is labelled
`schedule` and `namespace`:

| Metric | Type | Meaning |
|--------|------|---------|
| `nyx_targets_asleep` | gauge | Targeted workloads currently asleep. |
| `nyx_targets_awake` | gauge | Targeted workloads currently awake. |
| `nyx_active_wakes` | gauge | Active (non-expired) wake override entries. |
| `nyx_override_seconds_remaining` | gauge | Seconds until the earliest active override expires (0 when none). |
| `nyx_restore_failures_total` | counter | Failed restore (wake) attempts. |

The chart ships a `ClusterIP` Service (`<release>-k8s-nyx-chart-metrics`, on by
default — disable with `metrics.service.enabled=false`) targeting the `metrics`
port, so a `ServiceMonitor` can scrape it directly; add scrape annotations via
`metrics.service.annotations`. Alternatively use a `PodMonitor` on port
`metrics`, or annotate the pod for annotation-based discovery.

Example PromQL:

```promql
# workloads asleep right now (cluster-wide)
sum(nyx_targets_asleep)

# schedules with an active override, and time left
nyx_override_seconds_remaining > 0

# restore failures in the last hour, per schedule
increase(nyx_restore_failures_total[1h])

# is a given schedule asleep?
nyx_targets_asleep{schedule="dev-hours", namespace="team-a"} > 0
```

## High availability (leader election)

For HA, run **2+ replicas** (`--set replicaCount=2`). Leader election (on by
default, `leaderElection.enabled`) ensures **exactly one** replica reconciles at
a time; the others are hot standbys.

- The leader holds a `coordination.k8s.io` **Lease** in the release namespace and
  renews it; standbys watch it. If the leader stops renewing (crash, eviction),
  a standby acquires leadership after the lease expires (`--leader-elect-lease-duration`,
  default 15s). On a **graceful** shutdown the leader releases the Lease
  immediately, so failover happens within `--leader-elect-retry-period` (≈2s).
- Tunables (operator flags): `--leader-elect-lease-duration`,
  `--leader-elect-renew-deadline`, `--leader-elect-retry-period`.
- Spread replicas across nodes/zones with the chart's `affinity` value (pod
  anti-affinity) so a single node failure can't take out every replica.

```sh
# inspect the current leader
kubectl -n k8s-nyx-system get lease k8s-nyx.nyx.dev -o yaml | grep holderIdentity
```

## Upgrades

```sh
helm upgrade k8s-nyx oci://ghcr.io/cedricfarinazzo/k8s-nyx-chart --version X.Y.Z
```

The CRD is templated with `helm.sh/resource-policy: keep`, so `helm upgrade`
updates it but `helm uninstall` does **not** delete it (your `SleepSchedule`
resources are safe). If you manage CRDs out-of-band, set `crds.install=false`.

## Verify a deployment

```sh
kubectl -n k8s-nyx-system rollout status deploy/k8s-nyx-k8s-nyx-chart
helm test k8s-nyx -n k8s-nyx-system   # operator SA lists SleepSchedules via the API
```

## Uninstall

```sh
helm uninstall k8s-nyx -n k8s-nyx-system
# CRD is kept by design; remove explicitly if intended:
# kubectl delete crd sleepschedules.nyx.dev
```

## Cost & safety notes

- Start with `spec.dryRun: true` on a new schedule to confirm the target set
  before any scaling happens.
- Use `excludeRefs` for workloads that must never sleep (databases you don't want
  scaled, billing, etc.).
- Set `temporaryWake.maxDuration` so an on-demand wake can't pin an environment
  awake indefinitely.
