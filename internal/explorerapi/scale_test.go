package explorerapi

import (
	"fmt"
	"testing"

	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

func TestExplorerScaleProfilesStayBounded(t *testing.T) {
	for _, size := range []int{10, 500, 4_000, 10_000} {
		t.Run(fmt.Sprintf("entities-%d", size), func(t *testing.T) {
			dataset := scaleDataset(size)
			service, err := NewService(dataset)
			if err != nil {
				t.Fatal(err)
			}

			page, meta, err := service.List(ListOptions{Limit: 50, Sort: "id"})
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Items) > 50 || meta.Total != size || meta.Truncated != (size > 50) {
				t.Fatalf("list bounds = %d %+v", len(page.Items), meta)
			}

			hits, hitMeta, err := service.Search(SearchOptions{Query: "bounded catalog", Limit: 50})
			if err != nil {
				t.Fatal(err)
			}
			if len(hits.Items) > 50 || hitMeta.Total != size || hitMeta.Truncated != (size > 50) {
				t.Fatalf("search bounds = %d %+v", len(hits.Items), hitMeta)
			}

			graph, graphMeta, err := service.Graph(GraphOptions{Mode: explorercontract.GraphAggregate, GroupBy: explorercontract.GroupKind})
			if err != nil {
				t.Fatal(err)
			}
			if graphMeta.Total != size || len(graph.Nodes) > MaxGraphNodes || len(graph.Edges) > MaxGraphEdges {
				t.Fatalf("graph bounds = %d/%d %+v", len(graph.Nodes), len(graph.Edges), graphMeta)
			}

			dossier, dossierMeta, err := service.Dossier(dataset.Entities[size/2].ID, 1)
			if err != nil {
				t.Fatal(err)
			}
			if dossier.Entity.ID == "" || len(dossier.Incoming)+len(dossier.Outgoing) > 1 || !dossierMeta.Truncated {
				t.Fatalf("dossier bounds = %+v %+v", dossier, dossierMeta)
			}
		})
	}
}

func scaleDataset(size int) explorercontract.Dataset {
	entities := make([]explorercontract.Entity, size)
	edges := make([]explorercontract.Edge, 0, max(0, size-1))
	for i := range entities {
		entities[i] = explorercontract.Entity{
			ID:      fmt.Sprintf("service.entity-%05d", i),
			Name:    fmt.Sprintf("Catalog Entity %05d", i),
			Kind:    []string{"service", "system", "document", "web-app"}[i%4],
			Status:  "active",
			Surface: []string{"backend", "platform", "docs", "frontend"}[i%4],
			Summary: "A bounded catalog scale fixture.",
			Tags:    []string{"bounded", "catalog"},
		}
		if i > 0 {
			edges = append(edges, explorercontract.Edge{Source: entities[i-1].ID, Target: entities[i].ID, Kind: "depends_on"})
		}
	}
	return explorercontract.Dataset{
		SchemaVersion: explorercontract.SchemaVersion, ProjectionMode: explorercontract.ProjectionLocal,
		SourceDigest: fmt.Sprintf("sha256:scale-%d", size), Entities: entities, Edges: edges,
	}
}
