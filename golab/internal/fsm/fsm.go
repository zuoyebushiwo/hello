// Package fsm models the node state machine of a dataflow scheduler: ready,
// running, success, failed and skipped, with retries and downstream skipping.
package fsm

import (
	"fmt"
	"sort"
	"sync"
)

type State string

const (
	Pending State = "pending"
	Ready   State = "ready"
	Running State = "running"
	Success State = "success"
	Failed  State = "failed"
	Skipped State = "skipped"
)

type Node struct {
	ID          string
	State       State
	Attempts    int
	MaxAttempts int
}

type Flow struct {
	mu          sync.Mutex
	nodes       map[string]*Node
	order       []string
	ups         map[string][]string
	downs       map[string][]string
	transitions []string
}

func New() *Flow {
	return &Flow{
		nodes: make(map[string]*Node),
		ups:   make(map[string][]string),
		downs: make(map[string][]string),
	}
}

func (f *Flow) AddNode(id string, maxAttempts int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.nodes[id]; ok {
		return fmt.Errorf("duplicate node %q", id)
	}
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	f.nodes[id] = &Node{ID: id, State: Pending, MaxAttempts: maxAttempts}
	f.order = append(f.order, id)
	return nil
}

func (f *Flow) AddEdge(from, to string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.nodes[from]; !ok {
		return fmt.Errorf("unknown upstream %q", from)
	}
	if _, ok := f.nodes[to]; !ok {
		return fmt.Errorf("unknown downstream %q", to)
	}
	for _, id := range f.downs[from] {
		if id == to {
			return nil
		}
	}
	f.downs[from] = append(f.downs[from], to)
	f.ups[to] = append(f.ups[to], from)
	sort.Strings(f.downs[from])
	sort.Strings(f.ups[to])
	return nil
}

func (f *Flow) record(id string, from, to State) {
	f.transitions = append(f.transitions, fmt.Sprintf("%s: %s -> %s", id, from, to))
}

func (f *Flow) Init() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range f.order {
		n := f.nodes[id]
		if len(f.ups[id]) == 0 && n.State == Pending {
			n.State = Ready
			f.record(id, Pending, Ready)
		}
	}
}

func (f *Flow) Start(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.nodes[id]
	if !ok {
		return fmt.Errorf("unknown node %q", id)
	}
	if n.State != Ready {
		return fmt.Errorf("node %s is %s, cannot start", id, n.State)
	}
	n.State = Running
	n.Attempts++
	f.record(id, Ready, Running)
	return nil
}

func (f *Flow) allUpstreamSuccess(id string) bool {
	for _, up := range f.ups[id] {
		if f.nodes[up].State != Success {
			return false
		}
	}
	return true
}

func (f *Flow) Succeed(id string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.nodes[id]
	if !ok {
		return fmt.Errorf("unknown node %q", id)
	}
	if n.State != Running {
		return fmt.Errorf("node %s is %s, cannot succeed", id, n.State)
	}
	n.State = Success
	f.record(id, Running, Success)
	for _, down := range f.downs[id] {
		d := f.nodes[down]
		if d.State == Pending && f.allUpstreamSuccess(down) {
			d.State = Ready
			f.record(down, Pending, Ready)
		}
	}
	return nil
}

// Fail returns retrying=true when the node was put back into Ready for another
// attempt. Otherwise the node is terminally failed and its downstream is skipped.
func (f *Flow) Fail(id string) (retrying bool, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.nodes[id]
	if !ok {
		return false, fmt.Errorf("unknown node %q", id)
	}
	if n.State != Running {
		return false, fmt.Errorf("node %s is %s, cannot fail", id, n.State)
	}
	if n.Attempts < n.MaxAttempts {
		n.State = Ready
		f.record(id, Running, Ready)
		return true, nil
	}
	n.State = Failed
	f.record(id, Running, Failed)
	f.skipDownstream(id)
	return false, nil
}

func (f *Flow) skipDownstream(id string) {
	for _, down := range f.downs[id] {
		n := f.nodes[down]
		if n.State == Pending || n.State == Ready {
			from := n.State
			n.State = Skipped
			f.record(down, from, Skipped)
			f.skipDownstream(down)
		}
	}
}

func (f *Flow) State(id string) State {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n, ok := f.nodes[id]; ok {
		return n.State
	}
	return ""
}

func (f *Flow) Attempts(id string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if n, ok := f.nodes[id]; ok {
		return n.Attempts
	}
	return 0
}

func (f *Flow) Final() State {
	f.mu.Lock()
	defer f.mu.Unlock()
	allDone := true
	for _, id := range f.order {
		switch f.nodes[id].State {
		case Failed:
			return Failed
		case Success, Skipped:
		default:
			allDone = false
		}
	}
	if allDone {
		return Success
	}
	return Running
}

func (f *Flow) Transitions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.transitions...)
}

func (f *Flow) Snapshot() []Node {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Node, 0, len(f.order))
	for _, id := range f.order {
		out = append(out, *f.nodes[id])
	}
	return out
}
