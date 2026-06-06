/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import (
	"context"
	"strings"
	"testing"

	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
)

func newJob(ns, name string, suspend *bool, active int32) *batchv1.Job {
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: name, UID: types.UID(name + "-uid")},
		Spec:       batchv1.JobSpec{Suspend: suspend},
		Status:     batchv1.JobStatus{Active: active},
	}
}

func jobSuspend(t *testing.T, c client.Client, name string) *bool {
	t.Helper()
	var j batchv1.Job
	if err := c.Get(context.Background(), types.NamespacedName{Namespace: "team-a", Name: name}, &j); err != nil {
		t.Fatalf("get job: %v", err)
	}
	return j.Spec.Suspend
}

// AC2: a Job with no active pods is suspended.
func TestJob_SleepSuspendsWhenIdle(t *testing.T) {
	s, c := newSleeper(newJob("team-a", "batch", nil, 0))
	sch := schedule()
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{{Kind: KindJob, Namespace: "team-a", Name: "batch"}}); err != nil {
		t.Fatal(err)
	}
	if sus := jobSuspend(t, c, "batch"); sus == nil || !*sus {
		t.Fatalf("suspend = %v, want true", sus)
	}
}

// AC3: a Job with active pods is skipped (not suspended), no checkpoint is
// written, and a Warning Event records the skip.
func TestJob_SkipsActiveJob(t *testing.T) {
	s, c := newSleeper(newJob("team-a", "batch", nil, 2))
	rec := record.NewFakeRecorder(10)
	s.Recorder = rec
	sch := schedule()
	ctx := context.Background()

	if err := s.Apply(ctx, sch, true, []Ref{{Kind: KindJob, Namespace: "team-a", Name: "batch"}}); err != nil {
		t.Fatal(err)
	}
	if sus := jobSuspend(t, c, "batch"); sus != nil {
		t.Fatalf("active Job was suspended (%v); should be skipped", sus)
	}
	key := checkpoint.Key("Job", "team-a", "batch", "batch-uid")
	if _, found, _ := s.Store.GetRaw(ctx, sch, key); found {
		t.Fatal("skipped Job should not be checkpointed")
	}
	ev := drain(rec)
	if len(ev) != 1 || !strings.Contains(ev[0], "Warning") || !strings.Contains(ev[0], "SkippedActiveJob") {
		t.Fatalf("events = %v, want one SkippedActiveJob Warning", ev)
	}
}

// an idle Job restores its prior suspend value on wake.
func TestJob_WakeRestores(t *testing.T) {
	s, c := newSleeper(newJob("team-a", "batch", boolPtr(false), 0))
	sch := schedule()
	ctx := context.Background()
	ref := Ref{Kind: KindJob, Namespace: "team-a", Name: "batch"}

	_ = s.Apply(ctx, sch, true, []Ref{ref})
	if sus := jobSuspend(t, c, "batch"); sus == nil || !*sus {
		t.Fatalf("asleep suspend = %v, want true", sus)
	}
	_ = s.Apply(ctx, sch, false, []Ref{ref})
	if sus := jobSuspend(t, c, "batch"); sus == nil || *sus {
		t.Fatalf("restored suspend = %v, want false", sus)
	}
}
