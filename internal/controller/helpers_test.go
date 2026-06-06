/*
Copyright 2026.

Licensed under the MIT License.
*/

package controller

import (
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

func TestRequeueDelay(t *testing.T) {
	now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	zero := time.Time{}

	if d := requeueDelay(now, zero, zero); d != 0 {
		t.Fatalf("no targets ⇒ 0, got %s", d)
	}
	// Earliest expiry sooner than the next transition wins.
	got := requeueDelay(now, now.Add(8*time.Hour), now.Add(1*time.Hour))
	if got != time.Hour {
		t.Fatalf("expected 1h, got %s", got)
	}
	// Next transition only.
	if got := requeueDelay(now, now.Add(30*time.Minute), zero); got != 30*time.Minute {
		t.Fatalf("expected 30m, got %s", got)
	}
	// A boundary already (just) in the past is floored to 1s, not negative.
	if got := requeueDelay(now, now.Add(-time.Hour), zero); got != time.Second {
		t.Fatalf("expected floor 1s, got %s", got)
	}
}

func TestStatusChanged(t *testing.T) {
	t1 := metav1.NewTime(time.Date(2026, 6, 1, 20, 0, 0, 0, time.UTC))
	t2 := metav1.NewTime(time.Date(2026, 6, 2, 8, 0, 0, 0, time.UTC))
	base := nyxv1alpha1.SleepScheduleStatus{Phase: nyxv1alpha1.PhaseAsleep, NextTransition: &t1, ActiveWakes: 0}

	if statusChanged(base, nyxv1alpha1.PhaseAsleep, &t1, 0) {
		t.Fatal("identical status should not be changed")
	}
	if !statusChanged(base, nyxv1alpha1.PhaseAwake, &t1, 0) {
		t.Fatal("phase change should be detected")
	}
	if !statusChanged(base, nyxv1alpha1.PhaseAsleep, &t1, 1) {
		t.Fatal("activeWakes change should be detected")
	}
	if !statusChanged(base, nyxv1alpha1.PhaseAsleep, &t2, 0) {
		t.Fatal("nextTransition change should be detected")
	}
	if !statusChanged(base, nyxv1alpha1.PhaseAsleep, nil, 0) {
		t.Fatal("nextTransition set→nil should be detected")
	}
	none := nyxv1alpha1.SleepScheduleStatus{Phase: nyxv1alpha1.PhaseAsleep}
	if statusChanged(none, nyxv1alpha1.PhaseAsleep, nil, 0) {
		t.Fatal("nil→nil nextTransition should not be a change")
	}
}

func TestTemporaryWakeBounds(t *testing.T) {
	if def, max := temporaryWakeBounds(&nyxv1alpha1.SleepSchedule{}); def != 0 || max != 0 {
		t.Fatalf("nil temporaryWake ⇒ 0,0; got %s,%s", def, max)
	}
	ss := &nyxv1alpha1.SleepSchedule{Spec: nyxv1alpha1.SleepScheduleSpec{
		TemporaryWake: &nyxv1alpha1.TemporaryWake{
			DefaultDuration: metav1.Duration{Duration: time.Hour},
			MaxDuration:     metav1.Duration{Duration: 8 * time.Hour},
		},
	}}
	if def, max := temporaryWakeBounds(ss); def != time.Hour || max != 8*time.Hour {
		t.Fatalf("bounds = %s,%s", def, max)
	}
}

func TestWakeConfigMapName(t *testing.T) {
	if got := WakeConfigMapName("dev-hours"); got != "dev-hours-wake" {
		t.Fatalf("WakeConfigMapName = %q", got)
	}
}
