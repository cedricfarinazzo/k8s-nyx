/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import (
	"context"
	"encoding/json"
	"fmt"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
)

// hpaBounds is the checkpointed HPA scaling range.
type hpaBounds struct {
	Min *int32 `json:"min,omitempty"`
	Max int32  `json:"max"`
}

// hpaHandler sleeps/restores HorizontalPodAutoscalers by neutralizing their
// min/max replica bounds so the HPA cannot scale a slept workload back up, then
// restoring the exact original bounds on wake.
type hpaHandler struct{}

func (hpaHandler) Kind() string { return KindHPA }

func (hpaHandler) List(ctx context.Context, c client.Client, opts ...client.ListOption) ([]Ref, error) {
	var list autoscalingv2.HorizontalPodAutoscalerList
	if err := c.List(ctx, &list, opts...); err != nil {
		return nil, err
	}
	refs := make([]Ref, 0, len(list.Items))
	for i := range list.Items {
		h := &list.Items[i]
		refs = append(refs, Ref{Kind: KindHPA, Namespace: h.Namespace, Name: h.Name})
	}
	return refs, nil
}

func (hpaHandler) Sleep(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error {
	log := logf.FromContext(ctx)
	hpa, err := loadHPA(ctx, c, ref)
	if err != nil || hpa == nil {
		return err
	}
	key := checkpoint.Key(ref.Kind, ref.Namespace, ref.Name, hpa.UID)

	if _, found, gerr := store.GetRaw(ctx, schedule, key); gerr != nil {
		return gerr
	} else if found {
		return nil // already slept by us — idempotent no-op
	}

	// Neutralize: pin min=max to the sleep floor so the HPA holds the workload
	// there. maxReplicas must be >= 1; minReplicas needs the HPAScaleToZero
	// feature gate to go below 1, which the operator can't assume — so a floor of
	// 0 is clamped to 1 with a Warning (AC3).
	floor := schedule.Spec.SleepReplicas
	neutralized := floor
	if neutralized < 1 {
		neutralized = 1
		if floor == 0 {
			warn(rec, hpa, "HPAScaleToZeroUnavailable",
				"minReplicas clamped to 1: scaling an HPA to 0 needs the HPAScaleToZero feature gate")
		}
	}

	if schedule.Spec.DryRun {
		log.Info("dry-run: would checkpoint + neutralize HPA min/max", "ref", ref, "to", neutralized)
		emit(rec, hpa, "DryRunSlept", fmt.Sprintf("dry-run: would pin HPA min/max to %d", neutralized))
		return nil
	}

	bounds := hpaBounds{Min: hpa.Spec.MinReplicas, Max: hpa.Spec.MaxReplicas}
	raw, merr := json.Marshal(bounds)
	if merr != nil {
		return merr
	}
	if serr := store.SetRaw(ctx, schedule, key, string(raw)); serr != nil {
		return serr
	}

	patch := client.MergeFrom(hpa.DeepCopy())
	hpa.Spec.MinReplicas = boundPtr(neutralized)
	hpa.Spec.MaxReplicas = neutralized
	if err := c.Patch(ctx, hpa, patch); err != nil {
		return err
	}
	emit(rec, hpa, "Slept", fmt.Sprintf("neutralized HPA min/max to %d", neutralized))
	return nil
}

func (hpaHandler) Restore(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error {
	hpa, err := loadHPA(ctx, c, ref)
	if err != nil || hpa == nil {
		return err
	}
	key := checkpoint.Key(ref.Kind, ref.Namespace, ref.Name, hpa.UID)

	raw, found, err := store.GetRaw(ctx, schedule, key)
	if err != nil || !found {
		return err // never slept by us
	}
	var bounds hpaBounds
	if uerr := json.Unmarshal([]byte(raw), &bounds); uerr != nil {
		return fmt.Errorf("corrupt HPA checkpoint %q: %w", key, uerr)
	}

	if schedule.Spec.DryRun {
		emit(rec, hpa, "DryRunWoke", "dry-run: would restore HPA min/max")
		return nil // leave the checkpoint in place; nothing was mutated
	}

	// Restore the exact checkpointed bounds (not the live ones).
	patch := client.MergeFrom(hpa.DeepCopy())
	hpa.Spec.MinReplicas = bounds.Min
	hpa.Spec.MaxReplicas = bounds.Max
	if err := c.Patch(ctx, hpa, patch); err != nil {
		return err
	}
	emit(rec, hpa, "Woke", "restored HPA min/max")
	return store.Delete(ctx, schedule, key)
}

func loadHPA(ctx context.Context, c client.Client, ref Ref) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	var hpa autoscalingv2.HorizontalPodAutoscaler
	if err := c.Get(ctx, types.NamespacedName{Namespace: ref.Namespace, Name: ref.Name}, &hpa); err != nil {
		return nil, client.IgnoreNotFound(err)
	}
	return &hpa, nil
}

func boundPtr(v int32) *int32 { return &v }
