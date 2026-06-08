/*
Copyright 2026.

Licensed under the MIT License.
*/

package checkpoint

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

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

// A concurrent writer can move the checkpoint Secret out from under the
// Get→Update in SetRaw, producing a Conflict ("object has been modified").
// SetRaw must refetch and retry rather than surfacing it as a reconcile error.
func TestStore_SetRawRetriesOnConflict(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = nyxv1alpha1.AddToScheme(scheme)
	var updates int
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				updates++
				if updates == 1 {
					return apierrors.NewConflict(
						schema.GroupResource{Resource: "secrets"}, obj.GetName(), errors.New("modified"))
				}
				return cl.Update(ctx, obj, opts...)
			},
		}).Build()
	s := &Store{Client: c, Namespace: "nyx-system"}
	ctx := context.Background()

	// Seed an entry so the Secret exists; the next Set takes the Get→Update path.
	if err := s.Set(ctx, sched(), "first", 1); err != nil {
		t.Fatal(err)
	}
	// The first Update conflicts; SetRaw should refetch and retry to success.
	if err := s.Set(ctx, sched(), "k", 3); err != nil {
		t.Fatalf("SetRaw should retry past a Conflict, got: %v", err)
	}
	if updates < 2 {
		t.Fatalf("expected the Update to be retried after Conflict, saw %d call(s)", updates)
	}
	if got, found, err := s.Get(ctx, sched(), "k"); err != nil || !found || got != 3 {
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
