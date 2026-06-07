# k8s-nyx Documentation

Start here, by role:

- **Just want it running?** → [Quickstart](quickstart.md)
- **Writing `SleepSchedule` resources?** → [User Guide](user-guide.md)
- **Installing & operating the controller?** → [Operator Guide](operator-guide.md)
- **Hacking on k8s-nyx?** → [Contributing](contributing.md)

Reference:

- [Helm chart values](../charts/k8s-nyx-chart/README.md)
- [Repository map & conventions](../CLAUDE.md)
- [Changelog](../CHANGELOG.md)

## What is k8s-nyx?

A Kubernetes operator that sleeps workloads on a schedule and wakes them on
demand, restoring their exact prior state. It handles Deployments, StatefulSets,
DaemonSets, CronJobs, Jobs, and HorizontalPodAutoscalers, sleeping each by the
smallest reversible change and patching only that one field — so it is safe
alongside GitOps tooling. See the [project README](../README.md) for the elevator
pitch.

## Concepts at a glance

| Term | Meaning |
|------|---------|
| **SleepSchedule** | The CRD (`nyx.dev/v1alpha1`) describing awake windows, targets, and wake bounds. |
| **Awake window** | A half-open `[from, to)` time range on given weekdays, in the schedule's IANA timezone. Outside all windows ⇒ asleep. |
| **Target** | The workloads a schedule acts on — by `namespaces` or by label `selector`. |
| **Checkpoint** | The out-of-band Secret storing each workload's pre-sleep state (replicas, `nodeSelector`, `spec.suspend`, or HPA bounds), for exact restore. |
| **Wake override** | An on-demand, time-bounded "stay awake now" entry in the schedule's wake ConfigMap. |
| **Phase** | `Asleep`, `Awake`, or `WokenByOverride` — surfaced in `status.phase`. |
