/*
Copyright 2026.

Licensed under the MIT License.
*/

package controller

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func validSpec() nyxv1alpha1.SleepScheduleSpec {
	return nyxv1alpha1.SleepScheduleSpec{
		Timezone: "Europe/Paris",
		Awake: []nyxv1alpha1.AwakeWindow{
			{Days: []nyxv1alpha1.Day{"Mon", "Tue"}, From: "08:00", To: "20:00"},
		},
		Target: nyxv1alpha1.Target{
			Mode:       nyxv1alpha1.TargetModeNamespaces,
			Namespaces: []string{"default"},
		},
	}
}

// The CRD installs into envtest; the reconciler evaluates the schedule, records
// phase + nextTransition in status, and requeues at the next boundary (AC3).
var _ = Describe("SleepSchedule controller", func() {
	const (
		resourceName = "test-sleepschedule"
		namespace    = "default"
	)

	nn := types.NamespacedName{Name: resourceName, Namespace: namespace}

	AfterEach(func() {
		obj := &nyxv1alpha1.SleepSchedule{}
		if err := k8sClient.Get(ctx, nn, obj); err == nil {
			Expect(k8sClient.Delete(ctx, obj)).To(Succeed())
		}
		// envtest has no GC controller, so clean the owned Wake ConfigMap explicitly.
		cm := &corev1.ConfigMap{}
		cmNN := types.NamespacedName{Namespace: namespace, Name: WakeConfigMapName(resourceName)}
		if err := k8sClient.Get(ctx, cmNN, cm); err == nil {
			Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
		}
		// Clean any workload + checkpoint Secret a test created, so specs stay isolated.
		_ = k8sClient.Delete(ctx, &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "web"}})
		_ = k8sClient.Delete(ctx, &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "agent"}})
		_ = k8sClient.Delete(ctx, &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "report"}})
		_ = k8sClient.Delete(ctx, &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "web-hpa"}})
		_ = k8sClient.Delete(ctx, &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "db-pvc"}})
		_ = k8sClient.Delete(ctx, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: resourceName + "-checkpoint"}})
	})

	It("creates and reads a SleepSchedule (CRD is installed)", func() {
		obj := &nyxv1alpha1.SleepSchedule{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec:       validSpec(),
		}
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())

		fetched := &nyxv1alpha1.SleepSchedule{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
		Expect(fetched.Spec.Timezone).To(Equal("Europe/Paris"))
		Expect(fetched.Spec.Awake).To(HaveLen(1))
	})

	It("sets status phase + nextTransition and requeues (AC3)", func() {
		obj := &nyxv1alpha1.SleepSchedule{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec:       validSpec(), // Mon/Tue 08:00-20:00 Europe/Paris
		}
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())

		paris, err := time.LoadLocation("Europe/Paris")
		Expect(err).NotTo(HaveOccurred())
		// Monday 10:00 → Awake, next transition is Monday 20:00.
		fixedNow := time.Date(2026, 6, 1, 10, 0, 0, 0, paris)

		r := &SleepScheduleReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
			Now:    func() time.Time { return fixedNow },
		}
		res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.RequeueAfter).To(BeNumerically(">", 0))

		fetched := &nyxv1alpha1.SleepSchedule{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
		Expect(fetched.Status.Phase).To(Equal(nyxv1alpha1.PhaseAwake))
		Expect(fetched.Status.NextTransition).NotTo(BeNil())
		Expect(fetched.Status.NextTransition.Time).To(BeTemporally("==", time.Date(2026, 6, 1, 20, 0, 0, 0, paris)))
	})

	It("is idempotent — a second same-time reconcile writes nothing (AC4)", func() {
		obj := &nyxv1alpha1.SleepSchedule{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec:       validSpec(),
		}
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())

		paris, err := time.LoadLocation("Europe/Paris")
		Expect(err).NotTo(HaveOccurred())
		fixedNow := time.Date(2026, 6, 1, 10, 0, 0, 0, paris)
		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Now: func() time.Time { return fixedNow }}

		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		after1 := &nyxv1alpha1.SleepSchedule{}
		Expect(k8sClient.Get(ctx, nn, after1)).To(Succeed())
		rv1 := after1.ResourceVersion

		// A second reconcile at the same instant must not write anything.
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		after2 := &nyxv1alpha1.SleepSchedule{}
		Expect(k8sClient.Get(ctx, nn, after2)).To(Succeed())
		Expect(after2.ResourceVersion).To(Equal(rv1), "second reconcile should not write status")
	})

	It("ensures the Wake ConfigMap with an owner reference (AC1)", func() {
		obj := &nyxv1alpha1.SleepSchedule{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec:       validSpec(),
		}
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())

		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		cm := &corev1.ConfigMap{}
		cmNN := types.NamespacedName{Namespace: namespace, Name: WakeConfigMapName(resourceName)}
		Expect(k8sClient.Get(ctx, cmNN, cm)).To(Succeed())

		created := &nyxv1alpha1.SleepSchedule{}
		Expect(k8sClient.Get(ctx, nn, created)).To(Succeed())
		Expect(cm.OwnerReferences).To(HaveLen(1))
		Expect(cm.OwnerReferences[0].UID).To(Equal(created.UID))
		Expect(cm.OwnerReferences[0].Kind).To(Equal("SleepSchedule"))
		Expect(cm.OwnerReferences[0].Controller).NotTo(BeNil())
		Expect(*cm.OwnerReferences[0].Controller).To(BeTrue())
	})

	It("recreates the Wake ConfigMap if it is deleted (AC3)", func() {
		obj := &nyxv1alpha1.SleepSchedule{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec:       validSpec(),
		}
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())

		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		cmNN := types.NamespacedName{Namespace: namespace, Name: WakeConfigMapName(resourceName)}
		cm := &corev1.ConfigMap{}
		Expect(k8sClient.Get(ctx, cmNN, cm)).To(Succeed())
		Expect(k8sClient.Delete(ctx, cm)).To(Succeed())
		Expect(k8sClient.Get(ctx, cmNN, cm)).NotTo(Succeed())

		// Next reconcile recreates it.
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(k8sClient.Get(ctx, cmNN, cm)).To(Succeed())
	})

	It("warns on malformed wake entries and audits accepted ones (AC3/AC4)", func() {
		obj := &nyxv1alpha1.SleepSchedule{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec:       validSpec(),
		}
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())

		rec := record.NewFakeRecorder(20)
		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Recorder: rec}
		// First reconcile creates the Wake ConfigMap.
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		// Write a valid and a malformed entry into it.
		cm := &corev1.ConfigMap{}
		cmNN := types.NamespacedName{Namespace: namespace, Name: WakeConfigMapName(resourceName)}
		Expect(k8sClient.Get(ctx, cmNN, cm)).To(Succeed())
		cm.Data = map[string]string{
			"alice-1": "2099-01-01T00:00:00Z;by=alice;reason=debug", // future → active, audited
			"bad-1":   "not-a-time",
		}
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())

		var ss nyxv1alpha1.SleepSchedule
		Expect(k8sClient.Get(ctx, nn, &ss)).To(Succeed())
		Expect(func() error { _, e := r.processWakeEntries(ctx, &ss, r.now()); return e }()).To(Succeed())

		var events []string
		Eventually(func() int { return len(rec.Events) }, "2s", "50ms").Should(BeNumerically(">=", 2))
		for len(rec.Events) > 0 {
			events = append(events, <-rec.Events)
		}
		var warned, audited bool
		for _, e := range events {
			if containsAll(e, "Warning", "MalformedWakeEntry", "bad-1") {
				warned = true
			}
			if containsAll(e, "Normal", "WakeEntryAccepted", "alice-1", "alice") {
				audited = true
			}
		}
		Expect(warned).To(BeTrue(), "expected a Warning naming the bad key")
		Expect(audited).To(BeTrue(), "expected a Normal audit event with by/reason")
	})

	It("warns and continues when spec.kinds includes a kind with no handler (AC2)", func() {
		obj := &nyxv1alpha1.SleepSchedule{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec:       validSpec(),
		}
		obj.Spec.Kinds = []string{"Deployment", "ReplicaSet"} // ReplicaSet has no handler yet
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())

		rec := record.NewFakeRecorder(20)
		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Recorder: rec}
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred()) // reconcile continues without error

		var events []string
		Eventually(func() int { return len(rec.Events) }, "2s", "50ms").Should(BeNumerically(">=", 1))
		for len(rec.Events) > 0 {
			events = append(events, <-rec.Events)
		}
		var warned bool
		for _, e := range events {
			if containsAll(e, "Warning", "UnhandledKind", "ReplicaSet") {
				warned = true
			}
		}
		Expect(warned).To(BeTrue(), "expected a Warning naming the unhandled kind")
	})

	withTempWake := func() *nyxv1alpha1.SleepSchedule {
		obj := &nyxv1alpha1.SleepSchedule{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec:       validSpec(),
		}
		obj.Spec.TemporaryWake = &nyxv1alpha1.TemporaryWake{
			MaxDuration:     metav1.Duration{Duration: 8 * time.Hour},
			DefaultDuration: metav1.Duration{Duration: time.Hour},
		}
		return obj
	}
	setWakeData := func(data map[string]string) {
		cm := &corev1.ConfigMap{}
		cmNN := types.NamespacedName{Namespace: namespace, Name: WakeConfigMapName(resourceName)}
		Expect(k8sClient.Get(ctx, cmNN, cm)).To(Succeed())
		cm.Data = data
		Expect(k8sClient.Update(ctx, cm)).To(Succeed())
	}
	getWakeData := func() map[string]string {
		cm := &corev1.ConfigMap{}
		cmNN := types.NamespacedName{Namespace: namespace, Name: WakeConfigMapName(resourceName)}
		Expect(k8sClient.Get(ctx, cmNN, cm)).To(Succeed())
		return cm.Data
	}

	It("stamps +duration to absolute exactly once (AC1)", func() {
		Expect(k8sClient.Create(ctx, withTempWake())).To(Succeed())
		now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Now: func() time.Time { return now }}
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		setWakeData(map[string]string{"bob": "+1h;by=bob"})
		var ss nyxv1alpha1.SleepSchedule
		Expect(k8sClient.Get(ctx, nn, &ss)).To(Succeed())
		Expect(func() error { _, e := r.processWakeEntries(ctx, &ss, r.now()); return e }()).To(Succeed())

		want := now.Add(time.Hour).UTC().Format(time.RFC3339)
		Expect(getWakeData()["bob"]).To(Equal(want + ";by=bob"))

		// A second pass at a LATER time must not re-extend the now-absolute value.
		r.Now = func() time.Time { return now.Add(30 * time.Minute) }
		Expect(func() error { _, e := r.processWakeEntries(ctx, &ss, r.now()); return e }()).To(Succeed())
		Expect(getWakeData()["bob"]).To(Equal(want+";by=bob"), "absolute value must not re-extend")
	})

	It("applies defaultDuration to a no-expiry entry (AC2)", func() {
		Expect(k8sClient.Create(ctx, withTempWake())).To(Succeed())
		now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Now: func() time.Time { return now }}
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		setWakeData(map[string]string{"alice": "by=alice;reason=lunch"})
		var ss nyxv1alpha1.SleepSchedule
		Expect(k8sClient.Get(ctx, nn, &ss)).To(Succeed())
		Expect(func() error { _, e := r.processWakeEntries(ctx, &ss, r.now()); return e }()).To(Succeed())

		want := now.Add(time.Hour).UTC().Format(time.RFC3339) // default 1h
		Expect(getWakeData()["alice"]).To(Equal(want + ";by=alice;reason=lunch"))
	})

	It("clamps an over-cap expiry and records an Event (AC3)", func() {
		Expect(k8sClient.Create(ctx, withTempWake())).To(Succeed())
		now := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
		rec := record.NewFakeRecorder(20)
		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), Recorder: rec, Now: func() time.Time { return now }}
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())

		over := now.Add(100 * time.Hour).UTC().Format(time.RFC3339)
		setWakeData(map[string]string{"big": over})
		var ss nyxv1alpha1.SleepSchedule
		Expect(k8sClient.Get(ctx, nn, &ss)).To(Succeed())
		Expect(func() error { _, e := r.processWakeEntries(ctx, &ss, r.now()); return e }()).To(Succeed())

		want := now.Add(8 * time.Hour).UTC().Format(time.RFC3339) // clamped to max
		Expect(getWakeData()["big"]).To(Equal(want))

		var clamped bool
		for len(rec.Events) > 0 {
			if e := <-rec.Events; containsAll(e, "WakeClamped", "big") {
				clamped = true
			}
		}
		Expect(clamped).To(BeTrue(), "expected a WakeClamped Event")
	})

	It("forces awake on an active wake, then re-sleeps when it expires (AC1–AC4)", func() {
		// Saturday → outside the Mon/Tue awake window, so the schedule is Asleep.
		now := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
		sched := withTempWake() // sleepReplicas 0, target namespaces [default]
		Expect(k8sClient.Create(ctx, sched)).To(Succeed())

		reps := int32(3)
		web := &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "web"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &reps,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "web"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "web"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
				},
			},
		}
		Expect(k8sClient.Create(ctx, web)).To(Succeed())

		webReplicas := func() int32 {
			d := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "web"}, d)).To(Succeed())
			return *d.Spec.Replicas
		}
		phase := func() nyxv1alpha1.SleepSchedulePhase {
			s := &nyxv1alpha1.SleepSchedule{}
			Expect(k8sClient.Get(ctx, nn, s)).To(Succeed())
			return s.Status.Phase
		}
		activeWakes := func() int32 {
			s := &nyxv1alpha1.SleepSchedule{}
			Expect(k8sClient.Get(ctx, nn, s)).To(Succeed())
			return s.Status.ActiveWakes
		}

		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), OperatorNamespace: namespace, Now: func() time.Time { return now }}

		// 1) Asleep: scaled to sleepReplicas, phase Asleep, no active wakes.
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(webReplicas()).To(Equal(int32(0)))
		Expect(phase()).To(Equal(nyxv1alpha1.PhaseAsleep))
		Expect(activeWakes()).To(Equal(int32(0)))

		// 2) Active wake → forced awake, restored to 3, phase WokenByOverride, count 1.
		setWakeData(map[string]string{"w1": now.Add(time.Hour).UTC().Format(time.RFC3339) + ";by=alice"})
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(webReplicas()).To(Equal(int32(3)))
		Expect(phase()).To(Equal(nyxv1alpha1.PhaseWokenByOverride))
		Expect(activeWakes()).To(Equal(int32(1)))

		// 3) After expiry → entry self-cleaned, re-slept, phase Asleep, count 0.
		r.Now = func() time.Time { return now.Add(2 * time.Hour) }
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(webReplicas()).To(Equal(int32(0)))
		Expect(phase()).To(Equal(nyxv1alpha1.PhaseAsleep))
		Expect(activeWakes()).To(Equal(int32(0)))
		_, present := getWakeData()["w1"]
		Expect(present).To(BeFalse(), "expired entry should be removed from the ConfigMap")
	})

	It("sleeps and restores a DaemonSet via the sentinel nodeSelector (VC-137)", func() {
		paris, err := time.LoadLocation("Europe/Paris")
		Expect(err).NotTo(HaveOccurred())
		sched := &nyxv1alpha1.SleepSchedule{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec:       validSpec(), // Mon/Tue 08:00-20:00; default kinds include DaemonSet
		}
		Expect(k8sClient.Create(ctx, sched)).To(Succeed())

		ds := &appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "agent"},
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "agent"}},
					Spec: corev1.PodSpec{
						NodeSelector: map[string]string{"disktype": "ssd"},
						Containers:   []corev1.Container{{Name: "c", Image: "busybox"}},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, ds)).To(Succeed())

		nodeSel := func() map[string]string {
			d := &appsv1.DaemonSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "agent"}, d)).To(Succeed())
			return d.Spec.Template.Spec.NodeSelector
		}

		// Saturday → asleep: sentinel injected (0 pods schedule), original kept.
		asleep := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), OperatorNamespace: namespace, Now: func() time.Time { return asleep }}
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(nodeSel()).To(HaveKeyWithValue("nyx.dev/asleep", "true"))
		Expect(nodeSel()).To(HaveKeyWithValue("disktype", "ssd"))

		// Monday 10:00 → awake: exact original nodeSelector restored, sentinel gone.
		r.Now = func() time.Time { return time.Date(2026, 6, 1, 10, 0, 0, 0, paris) }
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(nodeSel()).To(Equal(map[string]string{"disktype": "ssd"}))
	})

	It("sleeps and restores a CronJob via spec.suspend (VC-138)", func() {
		paris, err := time.LoadLocation("Europe/Paris")
		Expect(err).NotTo(HaveOccurred())
		sched := &nyxv1alpha1.SleepSchedule{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec:       validSpec(), // default kinds include CronJob
		}
		Expect(k8sClient.Create(ctx, sched)).To(Succeed())

		cj := &batchv1.CronJob{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "report"},
			Spec: batchv1.CronJobSpec{
				Schedule: "*/5 * * * *",
				JobTemplate: batchv1.JobTemplateSpec{
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyNever,
								Containers:    []corev1.Container{{Name: "c", Image: "busybox"}},
							},
						},
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, cj)).To(Succeed())

		suspend := func() *bool {
			c := &batchv1.CronJob{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "report"}, c)).To(Succeed())
			return c.Spec.Suspend
		}

		// Saturday → asleep: suspended.
		asleep := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), OperatorNamespace: namespace, Now: func() time.Time { return asleep }}
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(suspend()).NotTo(BeNil())
		Expect(*suspend()).To(BeTrue())

		// Monday 10:00 → awake: restored to the exact original. The API server
		// defaults CronJob spec.suspend to false on create, so the checkpointed
		// original is false (not nil) — restore brings it back to false.
		r.Now = func() time.Time { return time.Date(2026, 6, 1, 10, 0, 0, 0, paris) }
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(suspend()).NotTo(BeNil())
		Expect(*suspend()).To(BeFalse())
	})

	It("neutralizes and restores an HPA min/max (VC-139)", func() {
		paris, err := time.LoadLocation("Europe/Paris")
		Expect(err).NotTo(HaveOccurred())
		sched := &nyxv1alpha1.SleepSchedule{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec:       validSpec(), // default kinds include HorizontalPodAutoscaler; sleepReplicas 0
		}
		Expect(k8sClient.Create(ctx, sched)).To(Succeed())

		minTwo := int32(2)
		hpa := &autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "web-hpa"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "web", APIVersion: "apps/v1"},
				MinReplicas:    &minTwo,
				MaxReplicas:    10,
			},
		}
		Expect(k8sClient.Create(ctx, hpa)).To(Succeed())

		get := func() *autoscalingv2.HorizontalPodAutoscaler {
			h := &autoscalingv2.HorizontalPodAutoscaler{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "web-hpa"}, h)).To(Succeed())
			return h
		}

		// Saturday → asleep: sleepReplicas 0 → min/max clamped to 1/1 (AC1 + AC3).
		asleep := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), OperatorNamespace: namespace, Now: func() time.Time { return asleep }}
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		h := get()
		Expect(h.Spec.MinReplicas).NotTo(BeNil())
		Expect(*h.Spec.MinReplicas).To(Equal(int32(1)))
		Expect(h.Spec.MaxReplicas).To(Equal(int32(1)))

		// Monday 10:00 → awake: exact original min/max restored (AC2).
		r.Now = func() time.Time { return time.Date(2026, 6, 1, 10, 0, 0, 0, paris) }
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		h = get()
		Expect(*h.Spec.MinReplicas).To(Equal(int32(2)))
		Expect(h.Spec.MaxReplicas).To(Equal(int32(10)))
	})

	It("refuses to sleep a whenScaled=Delete StatefulSet unless opted in (VC-140)", func() {
		sched := &nyxv1alpha1.SleepSchedule{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
			Spec:       validSpec(), // default kinds include StatefulSet
		}
		Expect(k8sClient.Create(ctx, sched)).To(Succeed())

		reps := int32(3)
		sts := &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: "db-pvc"},
			Spec: appsv1.StatefulSetSpec{
				ServiceName: "db",
				Replicas:    &reps,
				Selector:    &metav1.LabelSelector{MatchLabels: map[string]string{"app": "db"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "db"}},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: "busybox"}}},
				},
				PersistentVolumeClaimRetentionPolicy: &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
					WhenScaled:  appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
					WhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
				},
			},
		}
		Expect(k8sClient.Create(ctx, sts)).To(Succeed())

		stsReplicas := func() int32 {
			s := &appsv1.StatefulSet{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "db-pvc"}, s)).To(Succeed())
			return *s.Spec.Replicas
		}

		// Saturday → asleep, but the Delete policy with no opt-in blocks the sleep.
		asleep := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
		rec := record.NewFakeRecorder(20)
		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme(), OperatorNamespace: namespace, Recorder: rec, Now: func() time.Time { return asleep }}
		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(stsReplicas()).To(Equal(int32(3)), "must not be slept without opt-in")
		var warned bool
		Eventually(func() int { return len(rec.Events) }, "2s", "50ms").Should(BeNumerically(">=", 1))
		for len(rec.Events) > 0 {
			if e := <-rec.Events; containsAll(e, "Warning", "PVCDeletionRisk") {
				warned = true
			}
		}
		Expect(warned).To(BeTrue(), "expected a PVCDeletionRisk Warning")

		// Opt in via the annotation → now it sleeps.
		Expect(k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: "db-pvc"}, sts)).To(Succeed())
		sts.Annotations = map[string]string{"nyx.dev/allow-pvc-deletion": "true"}
		Expect(k8sClient.Update(ctx, sts)).To(Succeed())
		_, err = r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(stsReplicas()).To(Equal(int32(0)), "opted in → slept")
	})

	It("reconciles a missing resource without error (no-op)", func() {
		obj := &nyxv1alpha1.SleepSchedule{}
		err := k8sClient.Get(ctx, nn, obj)
		Expect(errors.IsNotFound(err)).To(BeTrue())

		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(reconcile.Result{}))
	})
})
