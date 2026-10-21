// Package di is an explicit, typed dependency container. It replaces classpath
// scanning with code you can read: Provide registers, Get resolves lazily, and
// Close releases in reverse construction order.
package di

import (
	"errors"
	"fmt"
	"sync"
)

type Key[T any] struct{ name string }

func NewKey[T any](name string) Key[T] { return Key[T]{name: name} }

type def struct {
	name     string
	build    func(*Container) (any, error)
	closeFn  func(any) error
	instance any
	built    bool
	building bool
}

type Container struct {
	mu     sync.Mutex
	defs   map[string]*def
	order  []string
	closed bool
}

func New() *Container { return &Container{defs: make(map[string]*def)} }

func Provide[T any](c *Container, key Key[T], build func(*Container) (T, error), closeFn func(T) error) error {
	if build == nil {
		return errors.New("build function is required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("container is closed")
	}
	if _, exists := c.defs[key.name]; exists {
		return fmt.Errorf("duplicate key %q", key.name)
	}
	d := &def{name: key.name}
	d.build = func(ctn *Container) (any, error) { return build(ctn) }
	if closeFn != nil {
		d.closeFn = func(v any) error { return closeFn(v.(T)) }
	}
	c.defs[key.name] = d
	return nil
}

func Get[T any](c *Container, key Key[T]) (T, error) {
	var zero T
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return zero, errors.New("container is closed")
	}
	d, ok := c.defs[key.name]
	if !ok {
		c.mu.Unlock()
		return zero, fmt.Errorf("no provider for %q", key.name)
	}
	if d.built {
		v := d.instance
		c.mu.Unlock()
		return v.(T), nil
	}
	if d.building {
		c.mu.Unlock()
		return zero, fmt.Errorf("dependency cycle detected at %q", key.name)
	}
	d.building = true
	c.mu.Unlock()

	v, err := d.build(c)

	c.mu.Lock()
	d.building = false
	if err != nil {
		c.mu.Unlock()
		return zero, fmt.Errorf("build %s: %w", key.name, err)
	}
	d.instance = v
	d.built = true
	c.order = append(c.order, d.name)
	c.mu.Unlock()
	return v.(T), nil
}

func MustGet[T any](c *Container, key Key[T]) T {
	v, err := Get(c, key)
	if err != nil {
		panic(err)
	}
	return v
}

// Close releases constructed beans in reverse order. Beans that were never
// constructed are not closed. Multiple close errors are joined.
func (c *Container) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	order := append([]string(nil), c.order...)
	defs := make(map[string]*def, len(order))
	for _, name := range order {
		defs[name] = c.defs[name]
	}
	c.mu.Unlock()

	var errs []error
	for i := len(order) - 1; i >= 0; i-- {
		d := defs[order[i]]
		if d.closeFn == nil {
			continue
		}
		if err := d.closeFn(d.instance); err != nil {
			errs = append(errs, fmt.Errorf("close %s: %w", d.name, err))
		}
	}
	return errors.Join(errs...)
}

func (c *Container) BuiltOrder() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.order...)
}
