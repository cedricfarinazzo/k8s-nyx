//go:build e2e

/*
Copyright 2026.

Licensed under the MIT License.
*/

package e2e

import (
	"context"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

const (
	// workloadImage is the container used by every test workload; it runs forever
	// so suspend/replica toggles are observable (per the ticket: nginx alpine).
	workloadImage = "nginx:alpine"

	// sentinelKey mirrors the operator's unsatisfiable DaemonSet nodeSelector.
	sentinelKey   = "nyx.dev/asleep"
	sentinelValue = "true"

	// allowPVCDeletion opts a StatefulSet into being slept despite whenScaled=Delete.
	allowPVCDeletion = "nyx.dev/allow-pvc-deletion"

	awakeReplicas int32 = 2
	sleepReplicas int32 = 0
	hpaMin        int32 = 2
	hpaMax        int32 = 5
)

// Timeouts. The wait-for-wake budget tracks the chosen wake lead time; everything
// else is generous to absorb reconcile + image-pull latency on a cold kind node.
const (
	asleepTimeout = 120 * time.Second
	pollInterval  = 3 * time.Second
)

// durEnv reads a duration override (E2E_*), falling back to def — so the long
// CI waits (ticket: 5/7 min) can be shortened locally.
func durEnv(name string, def time.Duration) time.Duration {
	if v := os.Getenv(name); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// ---------------------------------------------------------------------------
// Live schedule windows ("compute sleep hour live")
// ---------------------------------------------------------------------------

// noonAnchoredTZ returns an Etc/GMT zone in which the current wall-clock time is
// ~12:00, so a window built at now+Δ (minutes) never crosses midnight or a
// weekday boundary — letting tests phrase short sleep/wake windows safely.
func noonAnchoredTZ(now time.Time) (string, *time.Location) {
	off := 12 - now.UTC().Hour() // hours to add to UTC to land near noon
	name := "Etc/GMT"
	switch {
	case off > 0:
		name = fmt.Sprintf("Etc/GMT-%d", off) // Etc/GMT sign is inverted: GMT-5 == UTC+5
	case off < 0:
		name = fmt.Sprintf("Etc/GMT+%d", -off)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return "UTC", time.UTC
	}
	return name, loc
}

// allDays is every weekday, so a noon-anchored [from,to) window applies today
// regardless of which day CI runs on.
var allDays = []nyxv1alpha1.Day{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

// liveAwakeWindow builds a schedule that is ASLEEP now and turns AWAKE wakeIn
// from now (staying awake for awakeFor). Returns the timezone and the window.
func liveAwakeWindow(wakeIn, awakeFor time.Duration) (string, nyxv1alpha1.AwakeWindow) {
	now := time.Now()
	tz, loc := noonAnchoredTZ(now)
	local := now.In(loc)
	from := local.Add(wakeIn)
	to := from.Add(awakeFor)
	return tz, nyxv1alpha1.AwakeWindow{
		Days: allDays,
		From: from.Format("15:04"),
		To:   to.Format("15:04"),
	}
}

// alwaysAsleepWindow builds a window that does not contain now, so targets sleep
// now and only wake after wakeIn — used by the override test where the wake is
// driven by a temporary override, not the window.
func alwaysAsleepWindow(wakeIn, awakeFor time.Duration) (string, nyxv1alpha1.AwakeWindow) {
	return liveAwakeWindow(wakeIn, awakeFor)
}

// ---------------------------------------------------------------------------
// Namespaces
// ---------------------------------------------------------------------------

func newNamespace(ctx context.Context) string {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{GenerateName: "nyx-e2e-"}}
	Expect(k8sClient.Create(ctx, ns)).To(Succeed())
	DeferCleanup(func() {
		_ = k8sClient.Delete(context.Background(), ns)
	})
	return ns.Name
}

// ---------------------------------------------------------------------------
// Workload builders (all on nginx:alpine)
// ---------------------------------------------------------------------------

func podLabels(app string) map[string]string { return map[string]string{"app": app} }

func podSpec() corev1.PodSpec {
	return corev1.PodSpec{
		Containers: []corev1.Container{{
			Name:  "nginx",
			Image: workloadImage,
			Ports: []corev1.ContainerPort{{ContainerPort: 80}},
		}},
	}
}

func newDeployment(ctx context.Context, ns, name string) *appsv1.Deployment {
	d := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(awakeReplicas),
			Selector: &metav1.LabelSelector{MatchLabels: podLabels(name)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels(name)},
				Spec:       podSpec(),
			},
		},
	}
	Expect(k8sClient.Create(ctx, d)).To(Succeed())
	return d
}

func newStatefulSet(
	ctx context.Context, ns, name string, whenScaledDelete bool, annotations map[string]string,
) *appsv1.StatefulSet {
	s := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns, Annotations: annotations},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    ptr(awakeReplicas),
			ServiceName: name,
			Selector:    &metav1.LabelSelector{MatchLabels: podLabels(name)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels(name)},
				Spec:       podSpec(),
			},
		},
	}
	if whenScaledDelete {
		s.Spec.PersistentVolumeClaimRetentionPolicy = &appsv1.StatefulSetPersistentVolumeClaimRetentionPolicy{
			WhenScaled:  appsv1.DeletePersistentVolumeClaimRetentionPolicyType,
			WhenDeleted: appsv1.RetainPersistentVolumeClaimRetentionPolicyType,
		}
	}
	Expect(k8sClient.Create(ctx, s)).To(Succeed())
	return s
}

