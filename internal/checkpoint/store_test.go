/*
Copyright 2026.

Licensed under the MIT License.
*/

package checkpoint

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

func newStore() (*Store, client.Client) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = nyxv1alpha1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	return &Store{Client: c, Namespace: "nyx-system"}, c
}

func sched() *nyxv1alpha1.SleepSchedule {
	return &nyxv1alpha1.SleepSchedule{ObjectMeta: metav1.ObjectMeta{Name: "sched", Namespace: "team-a"}}
}

func TestKey(t *testing.T) {
	k := Key("Deployment", "team-a", "api", "uid-123")
	if k != "apps_v1_Deployment_team-a_api_uid-123" {
		t.Fatalf("Key = %q", k)
	}
	// Different UID ⇒ different key (guards against stale restore of a recreated workload).
	if Key("Deployment", "team-a", "api", "uid-999") == k {
		t.Fatal("keys with different UID must differ")
	}
}

func TestStore_GetCorruptValue(t *testing.T) {
	s, c := newStore()
	ctx := context.Background()
	if err := s.Set(ctx, sched(), "k", 3); err != nil {
		t.Fatal(err)
	}
	sec := &corev1.Secret{}
	if err := c.Get(ctx, types.NamespacedName{Namespace: "nyx-system", Name: "sched-checkpoint"}, sec); err != nil {
		t.Fatal(err)
	}
	sec.Data["k"] = []byte("not-an-int")
	if err := c.Update(ctx, sec); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Get(ctx, sched(), "k"); err == nil {
		t.Fatal("expected error for corrupt checkpoint value")
	}
}

func TestStore_GetMissing(t *testing.T) {
	s, _ := newStore()
	if _, found, err := s.Get(context.Background(), sched(), "k"); err != nil || found {
		t.Fatalf("expected (not found, no error), got (found=%v, err=%v)", found, err)
	}
}

func TestStore_SetDoesNotClobber(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()
	if err := s.Set(ctx, sched(), "k", 3); err != nil {
		t.Fatal(err)
	}
	// A second Set for the same key must keep the first (true original) value.
	if err := s.Set(ctx, sched(), "k", 0); err != nil {
		t.Fatal(err)
	}
	got, found, err := s.Get(ctx, sched(), "k")
	if err != nil || !found || got != 3 {
		t.Fatalf("got (%d, %v, %v), want (3, true, nil)", got, found, err)
	}
}

func TestStore_DeleteRemovesEmptySecret(t *testing.T) {
	s, c := newStore()
	ctx := context.Background()
	if err := s.Set(ctx, sched(), "k", 3); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, sched(), "k"); err != nil {
		t.Fatal(err)
	}
	// The last entry was removed, so the Secret itself should be gone.
	var sec corev1.Secret
	err := c.Get(ctx, types.NamespacedName{Namespace: "nyx-system", Name: "sched-checkpoint"}, &sec)
	if err == nil {
		t.Fatalf("expected checkpoint Secret to be deleted")
	}
}

func TestStore_DeleteKeepsSecretWithRemainingEntries(t *testing.T) {
	s, c := newStore()
	ctx := context.Background()
	_ = s.Set(ctx, sched(), "k1", 3)
	_ = s.Set(ctx, sched(), "k2", 5)
	if err := s.Delete(ctx, sched(), "k1"); err != nil {
		t.Fatal(err)
	}
	var sec corev1.Secret
	if err := c.Get(ctx, types.NamespacedName{Namespace: "nyx-system", Name: "sched-checkpoint"}, &sec); err != nil {
		t.Fatalf("secret should still exist: %v", err)
	}
	if _, ok := sec.Data["k2"]; !ok {
		t.Fatalf("k2 should remain")
	}
	if _, ok := sec.Data["k1"]; ok {
		t.Fatalf("k1 should be gone")
	}
}
