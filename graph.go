package catalog

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

type GraphNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Status string `json:"status,omitempty"`
}

type GraphEdge struct {
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

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

func escapeMermaid(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\\", "\\\\"), "\"", "\\\"")
}
