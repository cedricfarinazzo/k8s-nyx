/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import (
	"context"
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
)

// sentinelKey is an unsatisfiable nodeSelector label: no node carries it, so a
// DaemonSet with this entry schedules zero pods. sleep injects it; restore
// removes it by writing back the exact checkpointed nodeSelector.
const (
	sentinelKey   = "nyx.dev/asleep"
	sentinelValue = "true"
)

// daemonSetHandler sleeps/restores DaemonSets by injecting/removing an
// unsatisfiable sentinel nodeSelector. DaemonSets have no replica field.
type daemonSetHandler struct{}

func (daemonSetHandler) Kind() string { return KindDaemonSet }

func (daemonSetHandler) List(ctx context.Context, c client.Client, opts ...client.ListOption) ([]Ref, error) {
	var list appsv1.DaemonSetList
	if err := c.List(ctx, &list, opts...); err != nil {
		return nil, err
	}
	refs := make([]Ref, 0, len(list.Items))
	for i := range list.Items {
		d := &list.Items[i]
		refs = append(refs, Ref{Kind: KindDaemonSet, Namespace: d.Namespace, Name: d.Name})
	}
	return refs, nil
}

func (daemonSetHandler) Sleep(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error {
	log := logf.FromContext(ctx)
	ds, err := getDaemonSet(ctx, c, ref)
	if err != nil || ds == nil {
		return err
	}
	if _, asleep := ds.Spec.Template.Spec.NodeSelector[sentinelKey]; asleep {
		return nil // already asleep — idempotent no-op
	}
	key := checkpoint.Key(ref.Kind, ref.Namespace, ref.Name, ds.UID)

	if schedule.Spec.DryRun {
		log.Info("dry-run: would checkpoint nodeSelector + sleep DaemonSet", "ref", ref)
		emit(rec, ds, "DryRunSlept", "dry-run: would inject sentinel nodeSelector")
		return nil
	}

	// Checkpoint the original nodeSelector exactly once (null when unset).
	if _, found, gerr := store.GetRaw(ctx, schedule, key); gerr != nil {
		return gerr
	} else if !found {
		raw, merr := json.Marshal(ds.Spec.Template.Spec.NodeSelector)
		if merr != nil {
			return merr
		}
		if serr := store.SetRaw(ctx, schedule, key, string(raw)); serr != nil {
			return serr
		}
	}

	patch := client.MergeFrom(ds.DeepCopy())
	sel := map[string]string{sentinelKey: sentinelValue}
	for k, v := range ds.Spec.Template.Spec.NodeSelector {
		sel[k] = v // keep original keys; the sentinel makes the set unsatisfiable
	}
	ds.Spec.Template.Spec.NodeSelector = sel
	if err := c.Patch(ctx, ds, patch); err != nil {
		return err
	}
	emit(rec, ds, "Slept", "injected sentinel nodeSelector (0 pods scheduled)")
	return nil
}

func (daemonSetHandler) Restore(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error {
	ds, err := getDaemonSet(ctx, c, ref)
	if err != nil || ds == nil {
		return err
	}
	key := checkpoint.Key(ref.Kind, ref.Namespace, ref.Name, ds.UID)

	raw, found, err := store.GetRaw(ctx, schedule, key)
	if err != nil || !found {
		return err // never slept by us
	}
	var original map[string]string
	if uerr := json.Unmarshal([]byte(raw), &original); uerr != nil {
		return fmt.Errorf("corrupt nodeSelector checkpoint %q: %w", key, uerr)
	}

	if schedule.Spec.DryRun {
		emit(rec, ds, "DryRunWoke", "dry-run: would restore original nodeSelector")
		return nil // leave the checkpoint in place; nothing was mutated
	}

	// Restore the exact checkpointed value (not the live one — AC3). A merge
	// patch deletes keys absent from the original (the sentinel) and sets
	// nodeSelector to null when it was originally unset.
	patch := client.MergeFrom(ds.DeepCopy())
	ds.Spec.Template.Spec.NodeSelector = original
	if err := c.Patch(ctx, ds, patch); err != nil {
		return err
	}
	emit(rec, ds, "Woke", "restored original nodeSelector")
	return store.Delete(ctx, schedule, key)
}

func getDaemonSet(ctx context.Context, c client.Client, ref Ref) (*appsv1.DaemonSet, error) {
	var ds appsv1.DaemonSet
	if err := c.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, &ds); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return &ds, nil
}
