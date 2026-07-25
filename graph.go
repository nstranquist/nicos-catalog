package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// Graph is the compiled entity relationship graph.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphNode is one entity in the graph.
type GraphNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Status string `json:"status,omitempty"`
}

// GraphEdge is one typed relationship between two entities. A Target need not
// correspond to a Node; the host decides how to render a dangling edge.
type GraphEdge struct {
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

// BuildGraph compiles the typed relationship graph from an index. Nodes follow
// entity order; edges are sorted by source, kind, then target so the result is
// byte-stable.
func BuildGraph(index Index) Graph {
	graph := Graph{}
	for _, entity := range index.Entities {
		graph.Nodes = append(graph.Nodes, GraphNode{ID: entity.ID, Name: entity.Name, Kind: entity.Kind, Status: entity.Status})
		for _, ref := range entity.Refs {
			graph.Edges = append(graph.Edges, GraphEdge{Source: entity.ID, Kind: ref.Kind, Target: ref.Target})
		}
	}
	sort.Slice(graph.Edges, func(i, j int) bool {
		if graph.Edges[i].Source != graph.Edges[j].Source {
			return graph.Edges[i].Source < graph.Edges[j].Source
		}
		if graph.Edges[i].Kind != graph.Edges[j].Kind {
			return graph.Edges[i].Kind < graph.Edges[j].Kind
		}
		return graph.Edges[i].Target < graph.Edges[j].Target
	})
	return graph
}

// Mermaid renders the graph as a Mermaid flowchart. Labels are escaped so that
// no name or kind can break the diagram's line structure.
func (g Graph) Mermaid() string {
	var builder strings.Builder
	builder.WriteString("graph LR\n")
	for _, node := range g.Nodes {
		fmt.Fprintf(&builder, "  %s[\"%s\"]\n", mermaidID(node.ID), escapeMermaid(node.Name))
	}
	for _, edge := range g.Edges {
		fmt.Fprintf(&builder, "  %s -- \"%s\" --> %s\n", mermaidID(edge.Source), escapeMermaid(edge.Kind), mermaidID(edge.Target))
	}
	return builder.String()
}

func mermaidID(value string) string {
	var builder strings.Builder
	builder.WriteString("n_")
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('_')
		}
	}
	digest := sha256.Sum256([]byte(value))
	return builder.String() + "_" + hex.EncodeToString(digest[:4])
}

// escapeMermaid renders value safe to embed inside a quoted Mermaid label.
//
// Backslash escaping alone is not enough: Mermaid is line-oriented, so a raw
// newline or carriage return inside a label ends the statement and corrupts
// every following line of the diagram. Control characters are therefore
// rendered as Mermaid numeric entities rather than escaped in place.
func escapeMermaid(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))
	for _, r := range value {
		switch {
		case r == '#':
			// Escaped first; otherwise the entities emitted below would
			// themselves be re-escaped on a second pass.
			builder.WriteString("#35;")
		case r == '"':
			builder.WriteString("#quot;")
		case r == '\\':
			builder.WriteString("#92;")
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&builder, "#%d;", r)
		default:
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
