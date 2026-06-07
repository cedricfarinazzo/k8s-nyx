# Quickstart

Get a `SleepSchedule` sleeping and waking a workload in ~5 minutes.

## Prerequisites

- A Kubernetes cluster (v1.25+) and `kubectl`.
- [Helm](https://helm.sh) 3.8+ (OCI support).

## 1. Install the operator

```sh
helm install k8s-nyx oci://ghcr.io/cedricfarinazzo/k8s-nyx-chart \
  --namespace k8s-nyx-system --create-namespace

kubectl -n k8s-nyx-system rollout status deploy/k8s-nyx-k8s-nyx-chart
```

This installs the `SleepSchedule` CRD, RBAC, and the controller. (The validating
webhook is off by default — see the [Operator Guide](operator-guide.md) to turn
it on.)

## 2. Create a workload to manage

```sh
kubectl create namespace demo
kubectl -n demo create deployment nginx --image=nginx --replicas=3
```

## 3. Create a SleepSchedule

```yaml
# schedule.yaml
apiVersion: nyx.dev/v1alpha1
kind: SleepSchedule
metadata:
  name: demo-weekdays
  namespace: demo
spec:
  timezone: Europe/Paris
  awake:
    - days: [Mon, Tue, Wed, Thu, Fri]
      from: "09:00"
      to:   "18:00"
  target:
    mode: namespaces
    namespaces: [demo]
```

```sh
kubectl apply -f schedule.yaml
```

## 4. Watch it work

```sh
kubectl get sleepschedule -n demo
# NAME            PHASE    TIMEZONE       NEXT                   AGE
# demo-weekdays   Asleep   Europe/Paris   2026-06-08T09:00:00…   10s

kubectl get deploy nginx -n demo
# outside 09:00–18:00 Paris time, replicas scale to 0 (sleepReplicas)
```

When the schedule is `Asleep`, `nginx` is scaled to `spec.sleepReplicas` (default
`0`). At the next `from` boundary it is restored to **3** — the exact count it had
when it first went to sleep.

> **Tip — test without waiting:** set `awake` so that *now* is inside (or outside)
> a window and re-apply, or use `spec.dryRun: true` to log intended actions
> without scaling anything.

## 5. Wake it on demand

The operator creates a ConfigMap `demo-weekdays-wake` in the `demo` namespace.
Set its `wake` value to wake the targets for two hours:

```sh
kubectl -n demo patch configmap demo-weekdays-wake --type merge \
  -p '{"data":{"wake":"+2h"}}'
```

`kubectl get sleepschedule -n demo` now shows `PHASE: WokenByOverride` and
`nginx` is scaled back up. After two hours the entry expires, is removed, and the
workload returns to sleep. See the [User Guide](user-guide.md#wake-overrides) for
the full entry format and bounds.

## 6. Clean up

```sh
kubectl delete -f schedule.yaml
kubectl delete namespace demo
helm uninstall k8s-nyx -n k8s-nyx-system
```

## Next steps

- [User Guide](user-guide.md) — the full `SleepSchedule` API and wake overrides.
- [Operator Guide](operator-guide.md) — webhook, cert-manager, upgrades, RBAC.
