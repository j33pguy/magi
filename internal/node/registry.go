package node

import "sync"

// Capability describes what a registered node can do.
type Capability struct {
	Type     NodeType
	PoolSize int
	Mode     Mode
}

// Registry tracks available node capabilities in the mesh.
type Registry struct {
	mu           sync.RWMutex
	capabilities map[NodeType]*Capability
}

// NewRegistry creates an empty node registry.
func NewRegistry() *Registry {
	return &Registry{
		capabilities: make(map[NodeType]*Capability),
	}
}

// Register advertises a node capability.
func (r *Registry) Register(cap *Capability) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities[cap.Type] = cap
}

// Unregister removes a node capability.
func (r *Registry) Unregister(nodeType NodeType) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.capabilities, nodeType)
}

// Get returns the capability for a node type, or nil if not registered.
func (r *Registry) Get(nodeType NodeType) *Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.capabilities[nodeType]
}

// List returns all registered capabilities.
func (r *Registry) List() []*Capability {
	r.mu.RLock()
	defer r.mu.RUnlock()
	caps := make([]*Capability, 0, len(r.capabilities))
	for _, cap := range r.capabilities {
		caps = append(caps, cap)
	}
	return caps
}

// Has returns true if the given node type is registered.
func (r *Registry) Has(nodeType NodeType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.capabilities[nodeType]
	return ok
}
