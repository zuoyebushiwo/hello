package graph

import "testing"

func buildDiamond(t *testing.T) *Graph {
	t.Helper()
	g := New()
	for _, id := range []string{"start", "extract", "clean_a", "clean_b", "join", "publish"} {
		g.AddNode(id)
	}
	edges := [][2]string{
		{"start", "extract"},
		{"extract", "clean_a"},
		{"extract", "clean_b"},
		{"clean_a", "join"},
		{"clean_b", "join"},
		{"join", "publish"},
	}
	for _, e := range edges {
		if err := g.AddEdge(e[0], e[1]); err != nil {
			t.Fatal(err)
		}
	}
	return g
}

func TestLayers(t *testing.T) {
	g := buildDiamond(t)
	layers, err := g.Layers()
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"start"}, {"extract"}, {"clean_a", "clean_b"}, {"join"}, {"publish"}}
	if len(layers) != len(want) {
		t.Fatalf("layers = %v, want %v", layers, want)
	}
	for i := range want {
		if len(layers[i]) != len(want[i]) {
			t.Fatalf("layer %d = %v, want %v", i, layers[i], want[i])
		}
		for j := range want[i] {
			if layers[i][j] != want[i][j] {
				t.Fatalf("layer %d = %v, want %v", i, layers[i], want[i])
			}
		}
	}
}

func TestCycleDetected(t *testing.T) {
	g := New()
	for _, id := range []string{"a", "b", "c"} {
		g.AddNode(id)
	}
	_ = g.AddEdge("a", "b")
	_ = g.AddEdge("b", "c")
	_ = g.AddEdge("c", "a")
	if err := g.Validate(); err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestUnknownNode(t *testing.T) {
	g := New()
	g.AddNode("a")
	if err := g.AddEdge("a", "missing"); err == nil {
		t.Fatal("expected unknown node error")
	}
}
