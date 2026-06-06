/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
)

const ssd = "ssd"

func newDaemonSet(ns, name string, sel map[string]string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID(name + "-uid")},
		Spec: appsv1.DaemonSetSpec{
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{NodeSelector: sel}},
		},
	}
}

func dsNodeSelector(t *testing.T, c client.Client, name string) map[string]string {
	t.Helper()
	var ds appsv1.DaemonSet
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "team-a", Name: name}, &ds); err != nil {
		t.Fatalf("get daemonset: %v", err)
	}
	return ds.Spec.Template.Spec.NodeSelector
}

func dsRef(name string) Ref { return Ref{Kind: KindDaemonSet, Namespace: "team-a", Name: name} }

// AC1: sleep injects an unsatisfiable sentinel and checkpoints the original.
func TestDaemonSet_SleepInjectsSentinelAndCheckpoints(t *testing.T) {
	s, c := newSleeper(newDaemonSet("team-a", "agent", map[string]string{"disktype": ssd}))
	sch := schedule()
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{dsRef("agent")}); err != nil {
		t.Fatal(err)
	}
	sel := dsNodeSelector(t, c, "agent")
	if sel[sentinelKey] != sentinelValue {
		t.Fatalf("nodeSelector %v missing sentinel %s", sel, sentinelKey)
	}
	if sel["disktype"] != ssd {
		t.Fatalf("nodeSelector %v dropped original key", sel)
	}
	key := checkpoint.Key("DaemonSet", "team-a", "agent", "agent-uid")
	raw, found, err := s.Store.GetRaw(ctx, sch, key)
	if err != nil || !found {
		t.Fatalf("checkpoint missing (found=%v err=%v)", found, err)
	}
	if !strings.Contains(raw, `"disktype":"ssd"`) || strings.Contains(raw, sentinelKey) {
		t.Fatalf("checkpoint = %s, want original nodeSelector without sentinel", raw)
	}
}

// AC2 (set): wake restores the exact original nodeSelector and removes the sentinel.
func TestDaemonSet_WakeRestoresExact(t *testing.T) {
	s, c := newSleeper(newDaemonSet("team-a", "agent", map[string]string{"disktype": ssd}))
	sch := schedule()
	ctx := context.Background()

	_ = s.Apply(ctx, sch, true, []Ref{dsRef("agent")})
	if err := s.Apply(ctx, sch, false, []Ref{dsRef("agent")}); err != nil {
		t.Fatal(err)
	}
	sel := dsNodeSelector(t, c, "agent")
	if len(sel) != 1 || sel["disktype"] != ssd {
		t.Fatalf("restored nodeSelector = %v, want {disktype: ssd}", sel)
	}
	key := checkpoint.Key("DaemonSet", "team-a", "agent", "agent-uid")
	if _, found, _ := s.Store.GetRaw(ctx, sch, key); found {
		t.Fatal("checkpoint should be cleared after restore")
	}
}

// AC2 (unset): a DaemonSet with no nodeSelector restores to no nodeSelector.
func TestDaemonSet_WakeRestoresUnset(t *testing.T) {
	s, c := newSleeper(newDaemonSet("team-a", "agent", nil))
	sch := schedule()
	ctx := context.Background()

	_ = s.Apply(ctx, sch, true, []Ref{dsRef("agent")})
	// asleep: only the sentinel present
	if sel := dsNodeSelector(t, c, "agent"); len(sel) != 1 || sel[sentinelKey] != sentinelValue {
		t.Fatalf("asleep nodeSelector = %v, want only the sentinel", sel)
	}
	_ = s.Apply(ctx, sch, false, []Ref{dsRef("agent")})
	if sel := dsNodeSelector(t, c, "agent"); len(sel) != 0 {
		t.Fatalf("restored nodeSelector = %v, want empty/unset", sel)
	}
}

// AC3: restore uses the checkpointed value, not a live value changed while asleep.
func TestDaemonSet_RestoreUsesCheckpointNotLive(t *testing.T) {
	s, c := newSleeper(newDaemonSet("team-a", "agent", map[string]string{"disktype": ssd}))
	sch := schedule()
	ctx := context.Background()

	_ = s.Apply(ctx, sch, true, []Ref{dsRef("agent")})

	// A third party rewrites the live nodeSelector while it is asleep.
	var ds appsv1.DaemonSet
	if err := c.Get(ctx, types.NamespacedName{Namespace: "team-a", Name: "agent"}, &ds); err != nil {
		t.Fatal(err)
	}
	ds.Spec.Template.Spec.NodeSelector = map[string]string{"hacked": "yes", sentinelKey: sentinelValue}
	if err := c.Update(ctx, &ds); err != nil {
		t.Fatal(err)
	}

	_ = s.Apply(ctx, sch, false, []Ref{dsRef("agent")})
	sel := dsNodeSelector(t, c, "agent")
	if len(sel) != 1 || sel["disktype"] != ssd {
		t.Fatalf("restored nodeSelector = %v, want the checkpointed {disktype: ssd}", sel)
	}
}

// sleep is idempotent: a second sleep does not re-patch or re-event.
func TestDaemonSet_SleepIdempotent(t *testing.T) {
	s, _ := newSleeper(newDaemonSet("team-a", "agent", map[string]string{"disktype": ssd}))
	rec := record.NewFakeRecorder(10)
	s.Recorder = rec
	sch := schedule()
	ctx := context.Background()

	_ = s.Apply(ctx, sch, true, []Ref{dsRef("agent")})
	if ev := drain(rec); len(ev) != 1 || !strings.Contains(ev[0], "Slept") {
		t.Fatalf("first sleep events = %v, want one Slept", ev)
	}
	_ = s.Apply(ctx, sch, true, []Ref{dsRef("agent")})
	if ev := drain(rec); len(ev) != 0 {
		t.Fatalf("second sleep should be a no-op, got events %v", ev)
	}
}

// dry-run never mutates the DaemonSet or writes a checkpoint.
func TestDaemonSet_DryRun(t *testing.T) {
	s, c := newSleeper(newDaemonSet("team-a", "agent", map[string]string{"disktype": ssd}))
	sch := schedule()
	sch.Spec.DryRun = true
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{dsRef("agent")}); err != nil {
		t.Fatal(err)
	}
	if sel := dsNodeSelector(t, c, "agent"); sel[sentinelKey] != "" {
		t.Fatalf("dry-run injected sentinel: %v", sel)
	}
	key := checkpoint.Key("DaemonSet", "team-a", "agent", "agent-uid")
	if _, found, _ := s.Store.GetRaw(ctx, sch, key); found {
		t.Fatal("dry-run should not write a checkpoint")
	}
}
