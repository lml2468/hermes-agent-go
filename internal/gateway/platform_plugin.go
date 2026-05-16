package gateway

import (
	"fmt"
	"sync"
)

// PlatformPluginMeta contains metadata about a platform plugin.
type PlatformPluginMeta struct {
	ID          Platform
	Name        string
	Version     string
	Description string
}

// PlatformPlugin is the interface that platform plugins must implement.
type PlatformPlugin interface {
	Metadata() PlatformPluginMeta
	CreateAdapter(cfg *PlatformConfig) (PlatformAdapter, error)
}

// PlatformPluginRegistry holds registered platform plugins.
type PlatformPluginRegistry struct {
	mu      sync.RWMutex
	plugins map[Platform]PlatformPlugin
}

var globalPlatformRegistry = &PlatformPluginRegistry{
	plugins: make(map[Platform]PlatformPlugin),
}

// GlobalPlatformRegistry returns the global platform plugin registry.
func GlobalPlatformRegistry() *PlatformPluginRegistry {
	return globalPlatformRegistry
}

// Register adds a platform plugin to the registry.
func (r *PlatformPluginRegistry) Register(p PlatformPlugin) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	meta := p.Metadata()
	if _, exists := r.plugins[meta.ID]; exists {
		return fmt.Errorf("platform plugin %q already registered", meta.ID)
	}
	r.plugins[meta.ID] = p
	return nil
}

// Get retrieves a platform plugin by its platform identifier.
func (r *PlatformPluginRegistry) Get(id Platform) (PlatformPlugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[id]
	return p, ok
}

// All returns metadata for all registered platform plugins.
func (r *PlatformPluginRegistry) All() []PlatformPluginMeta {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var metas []PlatformPluginMeta
	for _, p := range r.plugins {
		metas = append(metas, p.Metadata())
	}
	return metas
}
