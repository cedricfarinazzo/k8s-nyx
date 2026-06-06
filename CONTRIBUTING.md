# Contributing to k8s-nyx

Thanks for your interest in contributing! This page covers the contribution
**policy**. For the hands-on developer workflow (build/test/codegen/release), see
the [Developer Guide](docs/contributing.md).

## Prerequisites

- Go (version pinned in [`go.mod`](go.mod))
- `make`
- Docker (for building the container image)

All other tooling — `controller-gen`, `setup-envtest`, `golangci-lint` — is
installed automatically into `./bin` by the Makefile targets.

## Conventional Commits (required)

This project uses [Conventional Commits](https://www.conventionalcommits.org) and
**automated releases** via [semantic-release](https://github.com/semantic-release/semantic-release).
Commit subjects (and PR titles — see below) **must** follow:

```
<type>(<optional scope>): <summary>
```

| Type | Use for | Release effect |
|------|---------|----------------|
| `feat` | a new user-facing capability | **minor** |
| `fix` | a bug fix | **patch** |
| `docs` | documentation only | none |
| `refactor` | code change that isn't a fix or feature | none |
| `test` | adding or fixing tests | none |
| `ci` | CI/workflow changes | none |
| `build` | build system / dependencies / Dockerfile | none |
| `chore` | maintenance | none |

A breaking change is marked with `!` after the type/scope (e.g. `feat!:`) or a
`BREAKING CHANGE:` footer, and triggers a **major** release.

Examples:

```
feat(wake): clamp overrides to temporaryWake.maxDuration
fix(target): scope excludeRefs to a namespace
docs: add operator guide
```

Keep the commit body's "why" when it isn't obvious from the diff.

**commitlint** enforces this on every PR (`.commitlintrc.yml`,
`@commitlint/config-conventional`). Non-conforming commits fail CI.

## Pull request flow

1. Fork the repo and create a branch off `master` (`feature/…`, `fix/…`, …).
2. Make your change. If you touch API types under `api/`, run
   `make manifests generate` and commit the regenerated files.
3. Ensure `make build`, `make test`, and `make lint` are green locally.
4. Open a pull request against `master`.
   - **The PR title must be a valid Conventional Commit** — PRs are
     squash-merged, so the PR title becomes the release-driving commit subject.
5. CI must pass (build, test, lint, Docker build, Helm lint/template + kind
   install, commitlint). A maintainer reviews; address comments until approved.
6. On merge to `master`, CI runs and — only if it passes — the release workflow
   may cut a new version automatically (depending on the commit types since the
   last release).

## Reporting bugs / requesting features

Use the issue templates (Bug report / Feature request) on the
[Issues](https://github.com/cedricfarinazzo/k8s-nyx/issues) page.

## License

By contributing, you agree that your contributions are licensed under the
[MIT License](LICENSE).
