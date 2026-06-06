# k8s-nyx

k8s-nyx is a Kubernetes operator that puts workloads to sleep on a schedule (nights and weekends) to save cost, and lets anyone wake them on demand for a bounded period — after which they automatically return to sleep, restored to their exact prior configuration. This repository hosts the source, container image, and Helm chart. See [CLAUDE.md](CLAUDE.md) for the architecture and development workflow.

## Distribution

Container images and the Helm chart are published to GitHub Container Registry (GHCR):

- **Image:** `ghcr.io/cedricfarinazzo/k8s-nyx`
- **Chart:** `oci://ghcr.io/cedricfarinazzo/k8s-nyx-chart`

```sh
# Pull the image
docker pull ghcr.io/cedricfarinazzo/k8s-nyx:latest

# Install the chart (pin a version with --version X.Y.Z)
helm install k8s-nyx oci://ghcr.io/cedricfarinazzo/k8s-nyx-chart \
  --namespace k8s-nyx-system --create-namespace
```

The chart installs the `SleepSchedule` CRD, RBAC, and the operator Deployment.
The chart `version` and `appVersion` (the operator image tag) move together with
each release. Common overrides (`helm show values oci://.../k8s-nyx-chart` for the
full list):

- `image.tag` — defaults to the chart's `appVersion`.
- `crds.install` — set `false` to manage the CRD out-of-band. The CRD carries
  `helm.sh/resource-policy: keep`, so it survives `helm uninstall`.
- `webhook.enabled` — the validating webhook is **off by default** (it needs TLS
  certs). Set `true` to enable it; `webhook.certManager.enabled` (default `true`)
  wires a cert-manager `Issuer`/`Certificate`, so cert-manager must be installed.

## License

Released under the [MIT License](LICENSE).
