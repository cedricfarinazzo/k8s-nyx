/*
Copyright 2026.

Licensed under the MIT License.
*/

package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

// SleepScheduleReconciler reconciles a SleepSchedule object.
//
// VC-120 scaffold: this is a no-op reconciler. Real reconcile logic lands in E2.
type SleepScheduleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nyx.dev,resources=sleepschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nyx.dev,resources=sleepschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nyx.dev,resources=sleepschedules/finalizers,verbs=update

// Reconcile is a no-op for the scaffold; it only logs that it was called.
func (r *SleepScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("reconcile (no-op scaffold)", "sleepschedule", req.NamespacedName)
	return ctrl.Result{}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *SleepScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nyxv1alpha1.SleepSchedule{}).
		Named("sleepschedule").
		Complete(r)
}
