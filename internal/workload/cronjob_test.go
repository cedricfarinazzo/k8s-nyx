/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import (
	"context"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
)

func newCronJob(ns, name string, suspend *bool) *batchv1.CronJob {
	return &batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID(name + "-uid")},
		Spec:       batchv1.CronJobSpec{Suspend: suspend},
	}
}

func cjSuspend(t *testing.T, c client.Client, name string) *bool {
	t.Helper()
	var cj batchv1.CronJob
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "team-a", Name: name}, &cj); err != nil {
		t.Fatalf("get cronjob: %v", err)
	}
	return cj.Spec.Suspend
}

// AC1 (sleep): an originally-unset CronJob is suspended and the prior value (null)
// is checkpointed.
func TestCronJob_SleepSuspendsAndCheckpoints(t *testing.T) {
	s, c := newSleeper(newCronJob("team-a", "report", nil))
	sch := schedule()
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{{Kind: KindCronJob, Namespace: "team-a", Name: "report"}}); err != nil {
		t.Fatal(err)
	}
	if sus := cjSuspend(t, c, "report"); sus == nil || !*sus {
		t.Fatalf("suspend = %v, want true", sus)
	}
	key := checkpoint.Key("CronJob", "team-a", "report", "report-uid")
	raw, found, err := s.Store.GetRaw(ctx, sch, key)
	if err != nil || !found {
		t.Fatalf("checkpoint missing (found=%v err=%v)", found, err)
	}
	if raw != "null" {
		t.Fatalf("checkpoint = %q, want null (originally unset)", raw)
	}
}

// AC1 (wake): the exact prior value is restored — here suspend:false.
func TestCronJob_WakeRestoresPriorFalse(t *testing.T) {
	s, c := newSleeper(newCronJob("team-a", "report", boolPtr(false)))
	sch := schedule()
	ctx := context.Background()
	ref := Ref{Kind: KindCronJob, Namespace: "team-a", Name: "report"}

	_ = s.Apply(ctx, sch, true, []Ref{ref})
	if sus := cjSuspend(t, c, "report"); sus == nil || !*sus {
		t.Fatalf("asleep suspend = %v, want true", sus)
	}
	_ = s.Apply(ctx, sch, false, []Ref{ref})
	if sus := cjSuspend(t, c, "report"); sus == nil || *sus {
		t.Fatalf("restored suspend = %v, want false", sus)
	}
	key := checkpoint.Key("CronJob", "team-a", "report", "report-uid")
	if _, found, _ := s.Store.GetRaw(ctx, sch, key); found {
		t.Fatal("checkpoint should be cleared after restore")
	}
}

// wake restores an originally-unset CronJob back to unset (suspend nil).
func TestCronJob_WakeRestoresUnset(t *testing.T) {
	s, c := newSleeper(newCronJob("team-a", "report", nil))
	sch := schedule()
	ctx := context.Background()
	ref := Ref{Kind: KindCronJob, Namespace: "team-a", Name: "report"}

	_ = s.Apply(ctx, sch, true, []Ref{ref})
	_ = s.Apply(ctx, sch, false, []Ref{ref})
	if sus := cjSuspend(t, c, "report"); sus != nil {
		t.Fatalf("restored suspend = %v, want nil (unset)", sus)
	}
}

// sleep is idempotent: a second sleep does not re-checkpoint or re-event.
func TestCronJob_SleepIdempotent(t *testing.T) {
	s, c := newSleeper(newCronJob("team-a", "report", boolPtr(true)))
	sch := schedule()
	ctx := context.Background()
	ref := Ref{Kind: KindCronJob, Namespace: "team-a", Name: "report"}

	_ = s.Apply(ctx, sch, true, []Ref{ref}) // checkpoints prior (true)
	_ = s.Apply(ctx, sch, true, []Ref{ref}) // no-op
	key := checkpoint.Key("CronJob", "team-a", "report", "report-uid")
	raw, _, _ := s.Store.GetRaw(ctx, sch, key)
	if raw != "true" {
		t.Fatalf("checkpoint = %q, want true (not overwritten)", raw)
	}
	// restore brings back the original true.
	_ = s.Apply(ctx, sch, false, []Ref{ref})
	if sus := cjSuspend(t, c, "report"); sus == nil || !*sus {
		t.Fatalf("restored suspend = %v, want true", sus)
	}
}

// dry-run never mutates the CronJob or writes a checkpoint.
func TestCronJob_DryRun(t *testing.T) {
	s, c := newSleeper(newCronJob("team-a", "report", nil))
	sch := schedule()
	sch.Spec.DryRun = true
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{{Kind: KindCronJob, Namespace: "team-a", Name: "report"}}); err != nil {
		t.Fatal(err)
	}
	if sus := cjSuspend(t, c, "report"); sus != nil {
		t.Fatalf("dry-run set suspend = %v, want nil", sus)
	}
	key := checkpoint.Key("CronJob", "team-a", "report", "report-uid")
	if _, found, _ := s.Store.GetRaw(ctx, sch, key); found {
		t.Fatal("dry-run should not write a checkpoint")
	}
}
