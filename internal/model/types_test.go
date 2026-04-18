package model

import "testing"

func sampleGraph() Graph {
	return Graph{
		Nodes: []Node{
			{ID: "a", Type: "start"},
			{ID: "b", Type: "llm"},
			{ID: "c", Type: "end"},
		},
		Edges: []Edge{
			{ID: "e1", Source: "a", Target: "b"},
			{ID: "e2", Source: "b", Target: "c"},
		},
	}
}

func TestNodeByID_Found(t *testing.T) {
	g := sampleGraph()
	n := g.NodeByID("b")
	if n == nil || n.Type != "llm" {
		t.Fatalf("expected llm node, got %v", n)
	}
}

func TestNodeByID_Missing(t *testing.T) {
	g := sampleGraph()
	if g.NodeByID("zzz") != nil {
		t.Fatal("expected nil for missing id")
	}
}

func TestNodesByType(t *testing.T) {
	g := sampleGraph()
	starts := g.NodesByType("start")
	if len(starts) != 1 || starts[0].ID != "a" {
		t.Fatalf("expected 1 start (a), got %v", starts)
	}
}

func TestOutgoingIncoming(t *testing.T) {
	g := sampleGraph()
	if got := g.Outgoing("a"); len(got) != 1 || got[0].Target != "b" {
		t.Fatalf("outgoing a: %v", got)
	}
	if got := g.Incoming("c"); len(got) != 1 || got[0].Source != "b" {
		t.Fatalf("incoming c: %v", got)
	}
	if got := g.Outgoing("c"); len(got) != 0 {
		t.Fatalf("outgoing c should be empty, got %v", got)
	}
}
