/*
Copyright 2026.

Licensed under the MIT License.
*/

// Package sleeper applies the sleep/wake decision to selected workloads, scaling
// only /spec/replicas and restoring the exact prior count from the checkpoint.
package sleeper

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
	"github.com/cedricfarinazzo/k8s-nyx/internal/target"
)

// Sleeper scales workloads to sleep and restores them on wake.
type Sleeper struct {
	Client client.Client
	Store  *checkpoint.Store
}

// workload abstracts the replica-bearing fields shared by Deployment / StatefulSet.
type workload struct {
	obj         client.Object
	uid         types.UID
	replicas    int32 // effective current replicas (nil treated as 1, the k8s default)
	setReplicas func(int32)
}

// Apply reconciles every target to the desired phase. asleep=true scales targets to
// sleepReplicas (recording the original once); asleep=false restores from checkpoint.
func (s *Sleeper) Apply(ctx context.Context, schedule *nyxv1alpha1.SleepSchedule, asleep bool, targets []target.WorkloadRef) error {
	for _, ref := range targets {
		if err := s.applyOne(ctx, schedule, asleep, ref); err != nil {
			return err
		}
	}
	return nil
}

func (s *Sleeper) applyOne(ctx context.Context, schedule *nyxv1alpha1.SleepSchedule, asleep bool, ref target.WorkloadRef) error {
	log := logf.FromContext(ctx)

	w, err := s.load(ctx, ref)
	if err != nil {
		return err
	}
	if w == nil {
		return nil // workload vanished between resolve and apply; skip
	}
	key := checkpoint.Key(ref.Kind, ref.Namespace, ref.Name, w.uid)

	if asleep {
		// Capture the true original exactly once — never overwrite while asleep.
		_, found, err := s.Store.Get(ctx, schedule, key)
		if err != nil {
			return err
		}
		if !found {
			if schedule.Spec.DryRun {
				log.Info("dry-run: would checkpoint + sleep", "ref", ref, "replicas", w.replicas)
				return nil
			}
			if err := s.Store.Set(ctx, schedule, key, w.replicas); err != nil {
				return err
			}
		}
		return s.patchReplicas(ctx, w, schedule.Spec.SleepReplicas, schedule.Spec.DryRun)
	}

	// Awake: restore from checkpoint if present, then clear it.
	orig, found, err := s.Store.Get(ctx, schedule, key)
	if err != nil {
		return err
	}
	if !found {
		return nil // never slept by us
	}
	if schedule.Spec.DryRun {
		log.Info("dry-run: would restore", "ref", ref, "replicas", orig)
		return nil
	}
	if err := s.patchReplicas(ctx, w, orig, false); err != nil {
		return err
	}
	return s.Store.Delete(ctx, schedule, key)
}

// patchReplicas sets spec.replicas to want via a merge patch (only /spec/replicas
// is in the patch, honouring the ArgoCD contract). No-op if already at want.
func (s *Sleeper) patchReplicas(ctx context.Context, w *workload, want int32, dryRun bool) error {
	if w.replicas == want {
		return nil
	}
	if dryRun {
		logf.FromContext(ctx).Info("dry-run: would scale", "to", want)
		return nil
	}
	patch := client.MergeFrom(w.obj.DeepCopyObject().(client.Object))
	w.setReplicas(want)
	return s.Client.Patch(ctx, w.obj, patch)
}

// load fetches the workload and exposes its replicas + UID. Returns nil if the
// object no longer exists.
func (s *Sleeper) load(ctx context.Context, ref target.WorkloadRef) (*workload, error) {
	nn := types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}
	switch ref.Kind {
	case target.KindDeployment:
		var d appsv1.Deployment
		if err := s.Client.Get(ctx, nn, &d); err != nil {
			return nil, client.IgnoreNotFound(err)
		}
		dep := &d
		return &workload{
			obj:         dep,
			uid:         dep.UID,
			replicas:    replicasOf(dep.Spec.Replicas),
			setReplicas: func(v int32) { dep.Spec.Replicas = &v },
		}, nil
	case target.KindStatefulSet:
		var sts appsv1.StatefulSet
		if err := s.Client.Get(ctx, nn, &sts); err != nil {
			return nil, client.IgnoreNotFound(err)
		}
		set := &sts
		return &workload{
			obj:         set,
			uid:         set.UID,
			replicas:    replicasOf(set.Spec.Replicas),
			setReplicas: func(v int32) { set.Spec.Replicas = &v },
		}, nil
	default:
		return nil, fmt.Errorf("unsupported kind %q", ref.Kind)
	}
}

func replicasOf(p *int32) int32 {
	if p == nil {
		return 1 // Kubernetes default when spec.replicas is unset
	}
	return *p
}
