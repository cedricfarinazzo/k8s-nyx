/*
Copyright 2026.

Licensed under the MIT License.
*/

package controller

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

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
