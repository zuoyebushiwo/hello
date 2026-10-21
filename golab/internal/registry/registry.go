// Package registry implements a source -> target converter registry. It is the
// Go equivalent of a small SPI/strategy map, with explicit registration instead
// of classpath scanning.
package registry

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Converter func(input string) string

type key struct{ source, target string }

type Registry struct {
	mu sync.RWMutex
	m  map[key]Converter
}

var ErrUnsupported = errors.New("unsupported conversion")

func New() *Registry { return &Registry{m: make(map[key]Converter)} }

func (r *Registry) Register(source, target string, c Converter) error {
	if source == "" || target == "" || c == nil {
		return errors.New("source, target and converter are required")
	}
	source, target = strings.ToLower(source), strings.ToLower(target)
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key{source, target}
	if _, exists := r.m[k]; exists {
		return fmt.Errorf("converter already registered for %s -> %s", source, target)
	}
	r.m[k] = c
	return nil
}

func (r *Registry) MustRegister(source, target string, c Converter) {
	if err := r.Register(source, target, c); err != nil {
		panic(err)
	}
}

func (r *Registry) Convert(source, target, input string) (string, error) {
	r.mu.RLock()
	c, ok := r.m[key{strings.ToLower(source), strings.ToLower(target)}]
	r.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("%w: %s -> %s", ErrUnsupported, source, target)
	}
	return c(input), nil
}

func (r *Registry) Pairs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.m))
	for k := range r.m {
		out = append(out, k.source+"->"+k.target)
	}
	sort.Strings(out)
	return out
}

func Default() *Registry {
	r := New()
	r.MustRegister("mysql", "tidb", func(in string) string {
		return strings.ToUpper(strings.TrimSpace(in))
	})
	r.MustRegister("mysql", "kudu", func(in string) string {
		u := strings.ToUpper(strings.TrimSpace(in))
		switch {
		case strings.HasPrefix(u, "VARCHAR"), strings.HasPrefix(u, "CHAR"), strings.HasPrefix(u, "TEXT"):
			return "STRING"
		case strings.HasPrefix(u, "BIGINT"):
			return "INT64"
		case strings.HasPrefix(u, "INT"):
			return "INT32"
		case strings.HasPrefix(u, "DATETIME"), strings.HasPrefix(u, "TIMESTAMP"):
			return "STRING"
		default:
			return u
		}
	})
	r.MustRegister("sqlserver", "tidb", func(in string) string {
		u := strings.ToUpper(strings.TrimSpace(in))
		if strings.HasPrefix(u, "NVARCHAR") {
			return "VARCHAR" + strings.TrimPrefix(u, "NVARCHAR")
		}
		if u == "BIT" {
			return "TINYINT(1)"
		}
		return u
	})
	return r
}
