/*
Copyright 2026.

Licensed under the MIT License.
*/

// Package metrics defines the Prometheus collectors k8s-nyx exposes on the
// manager's /metrics endpoint. Every series is labelled by the SleepSchedule it
// belongs to (schedule + namespace), so operators can observe sleep/wake
// behaviour and failures per schedule.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	labelSchedule  = "schedule"
	labelNamespace = "namespace"
)

var labels = []string{labelSchedule, labelNamespace}

var (
	targetsAsleep = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nyx_targets_asleep",
		Help: "Number of targeted workloads currently asleep, per SleepSchedule.",
	}, labels)

	targetsAwake = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nyx_targets_awake",
		Help: "Number of targeted workloads currently awake, per SleepSchedule.",
	}, labels)

	activeWakes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nyx_active_wakes",
		Help: "Number of active (non-expired) wake override entries, per SleepSchedule.",
	}, labels)

	overrideSecondsRemaining = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nyx_override_seconds_remaining",
		Help: "Seconds until the earliest active wake override expires (0 when none), per SleepSchedule.",
	}, labels)

	restoreFailures = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nyx_restore_failures_total",
		Help: "Total number of failed restore (wake) attempts, per SleepSchedule.",
	}, labels)
)

func init() {
	ctrlmetrics.Registry.MustRegister(
		targetsAsleep, targetsAwake, activeWakes, overrideSecondsRemaining, restoreFailures,
	)
}

// Set records the current state of one SleepSchedule. asleep selects whether the
// targetCount counts as asleep or awake.
func Set(namespace, name string, asleep bool, targetCount, activeWakeCount int, overrideSecondsLeft float64) {
	lv := []string{name, namespace}
	if asleep {
		targetsAsleep.WithLabelValues(lv...).Set(float64(targetCount))
		targetsAwake.WithLabelValues(lv...).Set(0)
	} else {
		targetsAsleep.WithLabelValues(lv...).Set(0)
		targetsAwake.WithLabelValues(lv...).Set(float64(targetCount))
	}
	activeWakes.WithLabelValues(lv...).Set(float64(activeWakeCount))
	overrideSecondsRemaining.WithLabelValues(lv...).Set(overrideSecondsLeft)
}

// IncRestoreFailure increments the restore-failure counter for a SleepSchedule.
func IncRestoreFailure(namespace, name string) {
	restoreFailures.WithLabelValues(name, namespace).Inc()
}

// Delete drops every series for a SleepSchedule (e.g. when it is deleted), so the
// metrics do not go stale. The counter is left in place to preserve totals.
func Delete(namespace, name string) {
	lv := []string{name, namespace}
	targetsAsleep.DeleteLabelValues(lv...)
	targetsAwake.DeleteLabelValues(lv...)
	activeWakes.DeleteLabelValues(lv...)
	overrideSecondsRemaining.DeleteLabelValues(lv...)
}
