/*
Copyright 2026.

Licensed under the MIT License.
*/

package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

// AC4: the SleepSchedule type compiles, its CRD installs into envtest, and the
// no-op reconciler returns no error / no requeue for both existing and missing objects.
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
			Spec:       nyxv1alpha1.SleepScheduleSpec{Suspend: true},
		}
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())

		fetched := &nyxv1alpha1.SleepSchedule{}
		Expect(k8sClient.Get(ctx, nn, fetched)).To(Succeed())
		Expect(fetched.Spec.Suspend).To(BeTrue())
	})

	It("reconciles an existing resource without error or requeue (no-op)", func() {
		obj := &nyxv1alpha1.SleepSchedule{
			ObjectMeta: metav1.ObjectMeta{Name: resourceName, Namespace: namespace},
		}
		Expect(k8sClient.Create(ctx, obj)).To(Succeed())

		r := &SleepScheduleReconciler{Client: k8sClient, Scheme: k8sClient.Scheme()}
		res, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: nn})
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Requeue).To(BeFalse())
		Expect(res.RequeueAfter).To(BeZero())
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
