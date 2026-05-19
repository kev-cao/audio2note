// Package registry provides a small generic name→adapter map used by the
// transcriber and summarizer packages.
package registry

import (
	"fmt"
	"sort"
)

type Named interface {
	Name() string
}

type Registry[T Named] struct {
	items map[string]T
}

func New[T Named]() *Registry[T] {
	return &Registry[T]{items: make(map[string]T)}
}

func (r *Registry[T]) Register(item T) {
	name := item.Name()
	if _, exists := r.items[name]; exists {
		panic(fmt.Sprintf("registry: duplicate registration for %q", name))
	}
	r.items[name] = item
}

func (r *Registry[T]) Get(name string) (T, error) {
	item, ok := r.items[name]
	if !ok {
		var zero T
		return zero, fmt.Errorf("unknown name %q (available: %v)", name, r.Names())
	}
	return item, nil
}

func (r *Registry[T]) Names() []string {
	names := make([]string, 0, len(r.items))
	for name := range r.items {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
