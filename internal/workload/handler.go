/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import (
	"context"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
)

// Handler knows how to list, sleep, and restore one workload kind. Every
// supported kind is one Handler registered in the Registry; the reconciler only
// ever touches a kind that has a Handler.
type Handler interface {
	// Kind is the workload kind this handler manages, e.g. "Deployment".
	Kind() string

	// List returns the refs of this kind matching the given list options
	// (namespace and/or label selector).
	List(ctx context.Context, c client.Client, opts ...client.ListOption) ([]Ref, error)

	// Sleep puts the referenced workload to sleep, checkpointing whatever it
	// needs to restore later (once — never overwritten while asleep). It is a
	// no-op when the workload is already asleep.
	Sleep(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error

	// Restore returns the referenced workload to its checkpointed state and
	// clears the checkpoint entry. It is a no-op when there is no checkpoint.
	Restore(ctx context.Context, c client.Client, rec record.EventRecorder, store *checkpoint.Store, schedule *nyxv1alpha1.SleepSchedule, ref Ref) error
}
