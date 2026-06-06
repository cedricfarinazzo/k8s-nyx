/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

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

// fixture: workloads across two namespaces with various labels.
func fixture() []client.Object {
	return []client.Object{
		deploy("team-a", "api", map[string]string{"app": "api"}),
		deploy("team-a", "critical-billing", nil),
		deploy("team-b", "api2", map[string]string{"app": "api"}),
		deploy("team-b", "web", map[string]string{"app": "web"}),
		sts("team-a", "db", map[string]string{"app": "db"}),
	}
}

func keys(refs []Ref) []string {
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

// namespaces mode selects only workloads in the listed namespaces.
func TestResolve_NamespacesMode(t *testing.T) {
	r := newResolver(fixture()...)
	spec := nyxv1alpha1.SleepScheduleSpec{
		Target: nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a"}},
	}
	got, _, err := r.Resolve(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Deployment:team-a/api", "Deployment:team-a/critical-billing", "StatefulSet:team-a/db"}
	if g := keys(got); !equal(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

// labels mode selects matching workloads cluster-wide.
func TestResolve_LabelsMode(t *testing.T) {
	r := newResolver(fixture()...)
	spec := nyxv1alpha1.SleepScheduleSpec{
		Target: nyxv1alpha1.Target{
			Mode:     nyxv1alpha1.TargetModeLabels,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
		},
	}
	got, _, err := r.Resolve(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Deployment:team-a/api", "Deployment:team-b/api2"}
	if g := keys(got); !equal(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

// a workload matching an excludeRef is never selected.
func TestResolve_Exclusions(t *testing.T) {
	r := newResolver(fixture()...)
	spec := nyxv1alpha1.SleepScheduleSpec{
		Target:      nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a"}},
		ExcludeRefs: []nyxv1alpha1.ResourceRef{{Kind: "Deployment", Name: "critical-billing"}},
	}
	got, _, err := r.Resolve(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Deployment:team-a/api", "StatefulSet:team-a/db"}
	if g := keys(got); !equal(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

// a namespace-scoped excludeRef drops only the matching namespace.
func TestResolve_ExclusionsNamespaceScoped(t *testing.T) {
	objs := append(fixture(), deploy("team-b", "api", map[string]string{"app": "api"}))
	r := newResolver(objs...)
	spec := nyxv1alpha1.SleepScheduleSpec{
		Target:      nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a", "team-b"}},
		Kinds:       []string{"Deployment"},
		ExcludeRefs: []nyxv1alpha1.ResourceRef{{Kind: "Deployment", Namespace: "team-a", Name: "api"}},
	}
	got, _, err := r.Resolve(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Deployment:team-a/critical-billing",
		"Deployment:team-b/api",
		"Deployment:team-b/api2",
		"Deployment:team-b/web",
	}
	if g := keys(got); !equal(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

// a namespace-less excludeRef is a wildcard across namespaces.
func TestResolve_ExclusionsWildcardAllNamespaces(t *testing.T) {
	objs := append(fixture(), deploy("team-b", "api", map[string]string{"app": "api"}))
	r := newResolver(objs...)
	spec := nyxv1alpha1.SleepScheduleSpec{
		Target:      nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a", "team-b"}},
		Kinds:       []string{"Deployment"},
		ExcludeRefs: []nyxv1alpha1.ResourceRef{{Kind: "Deployment", Name: "api"}},
	}
	got, _, err := r.Resolve(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"Deployment:team-a/critical-billing",
		"Deployment:team-b/api2",
		"Deployment:team-b/web",
	}
	if g := keys(got); !equal(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

// AC1: a kind not in spec.kinds is never resolved (so never mutated/checkpointed).
func TestResolve_KindsFilter(t *testing.T) {
	r := newResolver(fixture()...)
	spec := nyxv1alpha1.SleepScheduleSpec{
		Kinds:  []string{"Deployment"}, // StatefulSet excluded
		Target: nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a"}},
	}
	got, unhandled, err := r.Resolve(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if len(unhandled) != 0 {
		t.Fatalf("unhandled = %v, want none", unhandled)
	}
	want := []string{"Deployment:team-a/api", "Deployment:team-a/critical-billing"}
	if g := keys(got); !equal(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
	for _, ref := range got {
		if ref.Kind == KindStatefulSet {
			t.Fatalf("StatefulSet %s resolved despite not being in spec.kinds", ref.Name)
		}
	}
}

// AC2: a requested kind with no registered handler is reported as unhandled and
// contributes no workloads (the caller warns and continues).
func TestResolve_UnhandledKindReported(t *testing.T) {
	r := newResolver(fixture()...)
	spec := nyxv1alpha1.SleepScheduleSpec{
		Kinds:  []string{"Deployment", "ReplicaSet"}, // ReplicaSet has no handler yet
		Target: nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a"}},
	}
	got, unhandled, err := r.Resolve(context.Background(), spec)
	if err != nil {
		t.Fatalf("unhandled kind must not error: %v", err)
	}
	if !equal(unhandled, []string{"ReplicaSet"}) {
		t.Fatalf("unhandled = %v, want [ReplicaSet]", unhandled)
	}
	// Deployments still resolved; nothing of the unhandled kind appears.
	want := []string{"Deployment:team-a/api", "Deployment:team-a/critical-billing"}
	if g := keys(got); !equal(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}

// AC3: changing spec.kinds changes eligibility — a newly-included kind becomes
// resolvable, a newly-excluded kind drops out (no forced restore happens here;
// it is simply not acted on).
func TestResolve_KindsChangeEligibility(t *testing.T) {
	r := newResolver(fixture()...)
	target := nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a"}}

	// Only Deployments eligible.
	got, _, err := r.Resolve(context.Background(), nyxv1alpha1.SleepScheduleSpec{Kinds: []string{"Deployment"}, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if g := keys(got); !equal(g, []string{"Deployment:team-a/api", "Deployment:team-a/critical-billing"}) {
		t.Fatalf("pass 1 got %v", g)
	}

	// StatefulSet newly included → now eligible.
	got, _, err = r.Resolve(context.Background(), nyxv1alpha1.SleepScheduleSpec{Kinds: []string{"Deployment", "StatefulSet"}, Target: target})
	if err != nil {
		t.Fatal(err)
	}
	if g := keys(got); !equal(g, []string{"Deployment:team-a/api", "Deployment:team-a/critical-billing", "StatefulSet:team-a/db"}) {
		t.Fatalf("pass 2 got %v", g)
	}
}

// empty spec.kinds defaults to every registered kind.
func TestResolve_DefaultKinds(t *testing.T) {
	r := newResolver(fixture()...)
	got, _, err := r.Resolve(context.Background(), nyxv1alpha1.SleepScheduleSpec{
		Target: nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{"team-a"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Deployment:team-a/api", "Deployment:team-a/critical-billing", "StatefulSet:team-a/db"}
	if g := keys(got); !equal(g, want) {
		t.Fatalf("got %v, want %v", g, want)
	}
}
