/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
)

// jobHandler sleeps/restores Jobs via spec.suspend. Suspending a Job with active
// pods deletes them, so a running Job is skipped (the data-safe default) and the
// skip is recorded as a Warning Event.
type jobHandler struct{}

func (jobHandler) Kind() string { return KindJob }

func (jobHandler) List(ctx context.Context, c client.Client, opts ...client.ListOption) ([]Ref, error) {
	var list batchv1.JobList
	if err := c.List(ctx, &list, opts...); err != nil {
		return nil, err
	}
	refs := make([]Ref, 0, len(list.Items))
	for i := range list.Items {
		j := &list.Items[i]
		refs = append(refs, Ref{Kind: KindJob, Namespace: j.Namespace, Name: j.Name})
	}
	return refs, nil
}

func (jobHandler) Sleep(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error {
	job, err := loadJob(ctx, c, ref)
	if err != nil || job == nil {
		return err
	}
	if job.Status.Active > 0 {
		// Skip: suspending would delete the active pods. Record the skip.
		warn(rec, job, "SkippedActiveJob",
			"not suspended: Job has active pods (would be deleted); skipped by default policy")
		return nil
	}
	return sleepSuspend(ctx, c, rec, store, schedule, ref, jobSuspendObj(job))
}

func (jobHandler) Restore(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error {
	job, err := loadJob(ctx, c, ref)
	if err != nil || job == nil {
		return err
	}
	return restoreSuspend(ctx, c, rec, store, schedule, ref, jobSuspendObj(job))
}

func loadJob(ctx context.Context, c client.Client, ref Ref) (*batchv1.Job, error) {
	var job batchv1.Job
	if err := c.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, &job); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return &job, nil
}

func jobSuspendObj(job *batchv1.Job) *suspendObj {
	return &suspendObj{
		obj:        job,
		uid:        job.UID,
		suspend:    job.Spec.Suspend,
		setSuspend: func(v *bool) { job.Spec.Suspend = v },
	}
}
