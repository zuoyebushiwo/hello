// Package graph implements the DAG primitives a scheduler needs: validation,
// cycle detection, topological order and parallel layers.
package graph

import (
	"fmt"
	"sort"
)

type Graph struct {
	nodes map[string]struct{}
	outs  map[string]map[string]struct{}
	ins   map[string]map[string]struct{}
}

func New() *Graph {
	return &Graph{
		nodes: make(map[string]struct{}),
		outs:  make(map[string]map[string]struct{}),
		ins:   make(map[string]map[string]struct{}),
	}
}

func (g *Graph) AddNode(id string) {
	g.nodes[id] = struct{}{}
	if g.outs[id] == nil {
		g.outs[id] = make(map[string]struct{})
	}
	if g.ins[id] == nil {
		g.ins[id] = make(map[string]struct{})
	}
}

func (g *Graph) AddEdge(from, to string) error {
	if from == to {
		return fmt.Errorf("self loop at %q", from)
	}
	if _, ok := g.nodes[from]; !ok {
		return fmt.Errorf("unknown upstream node %q", from)
	}
	if _, ok := g.nodes[to]; !ok {
		return fmt.Errorf("unknown downstream node %q", to)
	}
	g.outs[from][to] = struct{}{}
	g.ins[to][from] = struct{}{}
	return nil
}

func (g *Graph) Nodes() []string {
	out := make([]string, 0, len(g.nodes))
	for id := range g.nodes {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (g *Graph) Upstreams(id string) []string   { return sortedSet(g.ins[id]) }
func (g *Graph) Downstreams(id string) []string { return sortedSet(g.outs[id]) }

func sortedSet(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Validate detects cycles with DFS colors and reports the concrete path.
func (g *Graph) Validate() error {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(g.nodes))
	var stack []string
	var visit func(string) error
	visit = func(id string) error {
		color[id] = gray
		stack = append(stack, id)
		for _, next := range g.Downstreams(id) {
			switch color[next] {
			case gray:
				cycle := append(append([]string(nil), stack...), next)
				return fmt.Errorf("cycle detected: %v", cycle)
			case white:
				if err := visit(next); err != nil {
					return err
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
		return nil
	}
	for _, id := range g.Nodes() {
		if color[id] == white {
			if err := visit(id); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *Graph) TopoSort() ([]string, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}
	indeg := make(map[string]int, len(g.nodes))
	for id := range g.nodes {
		indeg[id] = len(g.ins[id])
	}
	var ready []string
	for id, n := range indeg {
		if n == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)
	out := make([]string, 0, len(g.nodes))
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		out = append(out, id)
		nexts := g.Downstreams(id)
		for _, next := range nexts {
			indeg[next]--
			if indeg[next] == 0 {
				ready = append(ready, next)
				sort.Strings(ready)
			}
		}
	}
	if len(out) != len(g.nodes) {
		return nil, fmt.Errorf("topological sort incomplete: %d/%d", len(out), len(g.nodes))
	}
	return out, nil
}

// Layers groups nodes that can run in parallel.
func (g *Graph) Layers() ([][]string, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}
	indeg := make(map[string]int, len(g.nodes))
	for id := range g.nodes {
		indeg[id] = len(g.ins[id])
	}
	var current []string
	for id, n := range indeg {
		if n == 0 {
			current = append(current, id)
		}
	}
	sort.Strings(current)
	var layers [][]string
	seen := 0
	for len(current) > 0 {
		layers = append(layers, append([]string(nil), current...))
		seen += len(current)
		var next []string
		for _, id := range current {
			for _, down := range g.Downstreams(id) {
				indeg[down]--
				if indeg[down] == 0 {
					next = append(next, down)
				}
			}
		}
		sort.Strings(next)
		current = next
	}
	if seen != len(g.nodes) {
		return nil, fmt.Errorf("layers incomplete: %d/%d", seen, len(g.nodes))
	}
	return layers, nil
}
