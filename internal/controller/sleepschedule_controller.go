/*
Copyright 2026.

Licensed under the MIT License.
*/

package controller

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
	"github.com/cedricfarinazzo/k8s-nyx/internal/schedule"
	"github.com/cedricfarinazzo/k8s-nyx/internal/sleeper"
	"github.com/cedricfarinazzo/k8s-nyx/internal/target"
	"github.com/cedricfarinazzo/k8s-nyx/internal/wake"
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
	// Recorder emits Events on affected workloads; may be nil in tests.
	Recorder record.EventRecorder
	// Now returns the current time; overridable in tests. Defaults to time.Now.
	Now func() time.Time
}

// +kubebuilder:rbac:groups=nyx.dev,resources=sleepschedules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nyx.dev,resources=sleepschedules/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nyx.dev,resources=sleepschedules/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile evaluates the schedule, applies sleep/wake to the targeted workloads,
// updates status, and requeues at the next transition.
func (r *SleepScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var ss nyxv1alpha1.SleepSchedule
	if err := r.Get(ctx, req.NamespacedName, &ss); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure the per-schedule Wake ConfigMap exists (the override surface).
	if err := r.ensureWakeConfigMap(ctx, &ss); err != nil {
		log.Error(err, "ensure wake configmap", "sleepschedule", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// Parse the wake entries: surface malformed ones and audit accepted ones.
	// Resolving/honouring them is a separate story.
	if err := r.processWakeEntries(ctx, &ss); err != nil {
		log.Error(err, "process wake entries", "sleepschedule", req.NamespacedName)
		return ctrl.Result{}, err
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
	sl := &sleeper.Sleeper{
		Client:   r.Client,
		Store:    &checkpoint.Store{Client: r.Client, Namespace: r.OperatorNamespace},
		Recorder: r.Recorder,
	}
	if err := sl.Apply(ctx, &ss, !res.Awake, targets); err != nil {
		log.Error(err, "apply sleep/wake", "sleepschedule", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// Update status only when it actually changed — repeated reconciles with no
	// time change must not write (AC4 idempotency).
	var nextStatus *metav1.Time
	if !res.NextTransition.IsZero() {
		next := metav1.NewTime(res.NextTransition)
		nextStatus = &next
	}
	if statusChanged(ss.Status, res.Phase, nextStatus) {
		ss.Status.Phase = res.Phase
		ss.Status.NextTransition = nextStatus
		if err := r.Status().Update(ctx, &ss); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
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

// WakeConfigMapName is the name of the per-schedule Wake ConfigMap.
func WakeConfigMapName(scheduleName string) string {
	return scheduleName + "-wake"
}

// ensureWakeConfigMap creates the schedule's Wake ConfigMap (the override surface)
// if it does not already exist, owned by the SleepSchedule so it is garbage-collected
// with the schedule and re-enqueues the owner on changes. Create-only: an existing
// ConfigMap (which may hold wake entries written by triggers) is left untouched.
func (r *SleepScheduleReconciler) ensureWakeConfigMap(ctx context.Context, ss *nyxv1alpha1.SleepSchedule) error {
	name := WakeConfigMapName(ss.Name)
	var cm corev1.ConfigMap
	err := r.Get(ctx, types.NamespacedName{Namespace: ss.Namespace, Name: name}, &cm)
	if err == nil {
		return nil // already exists; never clobber trigger-written data
	}
	if !apierrors.IsNotFound(err) {
		return err
	}
	cm = corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ss.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "k8s-nyx",
				"nyx.dev/schedule":             ss.Name,
			},
		},
	}
	if err := controllerutil.SetControllerReference(ss, &cm, r.Scheme); err != nil {
		return err
	}
	return r.Create(ctx, &cm)
}

// processWakeEntries reads the schedule's Wake ConfigMap, drops malformed entries
// with a Warning Event naming the key (AC3), and records accepted entries' by/reason
// to the log and a Normal Event for the audit trail (AC4).
func (r *SleepScheduleReconciler) processWakeEntries(ctx context.Context, ss *nyxv1alpha1.SleepSchedule) error {
	log := logf.FromContext(ctx)

	var cm corev1.ConfigMap
	err := r.Get(ctx, types.NamespacedName{Namespace: ss.Namespace, Name: WakeConfigMapName(ss.Name)}, &cm)
	if apierrors.IsNotFound(err) {
		return nil // ensure runs first; absence here just means nothing to parse yet
	}
	if err != nil {
		return err
	}

	valid, errs := wake.ParseData(cm.Data)
	for key, perr := range errs {
		log.Info("ignoring malformed wake entry", "key", key, "error", perr.Error())
		if r.Recorder != nil {
			r.Recorder.Eventf(ss, corev1.EventTypeWarning, "MalformedWakeEntry",
				"ignored malformed wake entry %q: %v", key, perr)
		}
	}
	now := r.now()
	def, max := temporaryWakeBounds(ss)
	dirty := false
	for _, e := range valid {
		log.V(1).Info("wake entry", "key", e.Key, "by", e.By, "reason", e.Reason)
		if r.Recorder != nil {
			r.Recorder.Eventf(ss, corev1.EventTypeNormal, "WakeEntryAccepted",
				"wake entry %q accepted (by=%q reason=%q)", e.Key, e.By, e.Reason)
		}
		res, rerr := wake.Resolve(e, now, def, max)
		if rerr != nil {
			// e.g. no expiry and no defaultDuration configured — treat like malformed.
			log.Info("cannot resolve wake entry", "key", e.Key, "error", rerr.Error())
			if r.Recorder != nil {
				r.Recorder.Eventf(ss, corev1.EventTypeWarning, "UnresolvableWakeEntry",
					"ignored wake entry %q: %v", e.Key, rerr)
			}
			continue
		}
		if res.Changed {
			cm.Data[e.Key] = wake.FormatEntry(res.Expiry, e.By, e.Reason)
			dirty = true
		}
		if res.Clamped && r.Recorder != nil {
			r.Recorder.Eventf(ss, corev1.EventTypeNormal, "WakeClamped",
				"wake entry %q clamped to maxDuration (expiry %s)", e.Key, res.Expiry.UTC().Format(time.RFC3339))
		}
	}

	// Persist any stamped/clamped values exactly once; subsequent reconciles see
	// absolute, within-cap values and leave them unchanged (AC1 — no re-extend).
	if dirty {
		return r.Update(ctx, &cm)
	}
	return nil
}

// temporaryWakeBounds returns the configured default / max wake durations, or 0s
// when temporaryWake is unset (no default available, no cap applied).
func temporaryWakeBounds(ss *nyxv1alpha1.SleepSchedule) (def, max time.Duration) {
	if tw := ss.Spec.TemporaryWake; tw != nil {
		return tw.DefaultDuration.Duration, tw.MaxDuration.Duration
	}
	return 0, 0
}

// now returns the reconciler's clock (overridable in tests).
func (r *SleepScheduleReconciler) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// statusChanged reports whether the computed phase / nextTransition differ from
// what is already on the object, so the reconciler can skip a no-op status write.
func statusChanged(cur nyxv1alpha1.SleepScheduleStatus, phase nyxv1alpha1.SleepSchedulePhase, next *metav1.Time) bool {
	if cur.Phase != phase {
		return true
	}
	switch {
	case cur.NextTransition == nil && next == nil:
		return false
	case cur.NextTransition == nil || next == nil:
		return true
	default:
		return !cur.NextTransition.Equal(next)
	}
}

// SetupWithManager registers the controller with the manager.
func (r *SleepScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nyxv1alpha1.SleepSchedule{}).
		Owns(&corev1.ConfigMap{}).
		Named("sleepschedule").
		Complete(r)
}
