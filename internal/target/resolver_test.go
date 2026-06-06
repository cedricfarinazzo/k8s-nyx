/*
Copyright 2026.

Licensed under the MIT License.
*/

package target

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

func deploy(ns, name string, labels map[string]string) *appsv1.Deployment {
	return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, Labels: labels}}
}

func sts(ns, name string, labels map[string]string) *appsv1.StatefulSet {
	return &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, Labels: labels}}
}

func newResolver(objs ...client.Object) *Resolver {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = nyxv1alpha1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &Resolver{Client: c}
}

// fixture: workloads across three namespaces with various labels.
func fixture() []client.Object {
	return []client.Object{
		deploy("team-a", "api", map[string]string{"app": "api"}),
		deploy("team-a", "critical-billing", nil),
		deploy("team-b", "api2", map[string]string{"app": "api"}),
		deploy("team-b", "web", map[string]string{"app": "web"}),
		sts("team-a", "db", map[string]string{"app": "db"}),
	}
}

func keys(refs []WorkloadRef) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.Kind+":"+r.Namespace+"/"+r.Name)
	}
	return out
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// AC1: namespaces mode selects only workloads in the listed namespaces.
func TestResolve_NamespacesMode(t *testing.T) {
	r := newResolver(fixture()...)
	spec := nyxv1alpha1.SleepScheduleSpec{
		Target: nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a"}},
	}
	got, err := r.Resolve(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Deployment:team-a/api",
		"Deployment:team-a/critical-billing",
		"StatefulSet:team-a/db",
	}
	if g := keys(got); !equal(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

// AC2: labels mode selects workloads matching the selector cluster-wide.
func TestResolve_LabelsMode(t *testing.T) {
	r := newResolver(fixture()...)
	spec := nyxv1alpha1.SleepScheduleSpec{
		Target: nyxv1alpha1.Target{
			Mode:     nyxv1alpha1.TargetModeLabels,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
		},
	}
	got, err := r.Resolve(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	// api in team-a and api2 in team-b both carry app=api — cluster-wide.
	want := []string{"Deployment:team-a/api", "Deployment:team-b/api2"}
	if g := keys(got); !equal(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

// AC3: a workload matching an excludeRef is never selected.
func TestResolve_Exclusions(t *testing.T) {
	r := newResolver(fixture()...)
	spec := nyxv1alpha1.SleepScheduleSpec{
		Target:      nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a"}},
		ExcludeRefs: []nyxv1alpha1.ResourceRef{{Kind: "Deployment", Name: "critical-billing"}},
	}
	got, err := r.Resolve(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Deployment:team-a/api", "StatefulSet:team-a/db"}
	if g := keys(got); !equal(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

// AC4: only kinds listed in spec.kinds are considered.
func TestResolve_KindsFilter(t *testing.T) {
	r := newResolver(fixture()...)
	spec := nyxv1alpha1.SleepScheduleSpec{
		Kinds:  []string{"Deployment"}, // StatefulSet excluded
		Target: nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a"}},
	}
	got, err := r.Resolve(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Deployment:team-a/api", "Deployment:team-a/critical-billing"}
	if g := keys(got); !equal(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

// An unsupported kind in spec.kinds is ignored (deferred to E5), not an error.
func TestResolve_UnsupportedKindIgnored(t *testing.T) {
	r := newResolver(fixture()...)
	spec := nyxv1alpha1.SleepScheduleSpec{
		Kinds:  []string{"DaemonSet"}, // not yet supported
		Target: nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a"}},
	}
	got, err := r.Resolve(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no refs for unsupported kind, got %v", keys(got))
	}
}
