/*
Copyright 2026.

Licensed under the MIT License.
*/

package controller

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
	"github.com/cedricfarinazzo/k8s-nyx/internal/schedule"
)

// SleepScheduleReconciler reconciles a SleepSchedule object.
//
// VC-124: evaluates the schedule against the current time and records phase +
// next transition in status, requeuing at the next boundary. Applying the
// sleep/wake to workloads is a later E2 story.
type SleepScheduleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// Now returns the current time; overridable in tests. Defaults to time.Now.
	Now func() time.Time
}

// +kubebuilder:rbac:groups=nyx.dev,resources=sleepschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nyx.dev,resources=sleepschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nyx.dev,resources=sleepschedules/finalizers,verbs=update

// Reconcile evaluates the schedule and updates status; it requeues at the next
// transition so phase stays current. It does not mutate workloads (out of scope).
func (r *SleepScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var ss nyxv1alpha1.SleepSchedule
	if err := r.Get(ctx, req.NamespacedName, &ss); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	now := time.Now()
	if r.Now != nil {
		now = r.Now()
	}

	res, err := schedule.Evaluate(ss.Spec, now)
	if err != nil {
		// Spec passed admission, so this is unexpected (e.g. tzdata gap); surface it.
		log.Error(err, "evaluate schedule", "sleepschedule", req.NamespacedName)
		return ctrl.Result{}, err
	}

	ss.Status.Phase = res.Phase
	if res.NextTransition.IsZero() {
		ss.Status.NextTransition = nil
	} else {
		next := metav1.NewTime(res.NextTransition)
		ss.Status.NextTransition = &next
	}
	if err := r.Status().Update(ctx, &ss); err != nil {
		if apierrors.IsConflict(err) {
			return ctrl.Result{Requeue: true}, nil
		}
		return ctrl.Result{}, err
	}

	var requeueAfter time.Duration
	if !res.NextTransition.IsZero() {
		requeueAfter = res.NextTransition.Sub(now)
		if requeueAfter < time.Second {
			requeueAfter = time.Second
		}
	}
	log.V(1).Info("evaluated schedule", "phase", res.Phase, "requeueAfter", requeueAfter)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// SetupWithManager registers the controller with the manager.
func (r *SleepScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nyxv1alpha1.SleepSchedule{}).
		Named("sleepschedule").
		Complete(r)
}
