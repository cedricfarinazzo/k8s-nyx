# Security Policy

## Supported versions

k8s-nyx is released from `master` with automated [semantic-release](https://github.com/semantic-release/semantic-release)
versioning. Only the **latest released version** is supported with security
fixes. Pin a chart/image version in production and upgrade promptly.

## Reporting a vulnerability

**Please do not open a public issue for security vulnerabilities.**

Report privately through GitHub's
[**Report a vulnerability**](https://github.com/cedricfarinazzo/k8s-nyx/security/advisories/new)
flow (Security → Advisories). This opens a private advisory visible only to the
maintainers.

Please include:

- a description of the issue and its impact,
- the affected version (chart/image tag or commit),
- reproduction steps or a proof of concept,
- any suggested remediation.

You can expect an initial acknowledgement within a few days. Once a fix is ready
it ships in the next release and the advisory is published with credit (unless you
prefer to remain anonymous).

## Scope notes

k8s-nyx is a cluster operator that mutates workload scheduling fields. Relevant
hardening already in place:

- runs **non-root**, read-only, all capabilities dropped (see the chart's
  `securityContext`);
- **least-privilege RBAC** — no `cluster-admin`, no wildcards; it patches only the
  single sleep field per workload kind (see
  [docs/operator-guide.md → RBAC footprint](docs/operator-guide.md#rbac-footprint));
- the validating webhook is available for stricter admission control.

Misconfigurations of the *cluster* hosting the operator (RBAC granted to other
principals, unprotected API server, etc.) are out of scope.
