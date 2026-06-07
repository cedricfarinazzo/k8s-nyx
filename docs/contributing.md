# Contributing — Developer Guide

How to build, test, and release k8s-nyx. For the contribution *policy* (commit
format, PR flow, licensing) see the top-level [CONTRIBUTING.md](../CONTRIBUTING.md).

## Prerequisites

- Go (version pinned in [`go.mod`](../go.mod)).
- `make` and Docker (Docker only for building the image).
- Node.js (only if you want to run the release tooling locally; CI handles it).

All Go tooling — `controller-gen`, `setup-envtest`, `golangci-lint` — is
installed automatically into `./bin` by the Makefile at the pinned versions. You
do not need it preinstalled.

## Build, test, lint

```sh
make build      # compile the manager to bin/manager
make test       # unit + envtest (spins up a local control plane via setup-envtest)
make lint       # golangci-lint (pinned version)
make manifests  # regenerate CRD / RBAC / webhook manifests after API changes
make generate   # regenerate deepcopy after API changes
```

Run `make build test lint` before opening a PR — CI runs the same on every PR.

## After changing the API (`api/v1alpha1/`)

Regenerate and commit the generated files:

```sh
make manifests generate
```

This updates `config/crd/bases/...`, `config/webhook/manifests.yaml`,
`config/rbac/role.yaml`, and `api/.../zz_generated.deepcopy.go`. The Helm chart's
CRD (`charts/k8s-nyx-chart/templates/crds.yaml`) is derived from
`config/crd/bases` — keep it in sync when the CRD changes. CI fails if generated
files are stale.

> `config/` holds **controller-gen output only** (consumed by envtest and the
> chart). Deployment is the Helm chart, not kustomize.

## Testing strategy

- **Pure logic** (schedule, wake parsing/resolving, target resolution) is
  unit-tested with the controller-runtime fake client.
- **Anything needing a real API server** (CRD install, status subresource,
  webhook admission) uses **envtest** — `make test` provisions a local control
  plane; no cluster required.
- New behaviour ships with a test. Edge cases get their own case: DST boundaries,
  restart-and-restore, malformed wake input, namespace-scoped exclusions, etc.
- **Full-cluster end-to-end** behaviour (sleep → wake → exact restore across every
  supported kind) uses the **kind-based e2e suite** below.

## End-to-end (kind) suite

`test/e2e/` is a Ginkgo suite, build-tagged `e2e` so `make test` never picks it
up. It runs against a **real cluster** with the operator already installed, and
exercises the live behaviour: it provisions a Deployment, StatefulSet, DaemonSet,
CronJob, Job, and HPA (all on `nginx:alpine`) and asserts each one sleeps and then
wakes restored. Scenarios cover the single- and multi-namespace lifecycle, kind
targeting (`spec.kinds`), the `whenScaled=Delete` PVC-deletion guard, and the
temporary-wake override.

To keep runs short without faking time, the suite **computes the sleep window
live**: it anchors the schedule's timezone (an `Etc/GMT±N` zone) so the current
wall-clock is ~noon, then phrases an awake window a few minutes out — so a target
is asleep now and wakes at the window edge minutes later, never crossing midnight
or a weekday boundary.

```sh
# 1. a cluster with the operator installed (mirrors CI):
kind create cluster --name nyx-e2e
make docker-build IMG=k8s-nyx:dev
kind load docker-image k8s-nyx:dev --name nyx-e2e
helm install nyx charts/k8s-nyx-chart -n nyx-system --create-namespace \
  --set image.repository=k8s-nyx --set image.tag=dev --set image.pullPolicy=Never --wait

# 2. run the suite (uses the ambient KUBECONFIG):
make test-e2e
```

The waits default to the ticket's 5–7 min windows; shorten them locally by
exporting the `E2E_*` knobs (e.g. `E2E_T1_WAKE=90s E2E_T5_WINDOW=4m
E2E_T5_OVERRIDE_AT=30s make test-e2e`). In CI the `E2E (kind)` job provisions the
cluster, installs the chart, and runs the suite on every PR — it **gates merges**.

## Testing the chart

```sh
helm lint charts/k8s-nyx-chart
helm template nyx charts/k8s-nyx-chart -n nyx-system          # default
helm template nyx charts/k8s-nyx-chart -n nyx-system --set webhook.enabled=true
```

CI additionally spins up a kind cluster, builds + loads the operator image,
`helm install`s the chart, runs `helm test`, and then runs the full
[e2e suite](#end-to-end-kind-suite).

## Project layout

See [CLAUDE.md](../CLAUDE.md#project-structure) for the annotated tree. In short:
`cmd/` (entrypoint) · `api/v1alpha1/` (CRD types) · `internal/` (controller +
schedule/workload/checkpoint/wake/webhook packages) · `config/` (codegen
output) · `charts/` (Helm chart).

## Commits & releases

This repo uses **Conventional Commits**, enforced by commitlint on PRs, and
**automated releases** via semantic-release:

- `fix:` → patch, `feat:` → minor, `feat!:`/`BREAKING CHANGE:` → major.
- `docs:`, `ci:`, `chore:`, `refactor:`, `test:`, `build:` → **no release**.
- On merge to `master`, CI runs first; only if it passes does the release
  workflow run semantic-release, which computes the version, updates the
  [CHANGELOG](../CHANGELOG.md), creates the GitHub release + tag, and publishes
  the multi-arch image and Helm chart at that version.

Because PRs are squash-merged, **the PR title becomes the commit subject** — so
the PR title must be a valid Conventional Commit (e.g.
`fix(target): scope excludeRefs to a namespace`).

See [CONTRIBUTING.md](../CONTRIBUTING.md) for the full commit/PR rules.

## CI / release pipeline

- `.github/workflows/ci.yml` — on PRs and pushes to `master`: build, test, lint,
  Docker build, Helm lint/template, the `E2E (kind)` job (kind install +
  `helm test` + the e2e suite), and commitlint (PRs).
- `.github/workflows/release.yml` — triggered by a **successful CI run** on
  `master` (`workflow_run`); runs semantic-release and, only when a release is
  cut, publishes the image and chart at the computed version.

## Dependency updates (Renovate)

`renovate.json` configures automated dependency updates for Go modules, GitHub
Actions, the container base image (Dockerfile `FROM`), and Helm chart/value image
references. Two rules of the road:

- **7-day embargo.** `minimumReleaseAge: "7 days"` plus
  `internalChecksFilter: "strict"` means Renovate won't even open a PR for a new
  version until it is at least 7 days old — no day-zero adoption. Pending updates
  show up under "Pending Status Checks" on the Dependency Dashboard issue.
- **Grouped + gated.** Minor/patch/digest updates are batched into a single
  weekly PR (`before 9am on monday`); majors get their own PR. Every update PR
  runs the full CI suite (it can't merge red), and PRs are titled
  `chore(deps): …` so they cut no release.

## Local release dry-run

```sh
GITHUB_TOKEN="$(gh auth token)" npx semantic-release --dry-run
```

Shows the version and notes semantic-release would produce from the current
history, without publishing anything.
