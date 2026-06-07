//go:build e2e

/*
Copyright 2026.

Licensed under the MIT License.
*/

package e2e

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
	"github.com/cedricfarinazzo/k8s-nyx/internal/workload"
)

// Wake budgets. The window opens wakeIn from now; allow generous slack on top for
// reconcile + cold-node latency.
var (
	t1Wake       = durEnv("E2E_T1_WAKE", 5*time.Minute)
	t2Wake       = durEnv("E2E_T2_WAKE", 5*time.Minute)
	t5Window     = durEnv("E2E_T5_WINDOW", 10*time.Minute)
	t5OverrideAt = durEnv("E2E_T5_OVERRIDE_AT", 2*time.Minute)
	wakeSlack    = 150 * time.Second
)

// kindSet is the full set of workloads provisioned in one namespace.
type kindSet struct {
	ns     string
	deploy string
	sts    string
	ds     string
	cron   string
	job    string
	hpa    string
}

// provisionAllKinds creates one of every supported workload kind in ns (Deployment,
// StatefulSet, DaemonSet, CronJob, Job, HPA), all on nginx:alpine.
func provisionAllKinds(ctx context.Context, ns string) kindSet {
	ks := kindSet{ns: ns, deploy: "web", sts: "db", ds: "agent", cron: "report", job: "batch", hpa: "web-hpa"}
	newDeployment(ctx, ns, ks.deploy)
	newStatefulSet(ctx, ns, ks.sts, false, nil)
	newDaemonSet(ctx, ns, ks.ds)
	newCronJob(ctx, ns, ks.cron)
	newJob(ctx, ns, ks.job)
	newHPA(ctx, ns, ks.hpa, ks.deploy)
	return ks
}

func expectAllAsleep(ctx context.Context, ks kindSet, timeout time.Duration) {
	Eventually(deployReplicas(ctx, ks.ns, ks.deploy), timeout, pollInterval).
		Should(Equal(sleepReplicas), "deployment asleep")
	Eventually(stsReplicas(ctx, ks.ns, ks.sts), timeout, pollInterval).
		Should(Equal(sleepReplicas), "statefulset asleep")
	Eventually(dsAsleep(ctx, ks.ns, ks.ds), timeout, pollInterval).Should(BeTrue(), "daemonset sentinel injected")
	Eventually(cronJobSuspended(ctx, ks.ns, ks.cron), timeout, pollInterval).Should(BeTrue(), "cronjob suspended")
	Eventually(jobSuspended(ctx, ks.ns, ks.job), timeout, pollInterval).Should(BeTrue(), "job suspended")
	Eventually(hpaNeutralized(ctx, ks.ns, ks.hpa), timeout, pollInterval).Should(BeTrue(), "hpa neutralized")
}

func expectAllAwakeRestored(ctx context.Context, ks kindSet, timeout time.Duration) {
	Eventually(deployReplicas(ctx, ks.ns, ks.deploy), timeout, pollInterval).
		Should(Equal(awakeReplicas), "deployment restored exactly")
	Eventually(stsReplicas(ctx, ks.ns, ks.sts), timeout, pollInterval).
		Should(Equal(awakeReplicas), "statefulset restored exactly")
	Eventually(dsAsleep(ctx, ks.ns, ks.ds), timeout, pollInterval).Should(BeFalse(), "daemonset sentinel removed")
	Eventually(cronJobSuspended(ctx, ks.ns, ks.cron), timeout, pollInterval).Should(BeFalse(), "cronjob unsuspended")
	Eventually(jobSuspended(ctx, ks.ns, ks.job), timeout, pollInterval).Should(BeFalse(), "job unsuspended")
	Eventually(hpaRestored(ctx, ks.ns, ks.hpa), timeout, pollInterval).Should(BeTrue(), "hpa min/max restored")
}

