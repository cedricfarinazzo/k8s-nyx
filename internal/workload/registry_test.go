/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import "testing"

func TestRegistry_DefaultKinds(t *testing.T) {
	reg := Default()
	if got := reg.Kinds(); len(got) != 2 || got[0] != KindDeployment || got[1] != KindStatefulSet {
		t.Fatalf("Kinds() = %v, want [Deployment StatefulSet]", got)
	}
}

func TestRegistry_GetAndHas(t *testing.T) {
	reg := Default()
	if h, ok := reg.Get(KindDeployment); !ok || h.Kind() != KindDeployment {
		t.Fatalf("Get(Deployment) = (%v, %v)", h, ok)
	}
	if !reg.Has(KindStatefulSet) {
		t.Fatal("Has(StatefulSet) = false, want true")
	}
	if _, ok := reg.Get("DaemonSet"); ok {
		t.Fatal("Get(DaemonSet) ok = true, want false (no handler)")
	}
	if reg.Has("DaemonSet") {
		t.Fatal("Has(DaemonSet) = true, want false")
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
