# CLAUDE.md

Guidance for working in this repository.

## What this is

k8s-nyx is a Kubernetes operator (Go, controller-runtime) that puts workloads to
sleep on a schedule and wakes them on demand, restoring their exact prior state.
It scales `Deployments` and `StatefulSets` to a configured replica count during
"asleep" windows and restores the original replica count on wake. Original state
is held out-of-band so restore is exact and survives operator restarts.

The operator is the only component that mutates workloads. It touches **only**
`/spec/replicas` so it coexists cleanly with GitOps tools (e.g. ArgoCD).

## Build, test, lint

All tooling (controller-gen, setup-envtest, golangci-lint) is installed into
`./bin` by the Makefile — you don't need it preinstalled. Requires Go (version in
`go.mod`), `make`, and Docker (for the image only).

```sh
make build      # compile the manager to bin/manager
make test       # unit + envtest (spins up a local control plane via setup-envtest)
make lint       # golangci-lint
make manifests  # regenerate CRD / RBAC / webhook manifests after API changes
make generate   # regenerate deepcopy after API changes
```

Always run `make build`, `make test`, and `make lint` before opening a PR — CI
runs the same three on every pull request.

After changing anything under `api/`, run `make manifests generate` and commit the
regenerated files (`config/crd/bases/...`, `config/webhook/manifests.yaml`,
`api/.../zz_generated.deepcopy.go`, `config/rbac/role.yaml`).

## Layout

- `cmd/main.go` — manager entrypoint; wires the reconciler + validating webhook.
- `api/v1alpha1/` — the `SleepSchedule` CRD types (group `nyx.dev`, `v1alpha1`).
- `internal/controller/` — the reconciler: evaluate → resolve targets → apply
  sleep/wake → process wake overrides → update status → requeue.
- `internal/schedule/` — pure, timezone-aware evaluation: is the schedule Awake or
  Asleep now, and when does it next transition. DST-correct (wall-clock anchored).
- `internal/target/` — resolves the concrete workloads a schedule applies to
  (`namespaces` or `labels` mode, with `excludeRefs`).
- `internal/checkpoint/` — the exact-restore store: a per-schedule Secret in the
  operator namespace holding original replica counts, keyed by GVK+ns+name+UID.
- `internal/sleeper/` — scales targets to sleep and restores them on wake, using
  the checkpoint; patches only `/spec/replicas`.
- `internal/wake/` — parses and resolves entries in the per-schedule Wake
  ConfigMap (absolute RFC3339 or `+duration`, with `by`/`reason` attribution).
- `internal/webhook/v1alpha1/` — validating admission webhook (e.g. IANA timezone,
  window ordering, target consistency) for checks OpenAPI can't express.
- `config/` — kustomize bases: `crd`, `rbac`, `webhook`, `manager`, `default`.
- `.github/workflows/` — `ci.yml` (build/test/lint) and `release.yml` (tag-triggered stub).

## How the operator works

- **Schedule evaluation** (`internal/schedule`): awake windows are half-open
  `[from, to)` per weekday in the schedule's IANA timezone. Boundaries are built
  with `time.Date(..., loc)` so they stay anchored to wall-clock local time across
  DST. The reconciler requeues at the next transition.
- **Targeting** (`internal/target`): `namespaces` mode selects all in-scope
  workloads in the listed namespaces; `labels` mode selects workloads matching the
  selector cluster-wide. `excludeRefs` (kind+name) are always dropped.
- **Sleep/restore** (`internal/sleeper` + `internal/checkpoint`): on the first
  sleep, the current replica count is written to the checkpoint Secret (once — it
  is never overwritten while asleep). On wake, the exact count is restored and the
  entry cleared. The checkpoint lives in a Secret, so restore survives restarts.
- **Wake override** (`internal/wake` + reconciler): the operator owns a
  `<schedule>-wake` ConfigMap per schedule. Triggers write entries; the operator
  resolves relative `+duration`s to absolute timestamps (written back once, so they
  don't keep extending), applies a default duration, clamps to a max, deletes
  expired entries, and forces the targets awake while any entry is active
  (`status.phase = WokenByOverride`). Malformed entries are ignored and surfaced as
  Warning Events.
- **Status & events**: `status.phase` (Asleep / Awake / WokenByOverride),
  `nextTransition`, and `activeWakes` are kept current. Sleep/wake actions emit
  Events on the affected workloads. Reconcile is idempotent — it does not write
  when nothing changed.

## Conventions

- **Conventional Commits.** Commit subjects follow `type(scope): summary`
  (`feat`, `fix`, `chore`, `docs`, `ci`, `refactor`, …). Keep the body's "why"
  when it isn't obvious from the diff.
- **Only `/spec/replicas` is mutated** on workloads — never touch other fields
  (the GitOps-coexistence contract). Use a merge patch carrying just that field.
- **No silent assumptions in the API.** Anything OpenAPI can express (patterns,
  enums, required, min) lives on the CRD types as `+kubebuilder` markers; anything
  it can't (e.g. IANA timezone validity) lives in the validating webhook.
- **Timezone-aware, never UTC-assuming.** Schedule logic always loads the
  schedule's location. The binary embeds `time/tzdata` so `time.LoadLocation`
  works in the distroless image.
- **golangci-lint is pinned** (see the Makefile) to a version that understands the
  current Go toolchain's export data; bump it together with the Go version.
- **Tests:** pure logic is unit-tested with the fake client; anything needing a
  real API server (CRD install, webhook, status subresource) uses envtest. New
  behaviour ships with a test; edge cases (DST, restart-restore, malformed input)
  get their own case.

## Out of cluster

`make test` needs no cluster — envtest provides a local control plane. To run
against a real cluster, build/load the image and apply `config/default` (the
webhook needs cert wiring, which is part of the deploy/Helm packaging).
