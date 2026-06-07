//go:build e2e

/*
Copyright 2026.

Licensed under the MIT License.
*/

// Package e2e holds the kind-based end-to-end suite (VC-161). It runs against a
// real cluster (the operator already installed via Helm) and exercises
// sleep/wake/restore across every supported workload kind. It is build-tagged
// `e2e` so `make test` (unit + envtest) never picks it up; run it with
// `make test-e2e`.
package e2e

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

var (
	k8sClient client.Client
	scheme    = runtime.NewScheme()
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "k8s-nyx e2e suite")
}

// BeforeSuite runs once per Ginkgo process. It builds a client from the ambient
// kubeconfig (KUBECONFIG / in-cluster) — the same resolution controller-runtime
// uses — and confirms the operator's CRD is installed before any spec runs.
var _ = BeforeSuite(func() {
	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(autoscalingv2.AddToScheme(scheme)).To(Succeed())
	Expect(nyxv1alpha1.AddToScheme(scheme)).To(Succeed())

	cfg, err := ctrl.GetConfig()
	Expect(err).NotTo(HaveOccurred(), "no kubeconfig — the e2e suite needs a running cluster (see make test-e2e)")

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())

	// Sanity: the SleepSchedule CRD must be installed (operator deployed).
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var list nyxv1alpha1.SleepScheduleList
	Expect(k8sClient.List(ctx, &list)).To(Succeed(),
		"SleepSchedule CRD not found — install the operator (helm install) before running the e2e suite")
})
