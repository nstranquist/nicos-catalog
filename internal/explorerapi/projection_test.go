package explorerapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	catalog "github.com/nstranquist/nicos-catalog"
	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

func projectionIndex() catalog.Index {
	return catalog.Index{
		SchemaVersion: catalog.SchemaVersion, SourceDigest: "sha256:test-source",
		Entities: []catalog.Entity{
			{ID: "system.orchard", Name: "Orchard", Kind: "system", Surface: "platform", Status: "shipped", Description: "A public platform.", Owner: "Platform Team", Entrypoint: "cmd/orchard", Visibility: catalog.VisibilityPublic, PublicURL: "https://example.com/orchard", Tags: []string{"demo", "platform"}, Refs: []catalog.Ref{{Kind: "contains", Target: "service.seed-api"}, {Kind: "contains", Target: "telemetry.private"}}},
			{ID: "service.seed-api", Name: "Seed API", Kind: "service", Surface: "backend", Status: "beta", Description: "Searchable seed inventory.", Owner: "API Team", Entrypoint: "/Users/example/dev/seed/main.go", Visibility: catalog.VisibilityPublic, Tags: []string{"demo", "go"}},
			{ID: "telemetry.private", Name: "Private Telemetry", Kind: "telemetry", Description: "Synthetic local record.", Visibility: catalog.VisibilityPrivate, Annotations: map[string]string{"secret": "not-output"}},
		},
	}
}

func TestCompileLocalAndPublicAreClosed(t *testing.T) {
	index := projectionIndex()
	local, err := Compile(context.Background(), index, explorercontract.ProjectionLocal, catalog.ProjectionPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if len(local.Entities) != 3 || len(local.Edges) != 2 {
		t.Fatalf("local counts = %d/%d", len(local.Entities), len(local.Edges))
	}
	if got := local.Entities[0].EntrypointLabel; got != "main.go" {
		t.Fatalf("absolute entrypoint label = %q", got)
	}
	encoded := mustJSON(t, local)
	for _, forbidden := range []string{"/Users/example", "annotations", "not-output"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("local projection leaked %q: %s", forbidden, encoded)
		}
	}

	public, err := Compile(context.Background(), index, explorercontract.ProjectionPublic, catalog.ProjectionPolicy{RequireVisibility: catalog.VisibilityPublic, AllowHosts: []string{"example.com"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(public.Entities) != 2 || len(public.Edges) != 1 {
		t.Fatalf("public counts = %d/%d", len(public.Entities), len(public.Edges))
	}
	encoded = mustJSON(t, public)
	for _, forbidden := range []string{"Platform Team", "\"surface\"", "telemetry.private", "entrypoint_label", "owner_label"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("public projection leaked %q: %s", forbidden, encoded)
		}
	}
	if public.Entities[0].ID != "service.seed-api" || public.Entities[1].ID != "system.orchard" {
		t.Fatalf("entities are not stable: %+v", public.Entities)
	}
}

func TestCompileRejectsUnsafeProjectedCanariesWithoutEcho(t *testing.T) {
	canaries := []string{
		"/Users/private-person/secret/file.txt",
		"token=gh" + "p_ABCDEFGHIJKLMNOPQRSTUVWXYZ123456",
		"service.private-host-canary.internal",
	}
	for _, canary := range canaries {
		t.Run(canary[:min(8, len(canary))], func(t *testing.T) {
			index := projectionIndex()
			index.Entities[0].Description = canary
			_, err := Compile(context.Background(), index, explorercontract.ProjectionLocal, catalog.ProjectionPolicy{})
			if err == nil {
				t.Fatal("unsafe content was accepted")
			}
			if strings.Contains(err.Error(), canary) {
				t.Fatalf("error echoed canary: %v", err)
			}
		})
	}
}

func TestCompileCancellationAndModeValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Compile(ctx, projectionIndex(), explorercontract.ProjectionLocal, catalog.ProjectionPolicy{}); err == nil {
		t.Fatal("canceled compile succeeded")
	}
	if _, err := Compile(context.Background(), projectionIndex(), "private", catalog.ProjectionPolicy{}); err == nil {
		t.Fatal("private mode succeeded")
	}
}

func TestProjectionHelpersCoverBoundsAndFindings(t *testing.T) {
	if got := boundedLabel("  short  ", 20); got != "short" {
		t.Fatalf("bounded short = %q", got)
	}
	if got := boundedLabel("ééééé", 7); len(got) > 7 || !strings.HasSuffix(got, "…") {
		t.Fatalf("bounded UTF-8 = %q (%d)", got, len(got))
	}
	if got := cloneStrings(nil); got != nil {
		t.Fatalf("nil clone = %#v", got)
	}
	dataset := explorercontract.Dataset{
		Entities: []explorercontract.Entity{{ID: "service.a"}},
		Edges:    []explorercontract.Edge{{Source: "service.a", Target: "service.a", Kind: "depends_on"}, {Source: "service.a", Target: "service.a", Kind: "depends_on"}, {Source: "service.a", Target: "service.missing", Kind: "uses"}},
	}
	findings := projectedFindings(dataset)
	if len(findings) != 4 {
		t.Fatalf("findings = %+v", findings)
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}
