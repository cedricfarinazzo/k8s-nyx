/*
Copyright 2026.

Licensed under the MIT License.
*/

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// These tests start two managers with leader election against the envtest API
// server and assert single-leader + failover behaviour.
var _ = Describe("Leader election", func() {
	newManager := func() manager.Manager {
		lease, renew, retry := 4*time.Second, 3*time.Second, 1*time.Second
		m, err := manager.New(cfg, manager.Options{
			Scheme:                        k8sClient.Scheme(),
			Metrics:                       metricsserver.Options{BindAddress: "0"},
			LeaderElection:                true,
			LeaderElectionID:              "k8s-nyx-le-test",
			LeaderElectionNamespace:       "default",
			LeaderElectionReleaseOnCancel: true,
			LeaseDuration:                 &lease,
			RenewDeadline:                 &renew,
			RetryPeriod:                   &retry,
		})
		Expect(err).NotTo(HaveOccurred())
		return m
	}

	elected := func(m manager.Manager) bool {
		select {
		case <-m.Elected():
			return true
		default:
			return false
		}
	}

	It("elects exactly one leader and fails over to a standby (AC1–AC3)", func() {
		mgr1, mgr2 := newManager(), newManager()
		ctx1, cancel1 := context.WithCancel(context.Background())
		ctx2, cancel2 := context.WithCancel(context.Background())
		defer cancel1()
		defer cancel2()

		go func() { defer GinkgoRecover(); _ = mgr1.Start(ctx1) }()
		go func() { defer GinkgoRecover(); _ = mgr2.Start(ctx2) }()

		leaderCount := func() int {
			n := 0
			if elected(mgr1) {
				n++
			}
			if elected(mgr2) {
				n++
			}
			return n
		}

		// AC1: exactly one becomes leader, and it stays exactly one.
		Eventually(leaderCount, "15s", "200ms").Should(Equal(1))
		Consistently(leaderCount, "2s", "200ms").Should(Equal(1), "two leaders at once")

		// Identify the leader and the standby.
		leaderCancel, standby := cancel1, mgr2
		if elected(mgr2) {
			leaderCancel, standby = cancel2, mgr1
		}
		Expect(elected(standby)).To(BeFalse(), "standby must not be leader yet")

		// AC2/AC3: when the leader stops, the standby acquires leadership (and is
		// the only leader). ReleaseOnCancel makes this happen within the retry
		// period, well inside the lease window.
		leaderCancel()
		Eventually(func() bool { return elected(standby) }, "15s", "200ms").Should(BeTrue())
		Consistently(func() bool { return elected(standby) }, "1s", "200ms").Should(BeTrue())
	})
})
