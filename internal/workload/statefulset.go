/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
)

// statefulSetHandler sleeps/restores StatefulSets by scaling /spec/replicas.
// (PVCs created from volumeClaimTemplates are retained on scale-down, so state
// persists across the sleep; the operator never touches PVCs.)
type statefulSetHandler struct{}

func (statefulSetHandler) Kind() string { return KindStatefulSet }

func (statefulSetHandler) List(ctx context.Context, c client.Client, opts ...client.ListOption) ([]Ref, error) {
	var list appsv1.StatefulSetList
	if err := c.List(ctx, &list, opts...); err != nil {
		return nil, err
	}
	refs := make([]Ref, 0, len(list.Items))
	for i := range list.Items {
		s := &list.Items[i]
		refs = append(refs, Ref{Kind: KindStatefulSet, Namespace: s.Namespace, Name: s.Name})
	}
	return refs, nil
}

func (statefulSetHandler) Sleep(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error {
	return sleepReplica(ctx, c, rec, store, schedule, ref, loadStatefulSet)
}

func (statefulSetHandler) Restore(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error {
	return restoreReplica(ctx, c, rec, store, schedule, ref, loadStatefulSet)
}

func loadStatefulSet(ctx context.Context, c client.Client, ref Ref) (*replicaObj, error) {
	var sts appsv1.StatefulSet
	if err := c.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, &sts); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	set := &sts
	return &replicaObj{
		obj:         set,
		uid:         set.UID,
		replicas:    replicasOf(set.Spec.Replicas),
		setReplicas: func(v int32) { set.Spec.Replicas = &v },
	}, nil
}
