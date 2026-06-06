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
	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
	"github.com/cedricfarinazzo/k8s-nyx/internal/schedule"
	"github.com/cedricfarinazzo/k8s-nyx/internal/sleeper"
	"github.com/cedricfarinazzo/k8s-nyx/internal/target"
)

// SleepScheduleReconciler reconciles a SleepSchedule object: it evaluates the
// schedule, resolves the targeted workloads, scales them to sleep / restores them
// on wake (exact prior replicas via the Checkpoint Secret), records phase + next
// transition in status, and requeues at the next boundary.
type SleepScheduleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// OperatorNamespace is where the Checkpoint Secrets live (e.g. nyx-system).
	OperatorNamespace string
	// Now returns the current time; overridable in tests. Defaults to time.Now.
	Now func() time.Time
}

// +kubebuilder:rbac:groups=nyx.dev,resources=sleepschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nyx.dev,resources=sleepschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nyx.dev,resources=sleepschedules/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete

// Reconcile evaluates the schedule, applies sleep/wake to the targeted workloads,
// updates status, and requeues at the next transition.
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

	// Resolve targets and apply the sleep/wake decision.
	resolver := &target.Resolver{Client: r.Client}
	targets, err := resolver.Resolve(ctx, ss.Spec)
	if err != nil {
		log.Error(err, "resolve targets", "sleepschedule", req.NamespacedName)
		return ctrl.Result{}, err
	}
	sl := &sleeper.Sleeper{Client: r.Client, Store: &checkpoint.Store{Client: r.Client, Namespace: r.OperatorNamespace}}
	if err := sl.Apply(ctx, &ss, !res.Awake, targets); err != nil {
		log.Error(err, "apply sleep/wake", "sleepschedule", req.NamespacedName)
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
