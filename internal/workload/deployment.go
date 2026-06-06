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

// deploymentHandler sleeps/restores Deployments by scaling /spec/replicas.
type deploymentHandler struct{}

func (deploymentHandler) Kind() string { return KindDeployment }

func (deploymentHandler) List(ctx context.Context, c client.Client, opts ...client.ListOption) ([]Ref, error) {
	var list appsv1.DeploymentList
	if err := c.List(ctx, &list, opts...); err != nil {
		return nil, err
	}
	refs := make([]Ref, 0, len(list.Items))
	for i := range list.Items {
		d := &list.Items[i]
		refs = append(refs, Ref{Kind: KindDeployment, Namespace: d.Namespace, Name: d.Name})
	}
	return refs, nil
}

func (deploymentHandler) Sleep(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error {
	return sleepReplica(ctx, c, rec, store, schedule, ref, loadDeployment)
}

func (deploymentHandler) Restore(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error {
	return restoreReplica(ctx, c, rec, store, schedule, ref, loadDeployment)
}

func loadDeployment(ctx context.Context, c client.Client, ref Ref) (*replicaObj, error) {
	var d appsv1.Deployment
	if err := c.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, &d); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	dep := &d
	return &replicaObj{
		obj:         dep,
		uid:         dep.UID,
		replicas:    replicasOf(dep.Spec.Replicas),
		setReplicas: func(v int32) { dep.Spec.Replicas = &v },
	}, nil
}
