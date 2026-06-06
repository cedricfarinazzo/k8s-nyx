/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import "testing"

func TestRegistry_DefaultKinds(t *testing.T) {
	reg := Default()
	want := []string{KindDeployment, KindStatefulSet, KindDaemonSet}
	got := reg.Kinds()
	if len(got) != len(want) {
		t.Fatalf("Kinds() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Kinds() = %v, want %v", got, want)
		}
	}
}

func TestRegistry_GetAndHas(t *testing.T) {
	reg := Default()
	if h, ok := reg.Get(KindDeployment); !ok || h.Kind() != KindDeployment {
		t.Fatalf("Get(Deployment) = (%v, %v)", h, ok)
	}
	if !reg.Has(KindStatefulSet) || !reg.Has(KindDaemonSet) {
		t.Fatal("Has(StatefulSet/DaemonSet) = false, want true")
	}
	if _, ok := reg.Get("CronJob"); ok {
		t.Fatal("Get(CronJob) ok = true, want false (no handler)")
	}
	if reg.Has("CronJob") {
		t.Fatal("Has(CronJob) = true, want false")
	}
}

// Kinds() returns a copy — callers cannot mutate the registry's order.
func TestRegistry_KindsIsCopy(t *testing.T) {
	reg := Default()
	ks := reg.Kinds()
	ks[0] = "mutated"
	if reg.Kinds()[0] != KindDeployment {
		t.Fatal("Kinds() returned a mutable view of the registry")
	}
}

// A later handler for the same kind replaces an earlier one without duplicating
// the kind in Kinds().
func TestRegistry_OverrideNoDuplicate(t *testing.T) {
	reg := NewRegistry(deploymentHandler{}, deploymentHandler{})
	if got := reg.Kinds(); len(got) != 1 {
		t.Fatalf("Kinds() = %v, want one entry", got)
	}
}
