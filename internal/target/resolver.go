/*
Copyright 2026.

Licensed under the MIT License.
*/

// Package target resolves the concrete workloads a SleepSchedule applies to,
// honouring the targeting mode (namespaces vs labels) and excludeRefs.
package target

import (
	"context"
	"fmt"
	"sort"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nyxv1alpha1 "github.com/cedricfarinazzo/k8s-nyx/api/v1alpha1"
)

// Supported workload kinds. Other kinds are deferred to E5 and ignored for now.
const (
	KindDeployment  = "Deployment"
	KindStatefulSet = "StatefulSet"
)

// defaultKinds is used when spec.kinds is empty.
var defaultKinds = []string{KindDeployment, KindStatefulSet}

// WorkloadRef identifies a single selected workload.
type WorkloadRef struct {
	Kind      string
	Namespace string
	Name      string
}

// Resolver resolves a SleepSchedule's targets via the controller-runtime client.
type Resolver struct {
	Client client.Client
}

// Resolve returns the workloads the schedule applies to, after targeting and
// exclusions. It does not mutate anything.
func (r *Resolver) Resolve(ctx context.Context, spec nyxv1alpha1.SleepScheduleSpec) ([]WorkloadRef, error) {
	kinds := supportedKinds(spec.Kinds)
	excluded := excludeSet(spec.ExcludeRefs)

	var refs []WorkloadRef
	for _, kind := range kinds {
		found, err := r.listKind(ctx, kind, spec.Target)
		if err != nil {
			return nil, err
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
	return refs, nil
}

// listKind lists workloads of one kind subject to the target mode.
func (r *Resolver) listKind(ctx context.Context, kind string, target nyxv1alpha1.Target) ([]WorkloadRef, error) {
	switch target.Mode {
	case nyxv1alpha1.TargetModeNamespaces:
		var refs []WorkloadRef
		for _, ns := range target.Namespaces {
			part, err := r.listKindFiltered(ctx, kind, client.InNamespace(ns))
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
		return r.listKindFiltered(ctx, kind, client.MatchingLabelsSelector{Selector: sel})
	default:
		return nil, fmt.Errorf("unknown target mode %q", target.Mode)
	}
}

// listKindFiltered lists one workload kind with the given list options and maps
// the items to WorkloadRefs.
func (r *Resolver) listKindFiltered(ctx context.Context, kind string, opts ...client.ListOption) ([]WorkloadRef, error) {
	switch kind {
	case KindDeployment:
		var list appsv1.DeploymentList
		if err := r.Client.List(ctx, &list, opts...); err != nil {
			return nil, err
		}
		return mapItems(kind, list.Items, func(d appsv1.Deployment) (string, string) {
			return d.Namespace, d.Name
		}), nil
	case KindStatefulSet:
		var list appsv1.StatefulSetList
		if err := r.Client.List(ctx, &list, opts...); err != nil {
			return nil, err
		}
		return mapItems(kind, list.Items, func(s appsv1.StatefulSet) (string, string) {
			return s.Namespace, s.Name
		}), nil
	default:
		// Unsupported kind: ignored (deferred to E5).
		return nil, nil
	}
}

func mapItems[T any](kind string, items []T, nsName func(T) (string, string)) []WorkloadRef {
	refs := make([]WorkloadRef, 0, len(items))
	for _, it := range items {
		ns, name := nsName(it)
		refs = append(refs, WorkloadRef{Kind: kind, Namespace: ns, Name: name})
	}
	return refs
}

// supportedKinds intersects the requested kinds with the supported set, preserving
// the supported-set order. An empty request defaults to all supported kinds.
func supportedKinds(requested []string) []string {
	if len(requested) == 0 {
		return defaultKinds
	}
	want := make(map[string]bool, len(requested))
	for _, k := range requested {
		want[k] = true
	}
	var out []string
	for _, k := range defaultKinds {
		if want[k] {
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
func isExcluded(excluded map[string]struct{}, ref WorkloadRef) bool {
	if _, ok := excluded[exKey(ref.Kind, "", ref.Name)]; ok {
		return true
	}
	_, ok := excluded[exKey(ref.Kind, ref.Namespace, ref.Name)]
	return ok
}

func exKey(kind, namespace, name string) string { return kind + "/" + namespace + "/" + name }
