/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

// Resolver resolves the workloads a SleepSchedule applies to, consulting the
// registry: only kinds with a registered handler are listed.
type Resolver struct {
	Client client.Client
	// Registry of handlers; defaults to Default() when nil.
	Registry *Registry
}

func (r *Resolver) registry() *Registry {
	if r.Registry != nil {
		return r.Registry
	}
	return Default()
}

// Resolve returns the workloads to act on (after targeting and exclusions) and
// the list of requested kinds that have no registered handler. It mutates
// nothing. Kinds without a handler contribute no workloads — the caller surfaces
// them (e.g. a Warning Event) and continues.
func (r *Resolver) Resolve(ctx context.Context, spec nyxv1alpha1.SleepScheduleSpec) (refs []Ref, unhandled []string, err error) {
	reg := r.registry()
	excluded := excludeSet(spec.ExcludeRefs)

	for _, kind := range requestedKinds(spec.Kinds, reg) {
		h, ok := reg.Get(kind)
		if !ok {
			unhandled = append(unhandled, kind)
			continue
		}
		found, lerr := listKind(ctx, r.Client, h, spec.Target)
		if lerr != nil {
			return nil, nil, lerr
		}
		for _, ref := range found {
			if isExcluded(excluded, ref) {
				continue
			}
			refs = append(refs, ref)
		}
	}

	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Kind != refs[j].Kind {
			return refs[i].Kind < refs[j].Kind
		}
		if refs[i].Namespace != refs[j].Namespace {
			return refs[i].Namespace < refs[j].Namespace
		}
		return refs[i].Name < refs[j].Name
	})
	return refs, unhandled, nil
}

// listKind lists one kind via its handler, subject to the target mode.
func listKind(ctx context.Context, c client.Client, h Handler, target nyxv1alpha1.Target) ([]Ref, error) {
	switch target.Mode {
	case nyxv1alpha1.TargetModeNamespaces:
		var refs []Ref
		for _, ns := range target.Namespaces {
			part, err := h.List(ctx, c, client.InNamespace(ns))
			if err != nil {
				return nil, err
			}
			refs = append(refs, part...)
		}
		return refs, nil
	case nyxv1alpha1.TargetModeLabels:
		sel, err := metav1.LabelSelectorAsSelector(target.Selector)
		if err != nil {
			return nil, fmt.Errorf("invalid target selector: %w", err)
		}
		return h.List(ctx, c, client.MatchingLabelsSelector{Selector: sel})
	default:
		return nil, fmt.Errorf("unknown target mode %q", target.Mode)
	}
}

// requestedKinds returns the kinds the schedule asks for, de-duplicated in a
// stable order. An empty request defaults to every registered kind.
func requestedKinds(requested []string, reg *Registry) []string {
	if len(requested) == 0 {
		return reg.Kinds()
	}
	seen := make(map[string]bool, len(requested))
	out := make([]string, 0, len(requested))
	for _, k := range requested {
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

func excludeSet(refs []nyxv1alpha1.ResourceRef) map[string]struct{} {
	set := make(map[string]struct{}, len(refs))
	for _, r := range refs {
		set[exKey(r.Kind, r.Namespace, r.Name)] = struct{}{}
	}
	return set
}

// isExcluded reports whether ref matches an excludeRef. A namespaced excludeRef
// matches only that namespace; a namespace-less one matches the kind+name in any
// namespace (wildcard).
func isExcluded(excluded map[string]struct{}, ref Ref) bool {
	if _, ok := excluded[exKey(ref.Kind, "", ref.Name)]; ok {
		return true
	}
	_, ok := excluded[exKey(ref.Kind, ref.Namespace, ref.Name)]
	return ok
}

func exKey(kind, namespace, name string) string { return kind + "/" + namespace + "/" + name }
