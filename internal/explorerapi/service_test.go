package explorerapi

import (
	"fmt"
	"strings"
	"testing"

	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

func serviceFixture(t *testing.T) *Service {
	t.Helper()
	dataset := explorercontract.Dataset{
		SchemaVersion: explorercontract.SchemaVersion, ProjectionMode: explorercontract.ProjectionLocal, SourceDigest: "sha256:fixture",
		Entities: []explorercontract.Entity{
			{ID: "doc.guide", Name: "Growing Guide", Kind: "document", Status: "maintained", Surface: "docs", Summary: "Orchard onboarding guide", Tags: []string{"docs"}},
			{ID: "service.seed-api", Name: "Seed API", Kind: "service", Status: "shipped", Surface: "backend", Summary: "Go seed inventory service", Tags: []string{"go", "demo"}},
			{ID: "service.worker", Name: "Pollinator Worker", Kind: "service", Status: "beta", Surface: "backend", Summary: "Processes orchard events", Tags: []string{"worker"}},
			{ID: "system.orchard", Name: "Orchard Platform", Kind: "system", Status: "shipped", Surface: "platform", Summary: "Developer platform ownership graph", Tags: []string{"demo"}},
			{ID: "web.console", Name: "Grove Console", Kind: "web-app", Status: "beta", Surface: "frontend", Summary: "React catalog explorer", Tags: []string{"react"}},
		},
		Edges: []explorercontract.Edge{
			{Source: "doc.guide", Target: "system.orchard", Kind: "documents"},
			{Source: "service.seed-api", Target: "web.console", Kind: "serves"},
			{Source: "service.worker", Target: "service.seed-api", Kind: "depends_on"},
			{Source: "system.orchard", Target: "service.seed-api", Kind: "contains"},
			{Source: "system.orchard", Target: "web.console", Kind: "contains"},
		},
		Findings: []explorercontract.HealthFinding{{Code: "self_reference", Severity: explorercontract.HealthWarning, EntityID: "system.orchard", Remediation: "Remove it."}},
	}
	service, err := NewService(dataset)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestListPaginationFiltersAndSort(t *testing.T) {
	s := serviceFixture(t)
	page, meta, err := s.List(ListOptions{Limit: 2, Sort: "id"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 2 || !meta.Truncated || meta.NextCursor == "" || meta.Total != 5 {
		t.Fatalf("page/meta = %+v %+v", page, meta)
	}
	second, secondMeta, err := s.List(ListOptions{Limit: 3, Sort: "id", Cursor: meta.NextCursor})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 3 || secondMeta.Truncated {
		t.Fatalf("second = %+v %+v", second, secondMeta)
	}
	filtered, _, err := s.List(ListOptions{Kinds: []string{"service"}, Tags: []string{"go"}, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered.Items) != 1 || filtered.Items[0].ID != "service.seed-api" {
		t.Fatalf("filtered = %+v", filtered)
	}
	if _, _, err := s.List(ListOptions{Cursor: "bad"}); err == nil {
		t.Fatal("bad cursor succeeded")
	}
	if _, _, err := s.List(ListOptions{Sort: "score"}); err == nil {
		t.Fatal("bad sort succeeded")
	}
	if _, _, err := s.List(ListOptions{Direction: "sideways"}); err == nil {
		t.Fatal("bad direction succeeded")
	}
}

func TestProjectedBM25Search(t *testing.T) {
	s := serviceFixture(t)
	page, meta, err := s.Search(SearchOptions{Query: "seed inventory", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) == 0 || page.Items[0].Entity.ID != "service.seed-api" || meta.Total == 0 {
		t.Fatalf("search = %+v", page)
	}
	page, _, err = s.Search(SearchOptions{Query: "platform", Kinds: []string{"system"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Entity.ID != "system.orchard" {
		t.Fatalf("filtered search = %+v", page)
	}
	for _, options := range []SearchOptions{{}, {Query: strings.Repeat("x", 257)}, {Query: "seed", Limit: 51}} {
		if _, _, err := s.Search(options); err == nil {
			t.Fatalf("invalid search succeeded: %+v", options)
		}
	}
}

func TestEntityDetailAndProgressiveGraph(t *testing.T) {
	s := serviceFixture(t)
	detail, meta, err := s.EntityDetail("system.orchard", 1)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Entity.ID != "system.orchard" || !meta.Truncated || len(detail.Outgoing) != 1 {
		t.Fatalf("page = %+v %+v", detail, meta)
	}
	if _, _, err := s.EntityDetail("BAD ID", 10); err == nil {
		t.Fatal("invalid id succeeded")
	}
	if _, _, err := s.EntityDetail("service.missing", 10); err == nil {
		t.Fatal("missing id succeeded")
	}

	aggregate, _, err := s.Graph(GraphOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if aggregate.Mode != explorercontract.GraphAggregate || len(aggregate.Nodes) != 4 {
		t.Fatalf("aggregate = %+v", aggregate)
	}
	region, _, err := s.Graph(GraphOptions{Mode: explorercontract.GraphRegion, GroupBy: explorercontract.GroupSurface, Group: "backend"})
	if err != nil {
		t.Fatal(err)
	}
	if len(region.Nodes) != 2 || region.Scope != "backend" {
		t.Fatalf("region = %+v", region)
	}
	neighbor, _, err := s.Graph(GraphOptions{Mode: explorercontract.GraphNeighborhood, Entity: "system.orchard", Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(neighbor.Nodes) < 3 || neighbor.Depth != 2 {
		t.Fatalf("neighbor = %+v", neighbor)
	}
	refined, meta, err := s.Graph(GraphOptions{Mode: explorercontract.GraphRegion, GroupBy: explorercontract.GroupSurface, Group: "backend", MaxNodes: 1, MaxEdges: 10})
	if err != nil {
		t.Fatal(err)
	}
	if refined.Refinement == nil || meta.Notice != "refinement_required" {
		t.Fatalf("refinement = %+v %+v", refined, meta)
	}
	for _, options := range []GraphOptions{{Mode: "full"}, {GroupBy: "owner"}, {Mode: explorercontract.GraphNeighborhood, Entity: "system.orchard", Depth: 3}, {MaxNodes: 501}} {
		if _, _, err := s.Graph(options); err == nil {
			t.Fatalf("invalid graph succeeded: %+v", options)
		}
	}
}

func TestHealthBoundsAndValidation(t *testing.T) {
	s := serviceFixture(t)
	report, meta, err := s.Health(explorercontract.HealthWarning, 1)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK || len(report.Findings) != 1 || meta.Total != 1 {
		t.Fatalf("health = %+v %+v", report, meta)
	}
	if _, _, err := s.Health("fatal", 10); err == nil {
		t.Fatal("invalid severity succeeded")
	}
	if _, _, err := s.Health("", 101); err == nil {
		t.Fatal("invalid limit succeeded")
	}
}

func TestNewServiceRejectsBrokenDatasets(t *testing.T) {
	for name, dataset := range map[string]explorercontract.Dataset{
		"mode":      {SchemaVersion: 1, ProjectionMode: "private"},
		"id":        {SchemaVersion: 1, ProjectionMode: explorercontract.ProjectionLocal, Entities: []explorercontract.Entity{{ID: "BAD"}}},
		"duplicate": {SchemaVersion: 1, ProjectionMode: explorercontract.ProjectionLocal, Entities: []explorercontract.Entity{{ID: "service.a"}, {ID: "service.a"}}},
		"edge":      {SchemaVersion: 1, ProjectionMode: explorercontract.ProjectionLocal, Entities: []explorercontract.Entity{{ID: "service.a"}}, Edges: []explorercontract.Edge{{Source: "service.a", Target: "service.b"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewService(dataset); err == nil {
				t.Fatal("broken dataset succeeded")
			}
		})
	}
}

func TestServiceSimpleAccessorsAndError(t *testing.T) {
	s := serviceFixture(t)
	if s.Dataset().SourceDigest != "sha256:fixture" {
		t.Fatal("dataset accessor")
	}
	err := usageError("bad", "Bad request.")
	if err.Error() != "bad: Bad request." {
		t.Fatalf("error = %q", err.Error())
	}
}

func BenchmarkSearch(b *testing.B) {
	for _, size := range []int{500, 4000, 10000} {
		b.Run(fmt.Sprintf("entities_%d", size), func(b *testing.B) {
			entities := make([]explorercontract.Entity, size)
			for i := range entities {
				entities[i] = explorercontract.Entity{ID: fmt.Sprintf("service.synthetic-%05d", i), Name: fmt.Sprintf("Synthetic Service %05d", i), Kind: "service", Summary: "deterministic ownership graph benchmark"}
			}
			s, _ := NewService(explorercontract.Dataset{SchemaVersion: 1, ProjectionMode: explorercontract.ProjectionLocal, SourceDigest: "sha256:bench", Entities: entities})
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _, _ = s.Search(SearchOptions{Query: "ownership", Limit: 20})
			}
		})
	}
}
