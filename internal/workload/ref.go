/*
Copyright 2026.

Licensed under the MIT License.
*/

// Package workload owns the per-kind handling of the workloads a SleepSchedule
// acts on: a registry maps each supported kind to a Handler that knows how to
// list, sleep, and restore objects of that kind. The reconciler consults
// spec.kinds and dispatches to the matching handler — kinds without a handler
// are never mutated. New kinds plug in by registering a Handler; no reconciler
// changes are needed.
package workload

// Supported workload kinds handled by the default registry. Other kinds are
// deferred to their own stories and ignored (with a Warning) until a handler is
// registered for them.
const (
	KindDeployment  = "Deployment"
	KindStatefulSet = "StatefulSet"
	KindDaemonSet   = "DaemonSet"
	KindCronJob     = "CronJob"
	KindJob         = "Job"
)

// Ref identifies a single selected workload.
type Ref struct {
	Kind      string
	Namespace string
	Name      string
}
