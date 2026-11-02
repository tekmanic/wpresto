package plugin

import (
	"sort"
	"strings"
	"sync"
)

var (
	mu      sync.RWMutex
	plugins = map[string]Plugin{}
)

// Register adds a plugin to the global registry. It is called from a plugin
// package's init() function. Registering a duplicate name panics (a bug).
func Register(p Plugin) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := plugins[p.Name()]; exists {
		panic("plugin: duplicate name " + p.Name())
	}
	plugins[p.Name()] = p
}

// All returns every registered plugin, sorted by name.
func All() []Plugin {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]Plugin, 0, len(plugins))
	for _, p := range plugins {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

// Get returns the plugin with the given name.
func Get(name string) (Plugin, bool) {
	mu.RLock()
	defer mu.RUnlock()
	p, ok := plugins[name]
	return p, ok
}

// FindKind returns the first plugin whose Kinds() includes class.
func FindKind(class string) (Plugin, bool) {
	mu.RLock()
	defer mu.RUnlock()
	for _, p := range plugins {
		for _, k := range p.Kinds() {
			if strings.EqualFold(k, class) {
				return p, true
			}
		}
	}
	return nil, false
}

// Find returns the plugin whose route matches path and method, if any.
// Used by the plugin server to route incoming requests.
func Find(path, method string) (Plugin, bool) {
	mu.RLock()
	defer mu.RUnlock()
	for _, p := range plugins {
		for _, r := range p.Routes() {
			if routeMatches(r, path, method) {
				return p, true
			}
		}
	}
	return nil, false
}

// routeMatches reports whether a route matches a path and method.
func routeMatches(r Route, path, method string) bool {
	if !pathMatches(r.Pattern, path) {
		return false
	}
	if len(r.Methods) == 0 {
		return true
	}
	for _, m := range r.Methods {
		if strings.EqualFold(m, method) {
			return true
		}
	}
	return false
}

// pathMatches reports whether a route pattern matches a request path.
// A trailing "*" makes the pattern a prefix match; otherwise it is exact.
func pathMatches(pattern, path string) bool {
	if pattern == path {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(path, strings.TrimSuffix(pattern, "*"))
	}
	return false
}
