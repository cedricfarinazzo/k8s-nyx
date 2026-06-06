/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import (
	"context"
	"fmt"

	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
	"github.com/cedricfarinazzo/k8s-nyx/internal/checkpoint"
)

// Sleeper applies the sleep/wake decision to resolved workloads by dispatching
// each ref to its kind's registered handler.
type Sleeper struct {
	Client client.Client
	Store  *checkpoint.Store
	// Recorder emits Events on affected workloads; may be nil (events skipped).
	Recorder record.EventRecorder
	// Registry of handlers; defaults to Default() when nil.
	Registry *Registry
}

func (s *Sleeper) registry() *Registry {
	if s.Registry != nil {
		return s.Registry
	}
	return Default()
}

// Apply reconciles every ref to the desired phase. asleep=true puts targets to
// sleep (recording the original once); asleep=false restores them. Each ref is
// dispatched to its kind's handler; a ref whose kind has no handler is a
// programming error (Resolve never returns one) and surfaces as an error.
func (s *Sleeper) Apply(ctx context.Context, schedule *nyxv1alpha1.SleepSchedule, asleep bool, refs []Ref) error {
	reg := s.registry()
	for _, ref := range refs {
		h, ok := reg.Get(ref.Kind)
		if !ok {
			return fmt.Errorf("unsupported kind %q", ref.Kind)
		}
		var err error
		if asleep {
			err = h.Sleep(ctx, s.Client, s.Recorder, s.Store, schedule, ref)
		} else {
			err = h.Restore(ctx, s.Client, s.Recorder, s.Store, schedule, ref)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