var _ = Describe("sleep / wake / restore across kinds", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	// Test 1 — full lifecycle in a single namespace (AC1, AC3).
	It("sleeps every kind, then wakes and restores exactly", func() {
		ns := newNamespace(ctx)
		ks := provisionAllKinds(ctx, ns)
		tz, win, _ := liveAwakeWindow(t1Wake, time.Hour)
		newSchedule(ctx, ns, "lifecycle", tz, win, nil, nil)

		By("all kinds asleep shortly after creation")
		expectAllAsleep(ctx, ks, asleepTimeout)

		By("all kinds awake and restored once the window opens")
		expectAllAwakeRestored(ctx, ks, t1Wake+wakeSlack)
	})

	// Test 2 — same lifecycle across multiple namespaces, independent schedules (AC1).
	It("handles multiple namespaces independently", func() {
		var sets []kindSet
		for i := 0; i < 3; i++ {
			ns := newNamespace(ctx)
			ks := provisionAllKinds(ctx, ns)
			tz, win, _ := liveAwakeWindow(t2Wake, time.Hour)
			newSchedule(ctx, ns, "lifecycle", tz, win, nil, nil)
			sets = append(sets, ks)
		}
		By("every namespace sleeps")
		for _, ks := range sets {
			expectAllAsleep(ctx, ks, asleepTimeout)
		}
		By("every namespace wakes and restores")
		for _, ks := range sets {
			expectAllAwakeRestored(ctx, ks, t2Wake+wakeSlack)
		}
	})

	// Test 3 — kind targeting: only listed kinds sleep; the rest stay up (AC1).
	It("only sleeps the kinds in spec.kinds and leaves the others up", func() {
		ns := newNamespace(ctx)
		ks := provisionAllKinds(ctx, ns)
		tz, win, _ := liveAwakeWindow(time.Hour, time.Hour) // far future: stays asleep for the whole test
		newSchedule(ctx, ns, "partial", tz, win,
			[]string{workload.KindDeployment, workload.KindCronJob}, nil)

		By("targeted kinds sleep")
		Eventually(deployReplicas(ctx, ns, ks.deploy), asleepTimeout, pollInterval).Should(Equal(sleepReplicas))
		Eventually(cronJobSuspended(ctx, ns, ks.cron), asleepTimeout, pollInterval).Should(BeTrue())

		By("non-targeted kinds stay up")
		Consistently(stsReplicas(ctx, ns, ks.sts), 30*time.Second, pollInterval).Should(Equal(awakeReplicas))
		Consistently(dsAsleep(ctx, ns, ks.ds), 30*time.Second, pollInterval).Should(BeFalse())
		Consistently(jobSuspended(ctx, ns, ks.job), 30*time.Second, pollInterval).Should(BeFalse())
		Consistently(hpaNeutralized(ctx, ns, ks.hpa), 30*time.Second, pollInterval).Should(BeFalse())
	})

	// Test 4 — a StatefulSet with whenScaled=Delete and no opt-in must NOT be slept
	// (data-loss guard); a sibling that opts in via annotation IS slept (AC1).
	It("refuses to sleep a whenScaled=Delete StatefulSet unless opted in", func() {
		ns := newNamespace(ctx)
		newStatefulSet(ctx, ns, "guarded", true, nil)
		newStatefulSet(ctx, ns, "optedin", true, map[string]string{allowPVCDeletion: "true"})
		tz, win, _ := liveAwakeWindow(time.Hour, time.Hour)
		newSchedule(ctx, ns, "pvc-guard", tz, win, []string{workload.KindStatefulSet}, nil)

		By("opted-in StatefulSet sleeps")
		Eventually(stsReplicas(ctx, ns, "optedin"), asleepTimeout, pollInterval).Should(Equal(sleepReplicas))
		By("guarded StatefulSet stays up (PVC deletion risk)")
		Consistently(stsReplicas(ctx, ns, "guarded"), 60*time.Second, pollInterval).Should(Equal(awakeReplicas))
	})

	// Test 5 — temporary wake override: asleep, then a +duration override forces it
	// awake, then it sleeps again when the override expires, and finally wakes on
	// its own when the scheduled awake window opens (AC1: up, down, then back up on
	// schedule).
	It("honours a temporary wake override, sleeps again on expiry, then wakes on schedule", func() {
		ns := newNamespace(ctx)
		dep := "web"
		newDeployment(ctx, ns, dep)
		tz, win, opensAt := liveAwakeWindow(t5Window, time.Hour)
		newSchedule(ctx, ns, "override", tz, win, []string{workload.KindDeployment},
			&nyxv1alpha1.TemporaryWake{
				DefaultDuration: metav1Duration(2 * time.Minute),
				MaxDuration:     metav1Duration(10 * time.Minute),
			})

		By("asleep first")
		Eventually(deployReplicas(ctx, ns, dep), asleepTimeout, pollInterval).Should(Equal(sleepReplicas))

		By("triggering a +2m override after a short delay")
		time.Sleep(t5OverrideAt)
		writeWakeOverride(ctx, ns, "override", "+2m")

		By("forced awake by the override")
		Eventually(deployReplicas(ctx, ns, dep), wakeSlack, pollInterval).Should(Equal(awakeReplicas))

		By("asleep again once the override expires")
		Eventually(deployReplicas(ctx, ns, dep), 4*time.Minute, pollInterval).Should(Equal(sleepReplicas))

		// It must wake AT the scheduled window, not merely some time later: stay
		// asleep right up to the edge, then wake within a reconcile of it. The
		// Consistently guards against an early wake; the Eventually against a late one.
		By("stays asleep right up to the scheduled window opening (no early wake)")
		if until := time.Until(opensAt) - 20*time.Second; until > 0 {
			Consistently(deployReplicas(ctx, ns, dep), until, pollInterval).Should(Equal(sleepReplicas))
		}

		By("wakes within a reconcile of the scheduled window opening")
		Eventually(deployReplicas(ctx, ns, dep), 90*time.Second, pollInterval).Should(Equal(awakeReplicas))
	})
})
