/*
Copyright 2026.

Licensed under the MIT License.
*/

package sleeper

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
	"github.com/cedricfarinazzo/k8s-nyx/internal/target"
)

const opNamespace = "nyx-system"

func ptr(i int32) *int32 { return &i }

func newDeployment(ns, name string, replicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID(name + "-uid")},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr(replicas)},
	}
}

func newStatefulSet(ns, name string, replicas int32) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID(name + "-uid")},
		Spec:       appsv1.StatefulSetSpec{Replicas: ptr(replicas)},
	}
}

func newSleeper(objs ...client.Object) (*Sleeper, client.Client) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = nyxv1alpha1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &Sleeper{Client: c, Store: &checkpoint.Store{Client: c, Namespace: opNamespace}}, c
}

func schedule() *nyxv1alpha1.SleepSchedule {
	return &nyxv1alpha1.SleepSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "sched", Namespace: "team-a"},
		Spec: nyxv1alpha1.SleepScheduleSpec{
			Timezone:      "Europe/Paris",
			SleepReplicas: 0,
			Target:        nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a"}},
		},
	}
}

func deployReplicas(t *testing.T, c client.Client, name string) int32 {
	t.Helper()
	var d appsv1.Deployment
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "team-a", Name: name}, &d); err != nil {
		t.Fatalf("get deployment: %v", err)
	}
	return *d.Spec.Replicas
}

// AC1: asleep scales to sleepReplicas and records the original in the checkpoint.
func TestApply_SleepRecordsAndScales(t *testing.T) {
	s, c := newSleeper(newDeployment("team-a", "api", 3))
	sch := schedule()
	ref := target.WorkloadRef{Kind: target.KindDeployment, Namespace: "team-a", Name: "api"}

	if err := s.Apply(context.Background(), sch, true, []target.WorkloadRef{ref}); err != nil {
		t.Fatal(err)
	}
	if got := deployReplicas(t, c, "api"); got != 0 {
		t.Fatalf("replicas = %d, want 0", got)
	}
	key := checkpoint.Key("Deployment", "team-a", "api", "api-uid")
	orig, found, err := s.Store.Get(context.Background(), sch, key)
	if err != nil || !found {
		t.Fatalf("checkpoint missing (found=%v err=%v)", found, err)
	}
	if orig != 3 {
		t.Fatalf("checkpoint = %d, want 3", orig)
	}
}

// AC2: awake restores the exact original and clears the checkpoint entry.
func TestApply_WakeRestoresAndClears(t *testing.T) {
	s, c := newSleeper(newDeployment("team-a", "api", 3))
	sch := schedule()
	ref := target.WorkloadRef{Kind: target.KindDeployment, Namespace: "team-a", Name: "api"}

	_ = s.Apply(context.Background(), sch, true, []target.WorkloadRef{ref})
	if err := s.Apply(context.Background(), sch, false, []target.WorkloadRef{ref}); err != nil {
		t.Fatal(err)
	}
	if got := deployReplicas(t, c, "api"); got != 3 {
		t.Fatalf("restored replicas = %d, want 3", got)
	}
	key := checkpoint.Key("Deployment", "team-a", "api", "api-uid")
	if _, found, _ := s.Store.Get(context.Background(), sch, key); found {
		t.Fatalf("checkpoint entry should be cleared")
	}
}

// AC3: a fresh Sleeper (no in-memory state) restores from the persisted checkpoint.
func TestApply_RestoreSurvivesRestart(t *testing.T) {
	s1, c := newSleeper(newDeployment("team-a", "api", 5))
	sch := schedule()
	ref := target.WorkloadRef{Kind: target.KindDeployment, Namespace: "team-a", Name: "api"}

	_ = s1.Apply(context.Background(), sch, true, []target.WorkloadRef{ref})

	// Simulate operator restart: brand-new Sleeper + Store over the same cluster.
	s2 := &Sleeper{Client: c, Store: &checkpoint.Store{Client: c, Namespace: opNamespace}}
	if err := s2.Apply(context.Background(), sch, false, []target.WorkloadRef{ref}); err != nil {
		t.Fatal(err)
	}
	if got := deployReplicas(t, c, "api"); got != 5 {
		t.Fatalf("restored replicas = %d, want 5", got)
	}
}