func newDaemonSet(ctx context.Context, ns, name string) *appsv1.DaemonSet {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: podLabels(name)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels(name)},
				Spec:       podSpec(),
			},
		},
	}
	Expect(k8sClient.Create(ctx, ds)).To(Succeed())
	return ds
}

func newCronJob(ctx context.Context, ns, name string) *batchv1.CronJob {
	cj := &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: batchv1.CronJobSpec{
			Schedule: "*/5 * * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{Labels: podLabels(name)},
						Spec: func() corev1.PodSpec {
							p := podSpec()
							p.RestartPolicy = corev1.RestartPolicyNever
							return p
						}(),
					},
				},
			},
		},
	}
	Expect(k8sClient.Create(ctx, cj)).To(Succeed())
	return cj
}

func newJob(ctx context.Context, ns, name string) *batchv1.Job {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: batchv1.JobSpec{
			BackoffLimit: ptr(int32(0)),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels(name)},
				Spec: func() corev1.PodSpec {
					p := podSpec()
					p.RestartPolicy = corev1.RestartPolicyNever
					return p
				}(),
			},
		},
	}
	Expect(k8sClient.Create(ctx, job)).To(Succeed())
	return job
}

func newHPA(ctx context.Context, ns, name, targetDeploy string) *autoscalingv2.HorizontalPodAutoscaler {
	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1", Kind: "Deployment", Name: targetDeploy,
			},
			MinReplicas: ptr(hpaMin),
			MaxReplicas: hpaMax,
		},
	}
	Expect(k8sClient.Create(ctx, hpa)).To(Succeed())
	return hpa
}

// ---------------------------------------------------------------------------
// SleepSchedule + wake override
// ---------------------------------------------------------------------------

func newSchedule(
	ctx context.Context, ns, name, tz string, win nyxv1alpha1.AwakeWindow,
	kinds []string, tempWake *nyxv1alpha1.TemporaryWake,
) *nyxv1alpha1.SleepSchedule {
	ss := &nyxv1alpha1.SleepSchedule{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: nyxv1alpha1.SleepScheduleSpec{
			Timezone:      tz,
			Awake:         []nyxv1alpha1.AwakeWindow{win},
			Target:        nyxv1alpha1.Target{Mode: nyxv1alpha1.TargetModeNamespaces, Namespaces: []string{ns}},
			Kinds:         kinds,
			SleepReplicas: sleepReplicas,
			TemporaryWake: tempWake,
		},
	}
	Expect(k8sClient.Create(ctx, ss)).To(Succeed())
	return ss
}

// writeWakeOverride adds a data entry to the operator-owned <schedule>-wake
// ConfigMap (created by the operator). value is e.g. "+2m;by=e2e;reason=test".
func writeWakeOverride(ctx context.Context, ns, schedule, key, value string) {
	cmName := schedule + "-wake"
	Eventually(func() error {
		var cm corev1.ConfigMap
		if err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: cmName}, &cm); err != nil {
			return err
		}
		if cm.Data == nil {
			cm.Data = map[string]string{}
		}
		cm.Data[key] = value
		return k8sClient.Update(ctx, &cm)
	}, asleepTimeout, pollInterval).Should(Succeed(), "operator should have created the wake ConfigMap")
}

// ---------------------------------------------------------------------------
// Assertions (Eventually/Consistently friendly getters)
// ---------------------------------------------------------------------------

func get(ctx context.Context, obj client.Object) error {
	return k8sClient.Get(ctx, client.ObjectKeyFromObject(obj), obj)
}

func deployReplicas(ctx context.Context, ns, name string) func() int32 {
	return func() int32 {
		d := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		if err := get(ctx, d); err != nil || d.Spec.Replicas == nil {
			return -1
		}
		return *d.Spec.Replicas
	}
}

func stsReplicas(ctx context.Context, ns, name string) func() int32 {
	return func() int32 {
		s := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		if err := get(ctx, s); err != nil || s.Spec.Replicas == nil {
			return -1
		}
		return *s.Spec.Replicas
	}
}

func dsAsleep(ctx context.Context, ns, name string) func() bool {
	return func() bool {
		ds := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		if err := get(ctx, ds); err != nil {
			return false
		}
		return ds.Spec.Template.Spec.NodeSelector[sentinelKey] == sentinelValue
	}
}

func cronJobSuspended(ctx context.Context, ns, name string) func() bool {
	return func() bool {
		cj := &batchv1.CronJob{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		if err := get(ctx, cj); err != nil || cj.Spec.Suspend == nil {
			return false
		}
		return *cj.Spec.Suspend
	}
}

func jobSuspended(ctx context.Context, ns, name string) func() bool {
	return func() bool {
		job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		if err := get(ctx, job); err != nil || job.Spec.Suspend == nil {
			return false
		}
		return *job.Spec.Suspend
	}
}

// hpaNeutralized reports whether the HPA min==max==1 (the operator's asleep state).
func hpaNeutralized(ctx context.Context, ns, name string) func() bool {
	return func() bool {
		hpa := &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		if err := get(ctx, hpa); err != nil || hpa.Spec.MinReplicas == nil {
			return false
		}
		return *hpa.Spec.MinReplicas == 1 && hpa.Spec.MaxReplicas == 1
	}
}

func hpaRestored(ctx context.Context, ns, name string) func() bool {
	return func() bool {
		hpa := &autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		if err := get(ctx, hpa); err != nil || hpa.Spec.MinReplicas == nil {
			return false
		}
		return *hpa.Spec.MinReplicas == hpaMin && hpa.Spec.MaxReplicas == hpaMax
	}
}

func ptr[T any](v T) *T { return &v }

func metav1Duration(d time.Duration) metav1.Duration { return metav1.Duration{Duration: d} }
