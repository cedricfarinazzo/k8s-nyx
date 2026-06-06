/*
Copyright 2026.

Licensed under the MIT License.
*/

// Package audit emits a uniform audit trail for every lifecycle action the
// operator takes: a structured (JSON) log line carrying who / what / why / when
// plus the affected object ref, and a corresponding Kubernetes Event on the
// SleepSchedule that drove the action.
package audit

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// DefaultActor is the "who" for actions the operator takes on its own (a
// schedule transition), as opposed to a human-driven wake override.
const DefaultActor = "k8s-nyx"

// Info is the who/why attribution for the current reconcile pass. It is carried
// on the context so the per-object handlers can record it without threading it
// through every signature.
type Info struct {
	Who string // actor: the operator, or the wake "by" for an override
	Why string // reason: e.g. "asleep window", "active wake override"
}

type ctxKey struct{}

// NewContext returns ctx carrying the audit attribution.
func NewContext(ctx context.Context, info Info) context.Context {
	return context.WithValue(ctx, ctxKey{}, info)
}

// FromContext returns the audit attribution on ctx, defaulting Who to the
// operator when unset.
func FromContext(ctx context.Context) Info {
	info, _ := ctx.Value(ctxKey{}).(Info)
	if info.Who == "" {
		info.Who = DefaultActor
	}
	return info
}

// Record emits the audit trail for one action: a structured log line (AC1/AC3)
// and a Normal Event on the SleepSchedule (AC2). refKind/refNamespace/refName
// identify the affected workload for correlation; pass empty strings for an
// action that is about the schedule itself.
func Record(ctx context.Context, rec record.EventRecorder, schedule client.Object, refKind, refNamespace, refName, action, message string) {
	info := FromContext(ctx)
	objectRef := ""
	if refKind != "" {
		objectRef = fmt.Sprintf("%s/%s/%s", refKind, refNamespace, refName)
	}
	logf.FromContext(ctx).Info("audit",
		"action", action,
		"who", info.Who,
		"why", info.Why,
		"when", time.Now().UTC().Format(time.RFC3339),
		"objectRef", objectRef,
		"sleepSchedule", schedule.GetNamespace()+"/"+schedule.GetName(),
	)
	if rec != nil {
		msg := message
		if objectRef != "" {
			msg = fmt.Sprintf("%s [%s]", message, objectRef)
		}
		rec.Event(schedule, corev1.EventTypeNormal, action, msg)
	}
}
