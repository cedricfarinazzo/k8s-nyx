# k8s-nyx

k8s-nyx is the home for all k8s-nyx work — a Kubernetes operator and its supporting tooling. This repository hosts the source, container images, and Helm chart, with the full design captured in the linked Confluence design hub.

## Distribution

Container images and the Helm chart are published to GitHub Container Registry (GHCR):

- **Image:** `ghcr.io/cedricfarinazzo/k8s-nyx`
- **Chart:** `oci://ghcr.io/cedricfarinazzo/charts/k8s-nyx`

```sh
# Pull the image
docker pull ghcr.io/cedricfarinazzo/k8s-nyx:latest

# Install the chart
helm install k8s-nyx oci://ghcr.io/cedricfarinazzo/charts/k8s-nyx
```

## License

Released under the [MIT License](LICENSE).
