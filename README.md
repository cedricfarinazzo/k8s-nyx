# k8s-nyx

k8s-nyx is a Kubernetes operator that puts workloads to sleep on a schedule (nights and weekends) to save cost, and lets anyone wake them on demand for a bounded period — after which they automatically return to sleep, restored to their exact prior configuration. This repository hosts the source, container image, and Helm chart. See [CLAUDE.md](CLAUDE.md) for the architecture and development workflow.

## Distribution

Container images and the Helm chart are published to GitHub Container Registry (GHCR):

- **Image:** `ghcr.io/cedricfarinazzo/k8s-nyx`
- **Chart:** `oci://ghcr.io/cedricfarinazzo/k8s-nyx-chart`

```sh
# Pull the image
docker pull ghcr.io/cedricfarinazzo/k8s-nyx:latest

# Install the chart
helm install k8s-nyx oci://ghcr.io/cedricfarinazzo/k8s-nyx-chart
```

## License

Released under the [MIT License](LICENSE).
