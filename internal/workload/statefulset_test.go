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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
)

func newSTSWithPolicy(ns, name string, replicas int32, whenScaled appsv1.PersistentVolumeClaimRetentionPolicyType, annos map[string]string) *appsv1.StatefulSet {
	s := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID(name + "-uid"), Annotations: annos},
		Spec:       appsv1.StatefulSetSpec{Replicas: ptr(replicas)},
	}
	if whenScaled != "" {
		s.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
			WhenScaled: whenScaled,
		}
	}
	return s
}

func stsReplicas(t *testing.T, c client.Client, name string) int32 {
	t.Helper()
	var s appsv1.StatefulSet
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "team-a", Name: name}, &s); err != nil {
		t.Fatalf("get statefulset: %v", err)
	}
	return *s.Spec.Replicas
}

func stsRef(name string) Ref { return Ref{Kind: KindStatefulSet, Namespace: "team-a", Name: name} }

// AC1: whenScaled=Delete and no opt-in → skipped, no checkpoint, Warning Event.
func TestStatefulSet_RefusesWhenScaledDelete(t *testing.T) {
	s, c := newSleeper(newSTSWithPolicy("team-a", "db", 3, appsv1.DeletePersistentVolumeClaimRetentionPolicyType, nil))
	rec := record.NewFakeRecorder(10)
	s.Recorder = rec
	sch := schedule()
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{stsRef("db")}); err != nil {
		t.Fatal(err)
	}
	if got := stsReplicas(t, c, "db"); got != 3 {
		t.Fatalf("replicas = %d, want 3 (must not be slept)", got)
	}
	key := checkpoint.Key("StatefulSet", "team-a", "db", "db-uid")
	if _, found, _ := s.Store.GetRaw(ctx, sch, key); found {
		t.Fatal("a skipped StatefulSet must not be checkpointed")
	}
	var warned bool
	for _, e := range drain(rec) {
		if strings.Contains(e, "Warning") && strings.Contains(e, "PVCDeletionRisk") {
			warned = true
		}
	}
	if !warned {
		t.Fatal("expected a PVCDeletionRisk Warning Event")
	}
}

// AC2: the StatefulSet annotated nyx.dev/allow-pvc-deletion=true sleeps normally.
func TestStatefulSet_AllowsWithAnnotation(t *testing.T) {
	annos := map[string]string{AnnotationAllowPVCDeletion: "true"}
	s, c := newSleeper(newSTSWithPolicy("team-a", "db", 3, appsv1.DeletePersistentVolumeClaimRetentionPolicyType, annos))
	sch := schedule()
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{stsRef("db")}); err != nil {
		t.Fatal(err)
	}
	if got := stsReplicas(t, c, "db"); got != 0 {
		t.Fatalf("replicas = %d, want 0 (opted in → slept)", got)
	}
	key := checkpoint.Key("StatefulSet", "team-a", "db", "db-uid")
	if _, found, _ := s.Store.GetRaw(ctx, sch, key); !found {
		t.Fatal("opted-in StatefulSet should be checkpointed")
	}
}

// AC2 (inherited): the opt-in on the SleepSchedule also allows it.
func TestStatefulSet_AllowsViaScheduleAnnotation(t *testing.T) {
	s, c := newSleeper(newSTSWithPolicy("team-a", "db", 3, appsv1.DeletePersistentVolumeClaimRetentionPolicyType, nil))
	sch := schedule()
	sch.Annotations = map[string]string{AnnotationAllowPVCDeletion: "true"}
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{stsRef("db")}); err != nil {
		t.Fatal(err)
	}
	if got := stsReplicas(t, c, "db"); got != 0 {
		t.Fatalf("replicas = %d, want 0 (schedule opt-in → slept)", got)
	}
}

// AC3: whenScaled=Retain → guard inert, sleeps normally.
func TestStatefulSet_RetainSleepsNormally(t *testing.T) {
	s, c := newSleeper(newSTSWithPolicy("team-a", "db", 3, appsv1.RetainPersistentVolumeClaimRetentionPolicyType, nil))
	sch := schedule()
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{stsRef("db")}); err != nil {
		t.Fatal(err)
	}
	if got := stsReplicas(t, c, "db"); got != 0 {
		t.Fatalf("replicas = %d, want 0 (Retain → slept)", got)
	}
}

// no retention policy at all → guard inert, sleeps normally.
func TestStatefulSet_NoPolicySleepsNormally(t *testing.T) {
	s, c := newSleeper(newSTSWithPolicy("team-a", "db", 3, "", nil))
	sch := schedule()
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{stsRef("db")}); err != nil {
		t.Fatal(err)
	}
	if got := stsReplicas(t, c, "db"); got != 0 {
		t.Fatalf("replicas = %d, want 0 (no policy → slept)", got)
	}
}
