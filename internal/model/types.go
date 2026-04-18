// Package model defines the in-memory representation of a Dify workflow DSL.
//
// We deliberately keep the data model loose (map[string]any for node data) so
// we can surface rule violations without tightly coupling to every upstream
// schema version. The raw yaml.Node tree is retained alongside parsed structs
// so lint rules can report accurate line numbers.
package model

import "gopkg.in/yaml.v3"

// Workflow is the root document of a Dify DSL file.
type Workflow struct {
	App      App          `yaml:"app"`
	Kind     string       `yaml:"kind"`
	Version  string       `yaml:"version"`
	Workflow GraphWrapper `yaml:"workflow"`
	RawRoot  *yaml.Node   `yaml:"-"`
	// Path is the on-disk location (if any) for error messages.
	Path string `yaml:"-"`
}

// App captures metadata about the workflow application.
type App struct {
	Name        string `yaml:"name"`
	Mode        string `yaml:"mode"`
	Description string `yaml:"description"`
}

// GraphWrapper matches the Dify envelope `workflow.graph`.
type GraphWrapper struct {
	Graph Graph `yaml:"graph"`
}

// Graph holds the node and edge lists that form the workflow.
type Graph struct {
	Nodes []Node `yaml:"nodes"`
	Edges []Edge `yaml:"edges"`
}

// Node is a single unit in the workflow graph. Data is kept as a generic map
// because different node types have vastly different shapes.
type Node struct {
	ID       string         `yaml:"id"`
	Type     string         `yaml:"type"`
	Data     map[string]any `yaml:"data"`
	Position map[string]any `yaml:"position,omitempty"`
	// Line is the 1-based source line; 0 if unknown.
	Line int `yaml:"-"`
}

// Edge connects two nodes.
type Edge struct {
	ID           string `yaml:"id,omitempty"`
	Source       string `yaml:"source"`
	Target       string `yaml:"target"`
	SourceHandle string `yaml:"sourceHandle,omitempty"`
	TargetHandle string `yaml:"targetHandle,omitempty"`
	Line         int    `yaml:"-"`
}

// NodeByID returns the node with matching id or nil.
func (g Graph) NodeByID(id string) *Node {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

// NodesByType returns all nodes whose type matches the given kind.
func (g Graph) NodesByType(t string) []*Node {
	var out []*Node
	for i := range g.Nodes {
		if g.Nodes[i].Type == t {
			out = append(out, &g.Nodes[i])
		}
	}
	return out
}

// Outgoing returns edges whose source is the given node id.
func (g Graph) Outgoing(id string) []Edge {
	var out []Edge
	for _, e := range g.Edges {
		if e.Source == id {
			out = append(out, e)
		}
	}
	return out
}

// Incoming returns edges whose target is the given node id.
func (g Graph) Incoming(id string) []Edge {
	var out []Edge
	for _, e := range g.Edges {
		if e.Target == id {
			out = append(out, e)
		}
	}
	return out
}