// AC4: a second sleep pass must not re-checkpoint sleepReplicas; the real prior wins.
func TestApply_NoReCheckpointWhileAsleep(t *testing.T) {
	s, c := newSleeper(newDeployment("team-a", "api", 3))
	sch := schedule()
	ref := target.WorkloadRef{Kind: target.KindDeployment, Namespace: "team-a", Name: "api"}
	ctx := context.Background()

	_ = s.Apply(ctx, sch, true, []target.WorkloadRef{ref}) // 3 -> 0, checkpoint 3
	_ = s.Apply(ctx, sch, true, []target.WorkloadRef{ref}) // still asleep; must NOT record 0

	key := checkpoint.Key("Deployment", "team-a", "api", "api-uid")
	orig, found, _ := s.Store.Get(ctx, sch, key)
	if !found || orig != 3 {
		t.Fatalf("checkpoint = (%d, %v), want (3, true)", orig, found)
	}
	_ = s.Apply(ctx, sch, false, []target.WorkloadRef{ref})
	if got := deployReplicas(t, c, "api"); got != 3 {
		t.Fatalf("restored replicas = %d, want 3 (not sleepReplicas)", got)
	}
}

// StatefulSets are handled the same way.
func TestApply_StatefulSet(t *testing.T) {
	s, c := newSleeper(newStatefulSet("team-a", "db", 2))
	sch := schedule()
	ref := target.WorkloadRef{Kind: target.KindStatefulSet, Namespace: "team-a", Name: "db"}
	ctx := context.Background()

	_ = s.Apply(ctx, sch, true, []target.WorkloadRef{ref})
	var set appsv1.StatefulSet
	_ = c.Get(ctx, types.NamespacedName{Namespace: "team-a", Name: "db"}, &set)
	if *set.Spec.Replicas != 0 {
		t.Fatalf("sts replicas = %d, want 0", *set.Spec.Replicas)
	}
	_ = s.Apply(ctx, sch, false, []target.WorkloadRef{ref})
	_ = c.Get(ctx, types.NamespacedName{Namespace: "team-a", Name: "db"}, &set)
	if *set.Spec.Replicas != 2 {
		t.Fatalf("restored sts replicas = %d, want 2", *set.Spec.Replicas)
	}
}

// drain returns all currently-buffered events from a FakeRecorder.
func drain(rec *record.FakeRecorder) []string {
	var out []string
	for {
		select {
		case e := <-rec.Events:
			out = append(out, e)
		default:
			return out
		}
	}
}

// AC3: sleep and wake emit Events on the affected workload; a no-op emits none (AC4).
func TestApply_EmitsEvents(t *testing.T) {
	s, _ := newSleeper(newDeployment("team-a", "api", 3))
	rec := record.NewFakeRecorder(10)
	s.Recorder = rec
	sch := schedule()
	ref := target.WorkloadRef{Kind: target.KindDeployment, Namespace: "team-a", Name: "api"}
	ctx := context.Background()

	_ = s.Apply(ctx, sch, true, []target.WorkloadRef{ref}) // sleep
	ev := drain(rec)
	if len(ev) != 1 || !strings.Contains(ev[0], "Slept") {
		t.Fatalf("sleep events = %v, want one Slept", ev)
	}

	_ = s.Apply(ctx, sch, true, []target.WorkloadRef{ref}) // already asleep → no-op
	if ev := drain(rec); len(ev) != 0 {
		t.Fatalf("no-op sleep should emit no events, got %v", ev)
	}

	_ = s.Apply(ctx, sch, false, []target.WorkloadRef{ref}) // wake
	ev = drain(rec)
	if len(ev) != 1 || !strings.Contains(ev[0], "Woke") {
		t.Fatalf("wake events = %v, want one Woke", ev)
	}
}

// dryRun never mutates the workload or writes a checkpoint.
func TestApply_DryRun(t *testing.T) {
	s, c := newSleeper(newDeployment("team-a", "api", 3))
	rec := record.NewFakeRecorder(10)
	s.Recorder = rec
	sch := schedule()
	sch.Spec.DryRun = true
	ref := target.WorkloadRef{Kind: target.KindDeployment, Namespace: "team-a", Name: "api"}

	if err := s.Apply(context.Background(), sch, true, []target.WorkloadRef{ref}); err != nil {
		t.Fatal(err)
	}
	if got := deployReplicas(t, c, "api"); got != 3 {
		t.Fatalf("dry-run changed replicas to %d, want 3", got)
	}
	key := checkpoint.Key("Deployment", "team-a", "api", "api-uid")
	if _, found, _ := s.Store.Get(context.Background(), sch, key); found {
		t.Fatalf("dry-run should not write a checkpoint")
	}
	// AC1: dry-run still emits an Event describing the intended action.
	ev := drain(rec)
	if len(ev) != 1 || !strings.Contains(ev[0], "dry-run") {
		t.Fatalf("dry-run events = %v, want one dry-run event", ev)
	}
}
