/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import (
	"context"
	"encoding/json"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
)

// suspendObj is a loaded workload that sleeps by toggling spec.suspend
// (CronJob / Job), exposing what the shared logic needs.
type suspendObj struct {
	obj        client.Object
	uid        types.UID
	suspend    *bool
	setSuspend func(*bool)
}

func boolPtr(b bool) *bool { return &b }

// sleepSuspend checkpoints the prior spec.suspend once and sets it to true. It
// is a no-op when a checkpoint already exists (already slept by us).
func sleepSuspend(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref, w *suspendObj) error {
	log := logf.FromContext(ctx)
	key := checkpoint.Key(ref.Kind, ref.Namespace, ref.Name, w.uid)

	_, found, err := store.GetRaw(ctx, schedule, key)
	if err != nil {
		return err
	}
	if found {
		return nil // already slept by us — idempotent no-op
	}

	if schedule.Spec.DryRun {
		log.Info("dry-run: would checkpoint spec.suspend + suspend", "ref", ref)
		emit(rec, w.obj, "DryRunSlept", "dry-run: would set spec.suspend=true")
		return nil
	}

	raw, merr := json.Marshal(w.suspend)
	if merr != nil {
		return merr
	}
	if serr := store.SetRaw(ctx, schedule, key, string(raw)); serr != nil {
		return serr
	}

	patch := client.MergeFrom(w.obj.DeepCopyObject().(client.Object))
	w.setSuspend(boolPtr(true))
	if err := c.Patch(ctx, w.obj, patch); err != nil {
		return err
	}
	emit(rec, w.obj, "Slept", "set spec.suspend=true")
	return nil
}

// restoreSuspend restores the exact prior spec.suspend value (null → unset) and
// clears the checkpoint. No-op when there is no checkpoint.
func restoreSuspend(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref, w *suspendObj) error {
	key := checkpoint.Key(ref.Kind, ref.Namespace, ref.Name, w.uid)

	raw, found, err := store.GetRaw(ctx, schedule, key)
	if err != nil || !found {
		return err // never slept by us
	}
	var prior *bool
	if uerr := json.Unmarshal([]byte(raw), &prior); uerr != nil {
		return fmt.Errorf("corrupt suspend checkpoint %q: %w", key, uerr)
	}

	if schedule.Spec.DryRun {
		emit(rec, w.obj, "DryRunWoke", "dry-run: would restore spec.suspend")
		return nil // leave the checkpoint in place; nothing was mutated
	}

	patch := client.MergeFrom(w.obj.DeepCopyObject().(client.Object))
	w.setSuspend(prior)
	if err := c.Patch(ctx, w.obj, patch); err != nil {
		return err
	}
	emit(rec, w.obj, "Woke", "restored spec.suspend")
	return store.Delete(ctx, schedule, key)
}
