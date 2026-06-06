# Contributing to k8s-nyx

Thanks for your interest in contributing! This is a stub describing the basic
development flow; it will grow as the project matures.

## Prerequisites

- Go (version pinned in [`go.mod`](go.mod))
- `make`
- Docker (for building the container image)

All other tooling — `controller-gen`, `setup-envtest`, `golangci-lint` — is
installed automatically into `./bin` by the Makefile targets.

## Build, test, lint

```sh
make build      # compile the manager binary to bin/manager
make test       # run unit tests (envtest spins up a local control plane)
make lint       # run golangci-lint
make manifests  # regenerate CRD / RBAC manifests after changing API types
```

Run all three (`build`, `test`, `lint`) locally before opening a PR — CI runs
the same targets on every pull request.

## Pull request flow

1. Fork the repo and create a feature branch off `master`.
2. Make your change. If you touch the API types under `api/`, run
   `make manifests generate` and commit the regenerated files.
3. Ensure `make build`, `make test`, and `make lint` are green.
4. Open a pull request against `master` and fill in the PR template.
5. A maintainer (see [CODEOWNERS](.github/CODEOWNERS)) reviews; address review
   comments until approved, then it is merged.

## Reporting bugs / requesting features

Use the issue templates (Bug report / Feature request) on the
[Issues](https://github.com/cedricfarinazzo/k8s-nyx/issues) page.

## License

By contributing, you agree that your contributions are licensed under the
[MIT License](LICENSE).
