/*
Copyright 2026.

Licensed under the MIT License.
*/

package controller

import (
	"context"
	"fmt"
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
	"github.com/cedricfarinazzo/k8s-nyx/internal/audit"
	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
	"github.com/cedricfarinazzo/k8s-nyx/internal/schedule"
	"github.com/cedricfarinazzo/k8s-nyx/internal/wake"
	"github.com/cedricfarinazzo/k8s-nyx/internal/workload"
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
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets;daemonsets,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=batch,resources=cronjobs;jobs,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;update;patch
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

	now := r.now()

	// Parse wake entries: resolve/clamp/stamp, self-clean expired ones, and report
	// the active-wake state that may override the schedule.
	ws, err := r.processWakeEntries(ctx, &ss, now)
	if err != nil {
		log.Error(err, "process wake entries", "sleepschedule", req.NamespacedName)
		return ctrl.Result{}, err
	}

	res, err := schedule.Evaluate(ss.Spec, now)
	if err != nil {
		// Spec passed admission, so this is unexpected (e.g. tzdata gap); surface it.
		log.Error(err, "evaluate schedule", "sleepschedule", req.NamespacedName)
		return ctrl.Result{}, err
	}

	// An active wake forces the targets awake even outside an awake window.
	forcedAwake := ws.ActiveCount > 0
	effectiveAsleep := !res.Awake && !forcedAwake
	phase := res.Phase
	if !res.Awake && forcedAwake {
		phase = nyxv1alpha1.PhaseWokenByOverride
	}

	// Resolve targets and apply the sleep/wake decision. The registry maps each
	// workload kind to its handler; kinds in spec.kinds with no handler resolve
	// to nothing and are surfaced as Warning Events (reconcile continues).
	reg := workload.Default()
	resolver := &workload.Resolver{Client: r.Client, Registry: reg}
	targets, unhandled, err := resolver.Resolve(ctx, ss.Spec)
	if err != nil {
		log.Error(err, "resolve targets", "sleepschedule", req.NamespacedName)
		return ctrl.Result{}, err
	}
	for _, kind := range unhandled {
		log.Info("ignoring kind with no registered handler", "kind", kind, "sleepschedule", req.NamespacedName)
		if r.Recorder != nil {
			r.Recorder.Eventf(&ss, corev1.EventTypeWarning, "UnhandledKind",
				"spec.kinds includes %q but no handler is registered for it; it is ignored", kind)
		}
	}
	sl := &workload.Sleeper{
		Client:   r.Client,
		Store:    &checkpoint.Store{Client: r.Client, Namespace: r.OperatorNamespace},
		Recorder: r.Recorder,
		Registry: reg,
		Who:      audit.DefaultActor,
		Why:      auditReason(effectiveAsleep, forcedAwake),
	}
	if err := sl.Apply(ctx, &ss, effectiveAsleep, targets); err != nil {
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
	activeWakes := int32(ws.ActiveCount)
	if statusChanged(ss.Status, phase, nextStatus, activeWakes) {
		ss.Status.Phase = phase
		ss.Status.NextTransition = nextStatus
		ss.Status.ActiveWakes = activeWakes
		if err := r.Status().Update(ctx, &ss); err != nil {
			if apierrors.IsConflict(err) {
				return ctrl.Result{Requeue: true}, nil
			}
			return ctrl.Result{}, err
		}
	}

	requeueAfter := requeueDelay(now, res.NextTransition, ws.Earliest)
	log.V(1).Info("reconciled", "phase", phase, "activeWakes", activeWakes, "requeueAfter", requeueAfter)
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// requeueDelay returns the wait until the soonest of the next schedule transition
// and the earliest upcoming wake expiry (both may be zero/absent), floored at 1s
// when there is something to wait for.
func requeueDelay(now, nextTransition, earliestExpiry time.Time) time.Duration {
	var best time.Duration
	consider := func(t time.Time) {
		if t.IsZero() {
			return
		}
		d := t.Sub(now)
		if d < time.Second {
			d = time.Second
		}
		if best == 0 || d < best {
			best = d
		}
	}
	consider(nextTransition)
	consider(earliestExpiry)
	return best
}

// auditReason describes why this reconcile pass slept/woke the targets, for the
// audit trail's "why".
func auditReason(asleep, forcedAwake bool) string {
	switch {
	case asleep:
		return "asleep window"
	case forcedAwake:
		return "active wake override"
	default:
		return "awake window"
	}
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

// wakeState summarises the active wake overrides after a reconcile pass.
type wakeState struct {
	// ActiveCount is the number of entries whose expiry is still in the future.
	ActiveCount int
	// Earliest is the soonest upcoming active expiry (zero when no active wakes).
	Earliest time.Time
}

// processWakeEntries reads the Wake ConfigMap, resolves/clamps/stamps entries
// (VC-130), surfaces malformed ones (VC-129), deletes expired entries (AC3), and
// returns the active-wake state. The ConfigMap is written at most once per pass.
func (r *SleepScheduleReconciler) processWakeEntries(ctx context.Context, ss *nyxv1alpha1.SleepSchedule, now time.Time) (wakeState, error) {
	log := logf.FromContext(ctx)

	var cm corev1.ConfigMap
	err := r.Get(ctx, types.NamespacedName{Namespace: ss.Namespace, Name: WakeConfigMapName(ss.Name)}, &cm)
	if apierrors.IsNotFound(err) {
		return wakeState{}, nil // ensure runs first; absence just means nothing to parse yet
	}
	if err != nil {
		return wakeState{}, err
	}

	valid, errs := wake.ParseData(cm.Data)
	for key, perr := range errs {
		log.Info("ignoring malformed wake entry", "key", key, "error", perr.Error())
		if r.Recorder != nil {
			r.Recorder.Eventf(ss, corev1.EventTypeWarning, "MalformedWakeEntry",
				"ignored malformed wake entry %q: %v", key, perr)
		}
	}

	def, max := temporaryWakeBounds(ss)
	dirty := false
	var st wakeState
	for _, e := range valid {
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
		if res.Clamped && r.Recorder != nil {
			r.Recorder.Eventf(ss, corev1.EventTypeNormal, "WakeClamped",
				"wake entry %q clamped to maxDuration (expiry %s)", e.Key, res.Expiry.UTC().Format(time.RFC3339))
		}

		if !res.Expiry.After(now) {
			// Expired: self-clean (AC3).
			delete(cm.Data, e.Key)
			dirty = true
			if r.Recorder != nil {
				r.Recorder.Eventf(ss, corev1.EventTypeNormal, "WakeExpired",
					"wake entry %q expired and was removed", e.Key)
			}
			// Audit log only — the WakeExpired Event above already covers AC2.
			audit.Record(audit.NewContext(ctx, audit.Info{Why: "wake entry expired"}), nil, ss,
				"", "", "", "WakeExpired", fmt.Sprintf("wake entry %q expired", e.Key))
			continue
		}

		// Active wake.
		if res.Changed {
			cm.Data[e.Key] = wake.FormatEntry(res.Expiry, e.By, e.Reason)
			dirty = true
		}
		st.ActiveCount++
		if st.Earliest.IsZero() || res.Expiry.Before(st.Earliest) {
			st.Earliest = res.Expiry
		}
		log.V(1).Info("active wake", "key", e.Key, "by", e.By, "reason", e.Reason, "expiry", res.Expiry)
		if r.Recorder != nil {
			r.Recorder.Eventf(ss, corev1.EventTypeNormal, "WakeEntryAccepted",
				"wake entry %q accepted (by=%q reason=%q)", e.Key, e.By, e.Reason)
		}
		// Audit log only — the WakeEntryAccepted Event above already covers AC2.
		audit.Record(audit.NewContext(ctx, audit.Info{Who: e.By, Why: e.Reason}), nil, ss,
			"", "", "", "WakeOverride", fmt.Sprintf("wake entry %q active", e.Key))
	}

	if dirty {
		if err := r.Update(ctx, &cm); err != nil {
			return wakeState{}, err
		}
	}
	return st, nil
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
func statusChanged(cur nyxv1alpha1.SleepScheduleStatus, phase nyxv1alpha1.SleepSchedulePhase, next *metav1.Time, activeWakes int32) bool {
	if cur.Phase != phase || cur.ActiveWakes != activeWakes {
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
