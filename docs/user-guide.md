# User Guide

For people writing `SleepSchedule` resources. For installing/operating the
controller, see the [Operator Guide](operator-guide.md).

## The SleepSchedule resource

`SleepSchedule` is namespaced (`apiVersion: nyx.dev/v1alpha1`). A schedule acts
on workloads it targets; the workloads do not have to live in the schedule's own
namespace (see [targeting](#targeting)).

```yaml
apiVersion: nyx.dev/v1alpha1
kind: SleepSchedule
metadata:
  name: backoffice
  namespace: team-a
spec:
  timezone: Europe/Paris            # IANA timezone (required)
  awake:                            # at least one window (required)
    - days: [Mon, Tue, Wed, Thu, Fri]
      from: "09:00"                 # "HH:MM", 24h
      to:   "18:00"
  target:
    mode: namespaces                # "namespaces" | "labels"
    namespaces: [team-a, team-a-jobs]
  kinds: [Deployment, StatefulSet]  # optional; default is both
  excludeRefs:                      # optional; never touch these
    - kind: Deployment
      name: critical-billing
      namespace: team-a             # optional; omit = any namespace
  sleepReplicas: 0                  # replicas while asleep (default 0)
  temporaryWake:                    # optional; bounds wake overrides
    defaultDuration: 1h
    maxDuration: 8h
  dryRun: false                     # log actions without scaling (default false)
```

### `spec` fields

| Field | Type | Notes |
|-------|------|-------|
| `timezone` | string | IANA name, e.g. `Europe/Paris`, `America/New_York`, `UTC`. Validated by the webhook when enabled. |
| `awake[]` | list (≥1) | Awake windows. Outside *all* windows the schedule is asleep. |
| `awake[].days` | enum list (≥1) | `Mon Tue Wed Thu Fri Sat Sun`. |
| `awake[].from` / `.to` | `HH:MM` | 24-hour, zero-padded. The window is **half-open `[from, to)`**, and `from` must be **strictly before** `to`. |
| `target.mode` | enum | `namespaces` or `labels`. |
| `target.namespaces` | list | Required when `mode: namespaces`. |
| `target.selector` | LabelSelector | Required when `mode: labels`; matches workloads cluster-wide. |
| `kinds` | list | Restrict to `Deployment` and/or `StatefulSet`. Empty = both. |
| `excludeRefs[]` | list | `{kind, name, namespace?}` workloads to leave untouched. A `namespace`-less ref matches that kind+name in **any** namespace. |
| `sleepReplicas` | int (≥0) | Replica count applied while asleep. Default `0`. |
| `temporaryWake.defaultDuration` | duration | Applied to a wake entry with no explicit expiry. |
| `temporaryWake.maxDuration` | duration | Safety cap; any longer wake is clamped to `now + maxDuration`. |
| `dryRun` | bool | Log intended scaling + emit `DryRun*` events without mutating. |

### `status` fields

| Field | Meaning |
|-------|---------|
| `phase` | `Asleep`, `Awake`, or `WokenByOverride`. |
| `nextTransition` | Timestamp of the next schedule flip. |
| `activeWakes` | Number of non-expired wake override entries. |

## How windows are evaluated

- Windows are **half-open `[from, to)`** on each listed weekday, anchored to
  wall-clock local time in `spec.timezone`. At exactly `to`, the schedule is
  already asleep.
- Evaluation is **DST-correct**: boundaries are computed in the schedule's
  location, so 09:00 stays 09:00 across daylight-saving shifts.
- A window does not cross midnight; `from` must be earlier than `to`. To keep a
  workload awake overnight, use a window ending at the day boundary and another
  starting the next day, or rely on a wake override.
- The controller requeues itself at the next boundary, so transitions happen on
  time without polling.

## Targeting

**`namespaces` mode** selects every in-scope workload (of the configured `kinds`)
in each listed namespace:

```yaml
target:
  mode: namespaces
  namespaces: [team-a, team-b]
```

**`labels` mode** selects workloads matching a label selector, **cluster-wide**:

```yaml
target:
  mode: labels
  selector:
    matchLabels:
      nyx.dev/sleep: "true"
```

**Exclusions** are always dropped, regardless of mode:

```yaml
excludeRefs:
  - kind: Deployment
    name: critical-billing
    namespace: team-a     # only team-a/critical-billing
  - kind: StatefulSet
    name: kafka           # any namespace named "kafka"
```

## Sleep & exact restore

The first time a workload goes to sleep, its current replica count is recorded
once in a per-schedule checkpoint Secret and **never overwritten while asleep**.
On wake, that exact count is restored and the checkpoint entry cleared. Because
the checkpoint lives in a Secret (in the operator namespace), restore is exact
and survives operator restarts.

The operator patches **only** `/spec/replicas` with a merge patch — it never
touches anything else, so ArgoCD and friends keep managing the rest of the
manifest without conflict.

## Wake overrides

To keep targets awake *now* without editing the schedule, write an entry into the
schedule's wake ConfigMap. The operator owns one ConfigMap per schedule, named
`<schedule-name>-wake`, in the schedule's namespace (it is created automatically;
your entries are never clobbered).

### Entry format

Each ConfigMap **data value** is:

```
<expiry>[;by=<who>][;reason=<text>]
```

where `<expiry>` is either:

- an absolute **RFC3339** timestamp — `2026-06-05T15:00:00Z`, or
- a relative **`+duration`** — `+2h`, `+90m`, `+1h30m`, or
- *omitted* (value is empty or only `by=`/`reason=` attributes) — the schedule's
  `temporaryWake.defaultDuration` is applied.

The data **key** is yours to choose (e.g. a ticket id); entries are processed in
key order.

```sh
# wake for 2 hours, attributed
kubectl -n team-a patch configmap backoffice-wake --type merge \
  -p '{"data":{"INC-42":"+2h;by=alice;reason=hotfix"}}'

# wake until a specific time
kubectl -n team-a patch configmap backoffice-wake --type merge \
  -p '{"data":{"release":"2026-06-05T20:00:00Z;by=release-bot"}}'

# wake for the default duration
kubectl -n team-a patch configmap backoffice-wake --type merge \
  -p '{"data":{"adhoc":"by=bob;reason=looking-into-something"}}'
```

### What the operator does with entries

- **Stamps** a relative `+duration` (or a no-expiry entry) to an absolute
  timestamp, written back **once**, so it doesn't keep extending on every
  reconcile.
- **Clamps** any expiry beyond `temporaryWake.maxDuration` down to
  `now + maxDuration` (emits a `WakeClamped` event).
- **Forces the targets awake** while any entry is active — `status.phase` becomes
  `WokenByOverride` even outside an awake window.
- **Self-cleans**: expired entries are deleted (a `WakeExpired` event is emitted),
  and the targets return to whatever the schedule says.
- **Ignores malformed entries** (bad timestamp/duration) and surfaces them as
  `MalformedWakeEntry` / `UnresolvableWakeEntry` Warning events — the rest still
  apply.

> Waking requires `temporaryWake` to be configured if you rely on the default
> duration: a no-expiry entry with no `defaultDuration` is rejected (Warning
> event) rather than waking forever.

## Dry run

Set `spec.dryRun: true` to see what the operator *would* do without changing any
replica counts. It logs the intended actions and emits `DryRunSlept` /
`DryRunWoke` events on the affected workloads. Useful when rolling out a new
schedule.

## Events to watch

```sh
kubectl get events -n team-a --field-selector reason=Slept
kubectl get events -n team-a --field-selector reason=Woke
```

The operator emits `Slept` / `Woke` on workloads it scales (and the `DryRun*`
variants), plus `WakeClamped` / `WakeExpired` / `MalformedWakeEntry` on the
SleepSchedule for wake-override activity.
