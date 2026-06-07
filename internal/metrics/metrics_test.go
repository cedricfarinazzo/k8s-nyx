/*
Copyright 2026.

Licensed under the MIT License.
*/

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// AC1/AC2: Set publishes the gauges for one schedule under schedule+namespace
// labels; asleep selects which of asleep/awake carries the target count.
func TestSet_Asleep(t *testing.T) {
	Set("team-a", "dev-asleep", true, 3, 1, 42)

	g := targetsAsleep.WithLabelValues("dev-asleep", "team-a")
	if v := testutil.ToFloat64(g); v != 3 {
		t.Fatalf("targets_asleep = %v, want 3", v)
	}
	if v := testutil.ToFloat64(targetsAwake.WithLabelValues("dev-asleep", "team-a")); v != 0 {
		t.Fatalf("targets_awake = %v, want 0", v)
	}
	if v := testutil.ToFloat64(activeWakes.WithLabelValues("dev-asleep", "team-a")); v != 1 {
		t.Fatalf("active_wakes = %v, want 1", v)
	}
	if v := testutil.ToFloat64(overrideSecondsRemaining.WithLabelValues("dev-asleep", "team-a")); v != 42 {
		t.Fatalf("override_seconds_remaining = %v, want 42", v)
	}
}

func TestSet_Awake(t *testing.T) {
	Set("team-b", "dev-awake", false, 5, 0, 0)
	if v := testutil.ToFloat64(targetsAwake.WithLabelValues("dev-awake", "team-b")); v != 5 {
		t.Fatalf("targets_awake = %v, want 5", v)
	}
	if v := testutil.ToFloat64(targetsAsleep.WithLabelValues("dev-awake", "team-b")); v != 0 {
		t.Fatalf("targets_asleep = %v, want 0", v)
	}
}

// AC1: the restore-failure counter increments.
func TestIncRestoreFailure(t *testing.T) {
	IncRestoreFailure("team-a", "dev-fail")
	IncRestoreFailure("team-a", "dev-fail")
	if v := testutil.ToFloat64(restoreFailures.WithLabelValues("dev-fail", "team-a")); v != 2 {
		t.Fatalf("restore_failures_total = %v, want 2", v)
	}
}

// AC2: a series is addressable by its schedule + namespace labels (and only
// those two), which WithLabelValues enforces — so the metric carries exactly the
// schedule/namespace labels.
func TestSet_Labels(t *testing.T) {
	Set("prod", "billing", true, 7, 0, 0)
	if v := testutil.ToFloat64(targetsAsleep.WithLabelValues("billing", "prod")); v != 7 {
		t.Fatalf("targets_asleep{schedule=billing,namespace=prod} = %v, want 7", v)
	}
}

// Delete drops the gauge series for a schedule (no stale metrics).
func TestDelete(t *testing.T) {
	Set("team-c", "gone", true, 2, 0, 0)
	before := testutil.CollectAndCount(targetsAsleep)
	Delete("team-c", "gone")
	after := testutil.CollectAndCount(targetsAsleep)
	if after != before-1 {
		t.Fatalf("targets_asleep series count = %d after delete, want %d", after, before-1)
	}
}
