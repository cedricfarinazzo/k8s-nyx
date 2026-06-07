# k8s-nyx

[![CI](https://github.com/cedricfarinazzo/k8s-nyx/actions/workflows/ci.yml/badge.svg)](https://github.com/cedricfarinazzo/k8s-nyx/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/cedricfarinazzo/k8s-nyx?sort=semver)](https://github.com/cedricfarinazzo/k8s-nyx/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/cedricfarinazzo/k8s-nyx)](https://goreportcard.com/report/github.com/cedricfarinazzo/k8s-nyx)
[![Go Version](https://img.shields.io/github/go-mod/go-version/cedricfarinazzo/k8s-nyx)](go.mod)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

**Put Kubernetes workloads to sleep on a schedule, wake them on demand, and
restore their exact prior state — automatically.**

k8s-nyx is a Kubernetes operator that puts workloads to sleep during off-hours
(nights, weekends) to save cost, and lets anyone wake them on demand for a
bounded window — after which they return to sleep, restored to their **exact**
prior state. It handles `Deployments`, `StatefulSets`, `DaemonSets`, `CronJobs`,
`Jobs`, and `HorizontalPodAutoscalers`, sleeping each by the **smallest reversible
change** (replica count, a sentinel `nodeSelector`, `spec.suspend`, or HPA
bounds) and patching only that one field — so it coexists cleanly with GitOps
tools like ArgoCD.

> 📖 **Full documentation: [`docs/`](docs/README.md)** — [Quickstart](docs/quickstart.md) ·
> [User Guide](docs/user-guide.md) · [Operator Guide](docs/operator-guide.md) ·
> [Contributing](docs/contributing.md)

```yaml
apiVersion: nyx.dev/v1alpha1
kind: SleepSchedule
metadata:
  name: weekdays-9to6
  namespace: team-a
spec:
  timezone: Europe/Paris
  awake:
    - days: [Mon, Tue, Wed, Thu, Fri]
      from: "09:00"
      to:   "18:00"
  target:
    mode: namespaces
    namespaces: [team-a]
```

This keeps everything in `team-a` running 09:00–18:00 on weekdays (Paris time)
and asleep the rest of the time. Need it awake right now? See
[Wake on demand](#wake-on-demand-temporary-wake) below.

## Why

- **Cost.** Non-production workloads sit idle most of the week. Sleeping them is
  free money back.
- **Exact restore.** Each workload's pre-sleep state — replica count, original
  `nodeSelector`, `spec.suspend`, or HPA bounds — is checkpointed out-of-band (in
  a Secret) and restored precisely on wake; survives operator restarts.
- **Many kinds.** Deployments, StatefulSets, DaemonSets, CronJobs, Jobs, and
  HorizontalPodAutoscalers, each slept by a reversible, kind-appropriate
  mechanism (with a data-loss guard for `whenScaled: Delete` StatefulSets).
- **On-demand wake.** Anyone can wake a sleeping environment for a bounded window
  without editing the schedule — one value, with a max-duration safety cap.
- **GitOps-safe.** Only the one field needed to sleep each kind is patched, via a
  merge patch — no fights with ArgoCD over the rest of the manifest.
- **Observable.** Prometheus metrics, structured JSON audit logs, and Kubernetes
  Events for every sleep/wake/restore action.
- **Highly available.** Leader election keeps exactly one reconciler active; run
  2+ replicas for a hot standby.
- **Timezone- & DST-correct.** Windows are anchored to wall-clock local time in
  the schedule's IANA timezone.

## How it works

```
    ┌───────────────┐     evaluate      ┌───────────────┐
    │ SleepSchedule │ ────────────────▶ │   schedule    │   Awake? Asleep? next flip?
    └───────┬───────┘                   └───────────────┘
            │ resolve targets (per-kind handler registry)
            ▼
    ┌──────────────────────────┐  sleep/wake  ┌─────────┐   checkpoint
    │  Deploy · STS · DaemonSet  │ ◀─────────── │ sleeper │ ──────────▶ Secret
    │  CronJob · Job · HPA       │  one field   └─────────┘   (exact prior state)
    └──────────────────────────┘  per kind
```

The reconciler evaluates the schedule in its timezone, resolves the targeted
workloads through a per-kind handler registry, sleeps them by the minimal
reversible change (checkpointing the original state once) or restores them on
wake, processes any on-demand wake overrides, updates status/metrics, and
requeues at the next transition. See the [documentation](docs/README.md) for the
full picture.

## Wake on demand (temporary wake)

A sleeping environment can be woken **right now**, for a **bounded** period,
without editing the schedule — handy for an after-hours hotfix or a demo. Set the
`wake` key on the schedule's wake `ConfigMap` (`<schedule-name>-wake`, created
automatically in the schedule's namespace) to an expiry:

```sh
# wake team-a's "backoffice" schedule for 2 hours
kubectl -n team-a patch configmap backoffice-wake --type merge \
  -p '{"data":{"wake":"+2h"}}'
```

The targets come back up within a reconcile, `status.phase` becomes
`WokenByOverride`, and when the override expires the operator clears it and the
workloads return to sleep — restored exactly. The value is just the expiry: a
relative `+duration` (`+90m`, `+1h30m`), an absolute RFC3339 timestamp, or empty
(uses the schedule's `temporaryWake.defaultDuration`). Anything longer than
`temporaryWake.maxDuration` is clamped, so a forgotten wake can't pin an
environment awake forever. Cancel early by removing the key
(`-p '{"data":{"wake":null}}'`). Full details:
[User Guide → wake overrides](docs/user-guide.md#wake-overrides).

## Install

Published to GitHub Container Registry (GHCR):

- **Image:** `ghcr.io/cedricfarinazzo/k8s-nyx`
- **Chart:** `oci://ghcr.io/cedricfarinazzo/k8s-nyx-chart`

```sh
helm install k8s-nyx oci://ghcr.io/cedricfarinazzo/k8s-nyx-chart \
  --namespace k8s-nyx-system --create-namespace
```

Then create a `SleepSchedule` (see [docs/quickstart.md](docs/quickstart.md)).

## Documentation

| Guide | For |
|-------|-----|
| [Quickstart](docs/quickstart.md) | Get a schedule running in 5 minutes |
| [User Guide](docs/user-guide.md) | The `SleepSchedule` API, wake overrides, examples |
| [Operator Guide](docs/operator-guide.md) | Install, configure, secure, and operate the controller |
| [Contributing](docs/contributing.md) | Develop, test, and release k8s-nyx |
| [Chart README](charts/k8s-nyx-chart/README.md) | Helm chart values and options |
| [CLAUDE.md](CLAUDE.md) | Repository map & conventions (for humans and agents) |

## Status

`nyx.dev/v1alpha1` — early but functional. Releases are automated with
[semantic-release](https://github.com/semantic-release/semantic-release) from
Conventional Commits; see the [CHANGELOG](CHANGELOG.md).

## Contributing & community

Contributions welcome — start with the [Contributing guide](docs/contributing.md)
and the [policy](CONTRIBUTING.md) (Conventional Commits, build/test/lint, the
e2e suite). Participation is governed by the
[Code of Conduct](CODE_OF_CONDUCT.md). Found a vulnerability? Please report it
privately — see the [Security Policy](SECURITY.md).

## License

Released under the [MIT License](LICENSE).
