/*
Copyright 2026.

Licensed under the MIT License.
*/

package controller

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

// namespaceTerminatingErr mimics the apiserver's "forbidden: unable to create
// new content because the namespace is being terminated" — a Forbidden status
// carrying a NamespaceTerminating cause.
func namespaceTerminatingErr(name string) error {
	return &apierrors.StatusError{ErrStatus: metav1.Status{
		Status: metav1.StatusFailure,
		Reason: metav1.StatusReasonForbidden,
		Message: "configmaps \"" + name + "\" is forbidden: unable to create new content " +
			"in namespace because it is being terminated",
		Details: &metav1.StatusDetails{
			Causes: []metav1.StatusCause{{
				Type:    corev1.NamespaceTerminatingCause,
				Message: "namespace is being terminated",
			}},
		},
	}}
}

// When a schedule's namespace is being deleted, creating its wake ConfigMap is
// forbidden. That is expected teardown, not a reconcile error: ensureWakeConfigMap
// must return nil so the reconciler stops churning ERROR logs while GC runs.
func TestEnsureWakeConfigMap_NamespaceTerminating(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = nyxv1alpha1.AddToScheme(scheme)

	ss := &nyxv1alpha1.SleepSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "lifecycle", Namespace: "team-a"},
	}
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, obj client.Object, _ ...client.CreateOption) error {
				return namespaceTerminatingErr(obj.GetName())
			},
		}).Build()
	r := &SleepScheduleReconciler{Client: c, Scheme: scheme}

	if err := r.ensureWakeConfigMap(context.Background(), ss); err != nil {
		t.Fatalf("ensureWakeConfigMap should swallow a NamespaceTerminating create, got: %v", err)
	}
}

// A Create failure for any other reason must still surface — the swallow is
// scoped to namespace termination, not "all create errors".
func TestEnsureWakeConfigMap_OtherCreateErrorSurfaces(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = nyxv1alpha1.AddToScheme(scheme)

	ss := &nyxv1alpha1.SleepSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: "lifecycle", Namespace: "team-a"},
	}
	wantErr := errors.New("boom")
	c := fake.NewClientBuilder().WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(_ context.Context, _ client.WithWatch, _ client.Object, _ ...client.CreateOption) error {
				return wantErr
			},
		}).Build()
	r := &SleepScheduleReconciler{Client: c, Scheme: scheme}

	if err := r.ensureWakeConfigMap(context.Background(), ss); !errors.Is(err, wantErr) {
		t.Fatalf("expected the create error to surface, got: %v", err)
	}
}
