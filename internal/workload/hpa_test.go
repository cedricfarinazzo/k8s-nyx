/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import (
	"context"
	"strings"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
)

func newHPA(ns, name string, min *int32, max int32) *autoscalingv2.HorizontalPodAutoscaler {
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID(name + "-uid")},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "web", APIVersion: "apps/v1"},
			MinReplicas:    min,
			MaxReplicas:    max,
		},
	}
}

func getHPA(t *testing.T, c client.Client, name string) *autoscalingv2.HorizontalPodAutoscaler {
	t.Helper()
	var h autoscalingv2.HorizontalPodAutoscaler
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "team-a", Name: name}, &h); err != nil {
		t.Fatalf("get hpa: %v", err)
	}
	return &h
}

func hpaRef(name string) Ref { return Ref{Kind: KindHPA, Namespace: "team-a", Name: name} }

// AC1: sleep neutralizes min/max so the HPA cannot scale up, and checkpoints the
// original bounds.
func TestHPA_SleepNeutralizesAndCheckpoints(t *testing.T) {
	s, c := newSleeper(newHPA("team-a", "web", ptr(2), 10))
	sch := schedule()
	sch.Spec.SleepReplicas = 1 // floor 1 → no clamp
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{hpaRef("web")}); err != nil {
		t.Fatal(err)
	}
	h := getHPA(t, c, "web")
	if h.Spec.MinReplicas == nil || *h.Spec.MinReplicas != 1 || h.Spec.MaxReplicas != 1 {
		t.Fatalf("neutralized min/max = %v/%d, want 1/1", h.Spec.MinReplicas, h.Spec.MaxReplicas)
	}
	key := checkpoint.Key("HorizontalPodAutoscaler", "team-a", "web", "web-uid")
	raw, found, err := s.Store.GetRaw(ctx, sch, key)
	if err != nil || !found {
		t.Fatalf("checkpoint missing (found=%v err=%v)", found, err)
	}
	if !strings.Contains(raw, `"min":2`) || !strings.Contains(raw, `"max":10`) {
		t.Fatalf("checkpoint = %s, want original {min:2,max:10}", raw)
	}
}

// AC2: wake restores the exact original min/max and clears the checkpoint.
func TestHPA_WakeRestoresExact(t *testing.T) {
	s, c := newSleeper(newHPA("team-a", "web", ptr(2), 10))
	sch := schedule()
	sch.Spec.SleepReplicas = 1
	ctx := context.Background()
	ref := hpaRef("web")

	_ = s.Apply(ctx, sch, true, []Ref{ref})
	if err := s.Apply(ctx, sch, false, []Ref{ref}); err != nil {
		t.Fatal(err)
	}
	h := getHPA(t, c, "web")
	if h.Spec.MinReplicas == nil || *h.Spec.MinReplicas != 2 || h.Spec.MaxReplicas != 10 {
		t.Fatalf("restored min/max = %v/%d, want 2/10", h.Spec.MinReplicas, h.Spec.MaxReplicas)
	}
	key := checkpoint.Key("HorizontalPodAutoscaler", "team-a", "web", "web-uid")
	if _, found, _ := s.Store.GetRaw(ctx, sch, key); found {
		t.Fatal("checkpoint should be cleared after restore")
	}
}

// AC3: a sleep floor of 0 clamps min to 1 and emits a Warning Event.
func TestHPA_ClampsScaleToZero(t *testing.T) {
	s, c := newSleeper(newHPA("team-a", "web", ptr(2), 10))
	rec := record.NewFakeRecorder(10)
	s.Recorder = rec
	sch := schedule() // SleepReplicas defaults to 0
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{hpaRef("web")}); err != nil {
		t.Fatal(err)
	}
	h := getHPA(t, c, "web")
	if h.Spec.MinReplicas == nil || *h.Spec.MinReplicas != 1 || h.Spec.MaxReplicas != 1 {
		t.Fatalf("clamped min/max = %v/%d, want 1/1", h.Spec.MinReplicas, h.Spec.MaxReplicas)
	}
	var warned bool
	for _, e := range drain(rec) {
		if strings.Contains(e, "Warning") && strings.Contains(e, "HPAScaleToZeroUnavailable") {
			warned = true
		}
	}
	if !warned {
		t.Fatal("expected a HPAScaleToZeroUnavailable Warning")
	}
}

// sleep is idempotent: a second sleep does not re-checkpoint or re-event.
func TestHPA_SleepIdempotent(t *testing.T) {
	s, _ := newSleeper(newHPA("team-a", "web", ptr(2), 10))
	rec := record.NewFakeRecorder(10)
	s.Recorder = rec
	sch := schedule()
	sch.Spec.SleepReplicas = 1
	ctx := context.Background()

	_ = s.Apply(ctx, sch, true, []Ref{hpaRef("web")})
	_ = drain(rec) // first-sleep events
	_ = s.Apply(ctx, sch, true, []Ref{hpaRef("web")})
	if ev := drain(rec); len(ev) != 0 {
		t.Fatalf("second sleep should be a no-op, got events %v", ev)
	}
	key := checkpoint.Key("HorizontalPodAutoscaler", "team-a", "web", "web-uid")
	raw, _, _ := s.Store.GetRaw(ctx, sch, key)
	if !strings.Contains(raw, `"min":2`) {
		t.Fatalf("checkpoint = %s, want original (not overwritten)", raw)
	}
}

// dry-run never mutates the HPA or writes a checkpoint.
func TestHPA_DryRun(t *testing.T) {
	s, c := newSleeper(newHPA("team-a", "web", ptr(2), 10))
	sch := schedule()
	sch.Spec.SleepReplicas = 1
	sch.Spec.DryRun = true
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{hpaRef("web")}); err != nil {
		t.Fatal(err)
	}
	h := getHPA(t, c, "web")
	if *h.Spec.MinReplicas != 2 || h.Spec.MaxReplicas != 10 {
		t.Fatalf("dry-run changed min/max to %d/%d, want 2/10", *h.Spec.MinReplicas, h.Spec.MaxReplicas)
	}
	key := checkpoint.Key("HorizontalPodAutoscaler", "team-a", "web", "web-uid")
	if _, found, _ := s.Store.GetRaw(ctx, sch, key); found {
		t.Fatal("dry-run should not write a checkpoint")
	}
}
