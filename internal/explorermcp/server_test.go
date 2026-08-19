package explorermcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nstranquist/nicos-catalog/internal/explorerapi"
	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

func mcpService(t *testing.T) *explorerapi.Service {
	t.Helper()
	service, err := explorerapi.NewService(explorercontract.Dataset{
		SchemaVersion: 1, ProjectionMode: explorercontract.ProjectionLocal, SourceDigest: "sha256:mcp",
		Entities: []explorercontract.Entity{{ID: "service.seed-api", Name: "Seed API", Kind: "service", Summary: "Searchable seed inventory"}, {ID: "system.orchard", Name: "Orchard", Kind: "system", Summary: "Developer platform"}},
		Edges:    []explorercontract.Edge{{Source: "system.orchard", Target: "service.seed-api", Kind: "contains"}},
		Findings: []explorercontract.HealthFinding{{Code: "notice", Severity: explorercontract.HealthInfo, Remediation: "Review the synthetic notice."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestMCPHandshakeListAndReadOnlyTools(t *testing.T) {
	server, err := New(mcpService(t), "v0.3.0")
	if err != nil {
		t.Fatal(err)
	}
	messages := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"catalog_search","arguments":{"query":"seed"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"catalog_get","arguments":{"id":"system.orchard"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"catalog_graph","arguments":{"mode":"aggregate"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"catalog_health","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"ping"}`,
	}
	var output bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(strings.Join(messages, "\n")+"\n"), &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 7 {
		t.Fatalf("responses=%d\n%s", len(lines), output.String())
	}
	if strings.Contains(output.String(), "write") || strings.Contains(output.String(), "shell") || strings.Contains(output.String(), "source_path") {
		t.Fatalf("write surface leaked: %s", output.String())
	}
	for _, name := range []string{"catalog_search", "catalog_get", "catalog_graph", "catalog_health"} {
		if !strings.Contains(output.String(), name) && name != "catalog_search" {
			t.Fatalf("tools list missing %s", name)
		}
	}
	for _, line := range lines {
		var response map[string]any
		if err := json.Unmarshal([]byte(line), &response); err != nil || response["jsonrpc"] != "2.0" {
			t.Fatalf("response=%q err=%v", line, err)
		}
	}
}

func TestMCPErrorsAreBoundedAndDoNotEchoArguments(t *testing.T) {
	server, _ := New(mcpService(t), "v0.3.0")
	canary := "private-argument-canary"
	inputs := []string{
		`not-json`,
		`{"jsonrpc":"1.0","id":1,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":2,"method":"unknown"}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"catalog_search","arguments":{"query":"seed","unknown":"` + canary + `"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"catalog_search","arguments":{"query":"seed","limit":21}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"catalog_get","arguments":{"id":"BAD ID"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"catalog_graph","arguments":{"mode":"full"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"catalog_health","arguments":{"limit":51}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"catalog_write","arguments":{}}}`,
	}
	var output bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(strings.Join(inputs, "\n")+"\n"), &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), canary) {
		t.Fatalf("error echoed canary: %s", output.String())
	}
	if output.Len() > len(inputs)*(maxResultBytes+4096) {
		t.Fatalf("unbounded output: %d", output.Len())
	}
}

func TestMCPResultCeilingCancellationAndInputBound(t *testing.T) {
	entities := make([]explorercontract.Entity, 200)
	edges := make([]explorercontract.Edge, 0, 199)
	for i := range entities {
		entities[i] = explorercontract.Entity{ID: fmt.Sprintf("service.large-%03d", i), Name: strings.Repeat("Long synthetic name ", 40), Kind: "service"}
		if i > 0 {
			edges = append(edges, explorercontract.Edge{Source: entities[i-1].ID, Target: entities[i].ID, Kind: "links"})
		}
	}
	service, err := explorerapi.NewService(explorercontract.Dataset{SchemaVersion: 1, ProjectionMode: explorercontract.ProjectionLocal, SourceDigest: "sha256:large", Entities: entities, Edges: edges})
	if err != nil {
		t.Fatal(err)
	}
	server, _ := New(service, "v0.3.0")
	var output bytes.Buffer
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"catalog_graph","arguments":{"mode":"region","group_by":"kind","group":"service"}}}` + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if output.Len() > maxResultBytes+4096 || !strings.Contains(output.String(), "isError") {
		t.Fatalf("large result = %d %s", output.Len(), output.String())
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := server.Serve(canceled, strings.NewReader(""), &bytes.Buffer{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel = %v", err)
	}
	tooLarge := strings.NewReader(strings.Repeat("x", maxInputBytes+1))
	if err := server.Serve(context.Background(), tooLarge, &bytes.Buffer{}); err == nil {
		t.Fatal("oversized input succeeded")
	}
}

func TestMCPConstructionAndStrictDecode(t *testing.T) {
	if _, err := New(nil, "v0.3.0"); err == nil {
		t.Fatal("nil service accepted")
	}
	var target map[string]any
	if err := decodeStrict([]byte(`{} {}`), &target); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	if err := decodeStrict(nil, &target); err != nil {
		t.Fatal(err)
	}
	if result := toolError(&explorerapi.QueryError{Code: "bad", Summary: "Safe summary."}); !result.IsError || result.Content[0].Text != "Safe summary." {
		t.Fatalf("tool error = %+v", result)
	}
}
