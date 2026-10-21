package labkit

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type metricKey struct {
	name   string
	labels string
}

type histogram struct {
	count   uint64
	sum     float64
	bounds  []float64
	buckets []uint64
}

// Registry is a tiny Prometheus-compatible registry. It exists so the lab can
// teach metric design without adding a dependency.
type Registry struct {
	mu       sync.Mutex
	counters map[metricKey]float64
	gauges   map[metricKey]float64
	hists    map[metricKey]*histogram
}

var defaultBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}

func NewRegistry() *Registry {
	return &Registry{
		counters: make(map[metricKey]float64),
		gauges:   make(map[metricKey]float64),
		hists:    make(map[metricKey]*histogram),
	}
}

func labelString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, labels[k]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

func (r *Registry) Inc(name string, labels map[string]string) {
	r.Add(name, 1, labels)
}

func (r *Registry) Add(name string, v float64, labels map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.counters[metricKey{name, labelString(labels)}] += v
}

func (r *Registry) SetGauge(name string, v float64, labels map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gauges[metricKey{name, labelString(labels)}] = v
}

func (r *Registry) Observe(name string, v float64, labels map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := metricKey{name, labelString(labels)}
	h := r.hists[k]
	if h == nil {
		h = &histogram{bounds: append([]float64(nil), defaultBuckets...), buckets: make([]uint64, len(defaultBuckets))}
		r.hists[k] = h
	}
	h.count++
	h.sum += v
	for i, b := range h.bounds {
		if v <= b {
			h.buckets[i]++
		}
	}
}

func (r *Registry) Render() string {
	r.mu.Lock()
	defer r.mu.Unlock()

	keys := make([]metricKey, 0, len(r.counters))
	for k := range r.counters {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].name == keys[j].name {
			return keys[i].labels < keys[j].labels
		}
		return keys[i].name < keys[j].name
	})
	var b strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&b, "%s%s %g\n", k.name, k.labels, r.counters[k])
	}

	gkeys := make([]metricKey, 0, len(r.gauges))
	for k := range r.gauges {
		gkeys = append(gkeys, k)
	}
	sort.Slice(gkeys, func(i, j int) bool { return gkeys[i].name+gkeys[i].labels < gkeys[j].name+gkeys[j].labels })
	for _, k := range gkeys {
		fmt.Fprintf(&b, "%s%s %g\n", k.name, k.labels, r.gauges[k])
	}

	hkeys := make([]metricKey, 0, len(r.hists))
	for k := range r.hists {
		hkeys = append(hkeys, k)
	}
	sort.Slice(hkeys, func(i, j int) bool { return hkeys[i].name+hkeys[i].labels < hkeys[j].name+hkeys[j].labels })
	for _, k := range hkeys {
		h := r.hists[k]
		for i, bound := range h.bounds {
			fmt.Fprintf(&b, "%s_bucket%s %d\n", k.name, mergeLabels(k.labels, bound), h.buckets[i])
		}
		fmt.Fprintf(&b, "%s_bucket%s %d\n", k.name, mergeLabels(k.labels, "+Inf"), h.count)
		fmt.Fprintf(&b, "%s_sum%s %g\n", k.name, k.labels, h.sum)
		fmt.Fprintf(&b, "%s_count%s %d\n", k.name, k.labels, h.count)
	}
	return b.String()
}

func mergeLabels(existing string, le any) string {
	leLabel := fmt.Sprintf("le=%q", fmt.Sprint(le))
	if existing == "" {
		return "{" + leLabel + "}"
	}
	return strings.TrimSuffix(existing, "}") + "," + leLabel + "}"
}
