/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
)

// replicaObj is a loaded replica-bearing workload (Deployment / StatefulSet),
// exposing the bits the shared sleep/restore logic needs.
type replicaObj struct {
	obj         client.Object
	uid         types.UID
	replicas    int32 // effective current replicas (nil treated as 1, the k8s default)
	setReplicas func(int32)
}

// loadFunc fetches a replica workload by ref, or returns (nil, nil) if it no
// longer exists.
type loadFunc func(ctx context.Context, c client.Client, ref Ref) (*replicaObj, error)

// sleepReplica scales the workload to sleepReplicas, recording its original
// replica count in the checkpoint exactly once (never overwritten while asleep).
func sleepReplica(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref, load loadFunc) error {
	log := logf.FromContext(ctx)
	w, err := load(ctx, c, ref)
	if err != nil {
		return err
	}
	if w == nil {
		return nil // vanished between resolve and apply; skip
	}
	key := checkpoint.Key(ref.Kind, ref.Namespace, ref.Name, w.uid)

	_, found, err := store.Get(ctx, schedule, key)
	if err != nil {
		return err
	}
	if !found {
		if schedule.Spec.DryRun {
			log.Info("dry-run: would checkpoint + sleep", "ref", ref, "replicas", w.replicas)
		} else if err := store.Set(ctx, schedule, key, w.replicas); err != nil {
			return err
		}
	}
	return patchReplicas(ctx, c, rec, w, schedule.Spec.SleepReplicas, schedule.Spec.DryRun, "Slept")
}

// restoreReplica restores the exact checkpointed replica count and clears the
// checkpoint entry. It is a no-op when there is no checkpoint (never slept by us).
func restoreReplica(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref, load loadFunc) error {
	w, err := load(ctx, c, ref)
	if err != nil {
		return err
	}
	if w == nil {
		return nil
	}
	key := checkpoint.Key(ref.Kind, ref.Namespace, ref.Name, w.uid)

	orig, found, err := store.Get(ctx, schedule, key)
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if err := patchReplicas(ctx, c, rec, w, orig, schedule.Spec.DryRun, "Woke"); err != nil {
		return err
	}
	if schedule.Spec.DryRun {
		return nil // leave the checkpoint in place; nothing was mutated
	}
	return store.Delete(ctx, schedule, key)
}

// patchReplicas sets spec.replicas to want via a merge patch (only /spec/replicas
// is in the patch, honouring the ArgoCD contract). No-op when already at want.
func patchReplicas(ctx context.Context, c client.Client, rec record.EventRecorder, w *replicaObj, want int32, dryRun bool, reason string) error {
	if w.replicas == want {
		return nil
	}
	if dryRun {
		logf.FromContext(ctx).Info("dry-run: would scale", "to", want)
		emit(rec, w.obj, "DryRun"+reason, fmt.Sprintf("dry-run: would scale to %d replicas", want))
		return nil
	}
	patch := client.MergeFrom(w.obj.DeepCopyObject().(client.Object))
	w.setReplicas(want)
	if err := c.Patch(ctx, w.obj, patch); err != nil {
		return err
	}
	emit(rec, w.obj, reason, fmt.Sprintf("scaled to %d replicas", want))
	return nil
}

// emit records a Normal Event on obj if a recorder is configured.
func emit(rec record.EventRecorder, obj client.Object, reason, message string) {
	if rec != nil {
		rec.Event(obj, corev1.EventTypeNormal, reason, message)
	}
}

func replicasOf(p *int32) int32 {
	if p == nil {
		return 1 // Kubernetes default when spec.replicas is unset
	}
	return *p
}
