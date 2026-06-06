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

// cronJobHandler sleeps/restores CronJobs via spec.suspend.
type cronJobHandler struct{}

func (cronJobHandler) Kind() string { return KindCronJob }

func (cronJobHandler) List(ctx context.Context, c client.Client, opts ...client.ListOption) ([]Ref, error) {
	var list batchv1.CronJobList
	if err := c.List(ctx, &list, opts...); err != nil {
		return nil, err
	}
	refs := make([]Ref, 0, len(list.Items))
	for i := range list.Items {
		cj := &list.Items[i]
		refs = append(refs, Ref{Kind: KindCronJob, Namespace: cj.Namespace, Name: cj.Name})
	}
	return refs, nil
}

func (cronJobHandler) Sleep(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error {
	cj, err := loadCronJob(ctx, c, ref)
	if err != nil || cj == nil {
		return err
	}
	return sleepSuspend(ctx, c, rec, store, schedule, ref, cronJobSuspendObj(cj))
}

func (cronJobHandler) Restore(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error {
	cj, err := loadCronJob(ctx, c, ref)
	if err != nil || cj == nil {
		return err
	}
	return restoreSuspend(ctx, c, rec, store, schedule, ref, cronJobSuspendObj(cj))
}

func loadCronJob(ctx context.Context, c client.Client, ref Ref) (*batchv1.CronJob, error) {
	var cj batchv1.CronJob
	if err := c.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, &cj); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return &cj, nil
}

func cronJobSuspendObj(cj *batchv1.CronJob) *suspendObj {
	return &suspendObj{
		obj:        cj,
		uid:        cj.UID,
		suspend:    cj.Spec.Suspend,
		setSuspend: func(v *bool) { cj.Spec.Suspend = v },
	}
}
