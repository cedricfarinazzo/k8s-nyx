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

// AnnotationAllowPVCDeletion opts a StatefulSet (or its SleepSchedule) into being
// slept even when its persistentVolumeClaimRetentionPolicy.whenScaled is Delete,
// which would destroy its PVCs on scale-down. Value must be "true".
const (
	AnnotationAllowPVCDeletion = "nyx.dev/allow-pvc-deletion"
	annotationTrue             = "true"
)

// statefulSetHandler sleeps/restores StatefulSets by scaling /spec/replicas.
// (PVCs created from volumeClaimTemplates are normally retained on scale-down, so
// state persists across the sleep; the operator never touches PVCs. The exception
// is whenScaled: Delete — see Sleep.)
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
	var sts appsv1.StatefulSet
	if err := c.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, &sts); err != nil {
		return client.IgnoreNotFound(err) // vanished; skip
	}
	// Refuse to sleep a StatefulSet that would lose its PVCs on scale-down, unless
	// explicitly opted in. Scaling to 0 with whenScaled: Delete deletes the PVCs.
	if pvcDeletionRisk(&sts) && !allowsPVCDeletion(&sts, schedule) {
		warn(rec, &sts, "PVCDeletionRisk",
			"not slept: persistentVolumeClaimRetentionPolicy.whenScaled is Delete; scaling to 0 would delete the PVCs (data loss). "+
				"Set annotation "+AnnotationAllowPVCDeletion+`="true" to allow.`)
		return nil
	}
	return sleepReplica(ctx, c, rec, store, schedule, ref, loadStatefulSet)
}

// pvcDeletionRisk reports whether scaling this StatefulSet down would delete its
// PVCs (persistentVolumeClaimRetentionPolicy.whenScaled == Delete).
func pvcDeletionRisk(sts *appsv1.StatefulSet) bool {
	p := sts.Spec.PersistentVolumeClaimRetentionPolicy
	return p != nil && p.WhenScaled == appsv1.DeletePersistentVolumeClaimRetentionPolicyType
}

// allowsPVCDeletion reports whether PVC deletion was opted into, via the
// annotation on the StatefulSet (explicit per-resource override) or, inherited,
// on the SleepSchedule.
func allowsPVCDeletion(sts *appsv1.StatefulSet, schedule *nyxv1alpha1.SleepSchedule) bool {
	return sts.Annotations[AnnotationAllowPVCDeletion] == annotationTrue ||
		schedule.Annotations[AnnotationAllowPVCDeletion] == annotationTrue
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
