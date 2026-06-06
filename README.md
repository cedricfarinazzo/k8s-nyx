# k8s-nyx

**Put Kubernetes workloads to sleep on a schedule, wake them on demand, and
restore their exact prior state — automatically.**

k8s-nyx is a Kubernetes operator that scales `Deployments` and `StatefulSets`
down during off-hours (nights, weekends) to save cost, and lets anyone wake them
on demand for a bounded window — after which they return to sleep, restored to
the **exact** replica count they had before. It touches **only**
`/spec/replicas`, so it coexists cleanly with GitOps tools like ArgoCD.

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
and asleep the rest of the time. Need it awake right now? Drop one line in the
schedule's wake ConfigMap and it wakes for a bounded period, then sleeps again.

## Why

- **Cost.** Non-production workloads sit idle most of the week. Sleeping them is
  free money back.
- **Exact restore.** The pre-sleep replica count is checkpointed out-of-band
  (in a Secret) and restored precisely on wake — survives operator restarts.
- **On-demand wake.** Anyone can wake a sleeping environment for a bounded
  window without editing the schedule, with `by`/`reason` attribution.
- **GitOps-safe.** Only `/spec/replicas` is ever patched, via a merge patch — no
  fights with ArgoCD over the rest of the manifest.
- **Timezone- & DST-correct.** Windows are anchored to wall-clock local time in
  the schedule's IANA timezone.

## How it works

```
        ┌─────────────┐   evaluate    ┌──────────────┐
        │ SleepSchedule│ ───────────▶ │  schedule    │  Awake? Asleep? next flip?
        └─────────────┘               └──────────────┘
               │ resolve targets             │
               ▼                             ▼
        ┌─────────────┐   sleep/wake  ┌──────────────┐   checkpoint
        │  workloads  │ ◀──────────── │   sleeper    │ ───────────▶ Secret
        │ (Deploy/STS)│   /spec/replicas└──────────────┘  (exact replicas)
        └─────────────┘
```

The reconciler evaluates the schedule in its timezone, resolves the targeted
workloads, scales them to sleep (checkpointing the original count once) or
restores them on wake, processes any on-demand wake overrides, and requeues at
the next transition. See [docs/](docs/) for the full picture.

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

## License

Released under the [MIT License](LICENSE).
