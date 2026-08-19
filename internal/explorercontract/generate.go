package explorercontract

//go:generate go run ../../cmd/explorer-contract-gen --root ../..

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

var contractTypes = []reflect.Type{
	reflect.TypeOf(Meta{}), reflect.TypeOf(Error{}), reflect.TypeOf(Envelope{}),
	reflect.TypeOf(Entity{}), reflect.TypeOf(Edge{}), reflect.TypeOf(Dataset{}),
	reflect.TypeOf(Status{}), reflect.TypeOf(EntityPage{}), reflect.TypeOf(SearchHit{}),
	reflect.TypeOf(SearchPage{}), reflect.TypeOf(Dossier{}), reflect.TypeOf(GraphNode{}),
	reflect.TypeOf(GraphEdge{}), reflect.TypeOf(Refinement{}), reflect.TypeOf(GraphPage{}),
	reflect.TypeOf(HealthFinding{}), reflect.TypeOf(HealthReport{}),
	reflect.TypeOf(ContentDigests{}), reflect.TypeOf(Manifest{}),
	reflect.TypeOf(ExportReceipt{}), reflect.TypeOf(InitReceipt{}), reflect.TypeOf(ServeReceipt{}),
}

var enumValues = map[reflect.Type][]string{
	reflect.TypeOf(ProjectionMode("")): {string(ProjectionLocal), string(ProjectionPublic)},
	reflect.TypeOf(GraphMode("")):      {string(GraphAggregate), string(GraphRegion), string(GraphNeighborhood)},
	reflect.TypeOf(GraphGroup("")):     {string(GroupKind), string(GroupSurface)},
	reflect.TypeOf(HealthSeverity("")): {string(HealthError), string(HealthWarning), string(HealthInfo)},
}

// Generated returns the canonical JSON Schema and TypeScript contract bytes.
func Generated() ([]byte, []byte, error) {
	defs := map[string]any{}
	for _, typ := range contractTypes {
		defs[typ.Name()] = schemaFor(typ)
	}
	root := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://github.com/nstranquist/nicos-catalog/schema/explorer-v1.schema.json",
		"title":   "Nicos Catalog Explorer v1",
		"anyOf": []any{
			map[string]any{"$ref": "#/$defs/Envelope"},
			map[string]any{"$ref": "#/$defs/Manifest"},
		},
		"$defs": defs,
	}
	schema, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	schema = append(schema, '\n')
	return schema, []byte(typeScriptContract), nil
}

func schemaFor(typ reflect.Type) map[string]any {
	if values, ok := enumValues[typ]; ok {
		return map[string]any{"type": "string", "enum": values}
	}
	switch typ.Kind() {
	case reflect.Struct:
		properties := map[string]any{}
		var required []string
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name, optional := jsonField(field)
			if name == "" || name == "-" {
				continue
			}
			properties[name] = schemaValue(field.Type)
			if !optional {
				required = append(required, name)
			}
		}
		sort.Strings(required)
		out := map[string]any{"type": "object", "additionalProperties": false, "properties": properties}
		if len(required) > 0 {
			out["required"] = required
		}
		return out
	default:
		return schemaValue(typ)
	}
}

func schemaValue(typ reflect.Type) map[string]any {
	if values, ok := enumValues[typ]; ok {
		return map[string]any{"type": "string", "enum": values}
	}
	if typ.Kind() == reflect.Pointer {
		return map[string]any{"anyOf": []any{schemaValue(typ.Elem()), map[string]any{"type": "null"}}}
	}
	if typ.Name() != "" && typ.PkgPath() == reflect.TypeOf(Entity{}).PkgPath() && typ.Kind() == reflect.Struct {
		return map[string]any{"$ref": "#/$defs/" + typ.Name()}
	}
	switch typ.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.Slice:
		if typ.Elem().Kind() == reflect.Uint8 {
			return map[string]any{}
		}
		return map[string]any{"type": "array", "items": schemaValue(typ.Elem())}
	case reflect.Interface:
		return map[string]any{}
	default:
		return map[string]any{}
	}
}

func jsonField(field reflect.StructField) (string, bool) {
	parts := strings.Split(field.Tag.Get("json"), ",")
	name := parts[0]
	if name == "" {
		name = field.Name
	}
	for _, part := range parts[1:] {
		if part == "omitempty" {
			return name, true
		}
	}
	return name, false
}

// WriteGenerated writes or checks the repository-owned generated artifacts.
func WriteGenerated(root string, check bool) error {
	schema, types, err := Generated()
	if err != nil {
		return err
	}
	files := map[string][]byte{
		filepath.Join(root, "schema", "explorer-v1.schema.json"):           schema,
		filepath.Join(root, "explorer", "src", "generated", "contract.ts"): types,
	}
	for path, want := range files {
		if check {
			got, readErr := os.ReadFile(path)
			if readErr != nil || !bytes.Equal(got, want) {
				return fmt.Errorf("generated Explorer contract differs: run go generate ./internal/explorercontract")
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, want, 0o644); err != nil {
			return err
		}
	}
	return nil
}

const typeScriptContract = `// Code generated by go run ./cmd/explorer-contract-gen; DO NOT EDIT.
// Source: internal/explorercontract/types.go

export type ProjectionMode = 'local' | 'public'
export type GraphMode = 'aggregate' | 'region' | 'neighborhood'
export type GraphGroup = 'kind' | 'surface'
export type HealthSeverity = 'error' | 'warning' | 'info'

export interface Meta { truncated: boolean; next_cursor?: string; total?: number; notice?: string }
export interface ExplorerError { code: string; summary: string }
export interface Envelope<T = unknown> {
  schema_version: 1
  command?: string
  ok: boolean
  projection_mode: ProjectionMode
  source_digest: string
  data?: T
  error?: ExplorerError
  meta: Meta
}
export interface Entity {
  id: string; name: string; kind: string; status?: string; summary?: string
  surface?: string; tags?: string[]; url?: string; owner_label?: string; entrypoint_label?: string
}
export interface Edge { source: string; target: string; kind: string }
export interface Status { product_version: string; api_schema_version: number; entity_count: number; edge_count: number; finding_count: number }
export interface EntityPage { items: Entity[] }
export interface SearchHit { entity: Entity; score: number; matched_terms: string[] }
export interface SearchPage { items: SearchHit[] }
export interface Dossier { entity: Entity; incoming: Edge[]; outgoing: Edge[] }
export interface GraphNode { id: string; name: string; kind?: string; status?: string; group?: string; count?: number; aggregate?: boolean }
export interface GraphEdge { source: string; target: string; kind: string; count?: number }
export interface Refinement { code: string; summary: string }
export interface GraphPage { mode: GraphMode; group_by?: GraphGroup; scope?: string; depth?: number; nodes: GraphNode[]; edges: GraphEdge[]; refinement?: Refinement }
export interface HealthFinding { code: string; severity: HealthSeverity; entity_id?: string; remediation: string }
export interface HealthReport { ok: boolean; drift: string; findings: HealthFinding[] }
export interface ContentDigests { entities: string; graph: string; health: string; search: string }
export interface Manifest {
  schema_version: 1; generator: 'nicos-catalog-explorer'; product_version: string
  projection_mode: 'public'; source_digest: string; entity_count: number; edge_count: number
  finding_count: number; content: ContentDigests
}
`
