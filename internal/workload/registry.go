/*
Copyright 2026.

Licensed under the MIT License.
*/

package workload

// Registry maps a workload kind to its Handler. It is the single source of
// truth for which kinds the operator can act on: reconcile consults it before
// dispatching, so a kind without a handler is never mutated.
type Registry struct {
	byKind map[string]Handler
	order  []string // registration order, for stable Kinds()
}

// NewRegistry builds a Registry from the given handlers. A later handler for the
// same kind replaces an earlier one.
func NewRegistry(handlers ...Handler) *Registry {
	r := &Registry{byKind: map[string]Handler{}}
	for _, h := range handlers {
		if _, exists := r.byKind[h.Kind()]; !exists {
			r.order = append(r.order, h.Kind())
		}
		r.byKind[h.Kind()] = h
	}
	return r
}

// Default is the registry of kinds handled out of the box: Deployment and
// StatefulSet (replica-based). Additional kinds are registered by their own
// stories.
func Default() *Registry {
	return NewRegistry(deploymentHandler{}, statefulSetHandler{}, daemonSetHandler{})
}

// Get returns the handler for kind, and whether one is registered.
func (r *Registry) Get(kind string) (Handler, bool) {
	h, ok := r.byKind[kind]
	return h, ok
}

// Has reports whether a handler is registered for kind.
func (r *Registry) Has(kind string) bool {
	_, ok := r.byKind[kind]
	return ok
}

// Kinds returns the registered kinds in registration order.
func (r *Registry) Kinds() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}
