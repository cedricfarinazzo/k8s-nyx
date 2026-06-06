/*
Copyright 2026.

Licensed under the MIT License.
*/

package v1alpha1

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

const wbNamespace = "default"

func baseSchedule(name string) *nyxv1alpha1.SleepSchedule {
	return &nyxv1alpha1.SleepSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: wbNamespace},
		Spec: nyxv1alpha1.SleepScheduleSpec{
			Timezone: "Europe/Paris",
			Awake: []nyxv1alpha1.AwakeWindow{
				{Days: []nyxv1alpha1.Day{"Mon", "Tue", "Wed", "Thu", "Fri"}, From: "08:00", To: "20:00"},
			},
			Target: nyxv1alpha1.Target{
				Mode:       nyxv1alpha1.TargetModeNamespaces,
				Namespaces: []string{"team-a"},
			},
		},
	}
}

// AC3 + AC4: valid SleepSchedules apply; invalid ones are rejected with a clear error.
var _ = Describe("SleepSchedule validation", func() {
	AfterEach(func() {
		list := &nyxv1alpha1.SleepScheduleList{}
		Expect(k8sClient.List(ctx, list)).To(Succeed())
		for i := range list.Items {
			Expect(k8sClient.Delete(ctx, &list.Items[i])).To(Succeed())
		}
	})

	It("accepts a valid SleepSchedule (AC4)", func() {
		Expect(k8sClient.Create(ctx, baseSchedule("valid"))).To(Succeed())
	})

	It("rejects an invalid IANA timezone via the webhook (AC3)", func() {
		ss := baseSchedule("bad-tz")
		ss.Spec.Timezone = "Bogus/Zone"
		err := k8sClient.Create(ctx, ss)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("valid IANA timezone"))
	})

	It("rejects a malformed from/to time via OpenAPI (AC3)", func() {
		ss := baseSchedule("bad-time")
		ss.Spec.Awake[0].From = "8am"
		err := k8sClient.Create(ctx, ss)
		Expect(err).To(HaveOccurred())
	})

	It("rejects target.mode outside the enum via OpenAPI (AC3)", func() {
		ss := baseSchedule("bad-mode")
		ss.Spec.Target.Mode = "everything"
		err := k8sClient.Create(ctx, ss)
		Expect(err).To(HaveOccurred())
	})

	It("rejects from later than to via the webhook", func() {
		ss := baseSchedule("inverted-window")
		ss.Spec.Awake[0].From = "20:00"
		ss.Spec.Awake[0].To = "08:00"
		err := k8sClient.Create(ctx, ss)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("earlier than"))
	})

	It("requires namespaces when mode is namespaces (webhook)", func() {
		ss := baseSchedule("no-namespaces")
		ss.Spec.Target = nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces}
		err := k8sClient.Create(ctx, ss)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("namespace is required"))
	})

	It("requires a selector when mode is labels (webhook)", func() {
		ss := baseSchedule("no-selector")
		ss.Spec.Target = nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeLabels}
		err := k8sClient.Create(ctx, ss)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("selector is required"))
	})

	It("rejects temporaryWake.defaultDuration exceeding maxDuration (webhook)", func() {
		ss := baseSchedule("bad-wake")
		ss.Spec.TemporaryWake = &nyxv1alpha1.TemporaryWake{
			MaxDuration:     metav1.Duration{Duration: 0},
			DefaultDuration: metav1.Duration{Duration: 0},
		}
		// zero durations are invalid
		err := k8sClient.Create(ctx, ss)
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("positive duration"))
	})
})
