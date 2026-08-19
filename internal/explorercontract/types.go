// Package explorercontract owns the versioned data contract shared by the
// Explorer HTTP API, static bundle, CLI receipts, and MCP transport.
package explorercontract

import "encoding/json"

const (
	// SchemaVersion is the additive Explorer transport schema version.
	SchemaVersion = 1
	// StaticGenerator identifies an output directory that this product owns.
	StaticGenerator = "nicos-catalog-explorer"
)

// ProjectionMode selects the closed data boundary used by Explorer.
type ProjectionMode string

const (
	ProjectionLocal  ProjectionMode = "local"
	ProjectionPublic ProjectionMode = "public"
)

func (m ProjectionMode) Valid() bool { return m == ProjectionLocal || m == ProjectionPublic }

// Meta makes truncation and pagination explicit on every response.
type Meta struct {
	Truncated  bool   `json:"truncated"`
	NextCursor string `json:"next_cursor,omitempty"`
	Total      int    `json:"total,omitempty"`
	Notice     string `json:"notice,omitempty"`
}

// Error is a bounded error safe for a public transport.
type Error struct {
	Code    string `json:"code"`
	Summary string `json:"summary"`
}

// Envelope is the versioned HTTP and CLI receipt wrapper.
type Envelope struct {
	SchemaVersion  int             `json:"schema_version"`
	Command        string          `json:"command,omitempty"`
	OK             bool            `json:"ok"`
	ProjectionMode ProjectionMode  `json:"projection_mode"`
	SourceDigest   string          `json:"source_digest"`
	Data           json.RawMessage `json:"data,omitempty"`
	Error          *Error          `json:"error,omitempty"`
	Meta           Meta            `json:"meta"`
}

// Entity is the closed Explorer entity view. Local-only fields are absent in
// public mode rather than populated and filtered after serialization.
type Entity struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Kind            string   `json:"kind"`
	Status          string   `json:"status,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	Surface         string   `json:"surface,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	URL             string   `json:"url,omitempty"`
	OwnerLabel      string   `json:"owner_label,omitempty"`
	EntrypointLabel string   `json:"entrypoint_label,omitempty"`
}

// Edge exists only when both endpoints survived the active projection.
type Edge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
}

// Dataset is the in-memory authority used by every Explorer transport.
type Dataset struct {
	SchemaVersion  int             `json:"schema_version"`
	ProjectionMode ProjectionMode  `json:"projection_mode"`
	SourceDigest   string          `json:"source_digest"`
	Entities       []Entity        `json:"entities"`
	Edges          []Edge          `json:"edges"`
	Findings       []HealthFinding `json:"findings"`
}

// Status reports fixed product and active projection identity.
type Status struct {
	ProductVersion string `json:"product_version"`
	APISchema      int    `json:"api_schema_version"`
	EntityCount    int    `json:"entity_count"`
	EdgeCount      int    `json:"edge_count"`
	FindingCount   int    `json:"finding_count"`
}

// EntityPage is one stable, cursor-paginated catalog page.
type EntityPage struct {
	Items []Entity `json:"items"`
}

// StaticCatalog keeps projected entities and edges together for on-demand
// static page and region reads. The overview still loads aggregates only.
type StaticCatalog struct {
	Items []Entity `json:"items"`
	Edges []Edge   `json:"edges"`
}

// SearchHit is one scored result over projected fields only.
type SearchHit struct {
	Entity       Entity   `json:"entity"`
	Score        float64  `json:"score"`
	MatchedTerms []string `json:"matched_terms"`
}

// SearchPage is a bounded ranked result set.
type SearchPage struct {
	Items []SearchHit `json:"items"`
}

// EntityDetail is one entity plus its bounded direct relationships.
type EntityDetail struct {
	Entity   Entity `json:"entity"`
	Incoming []Edge `json:"incoming"`
	Outgoing []Edge `json:"outgoing"`
}

// GraphMode selects the progressive graph level.
type GraphMode string

const (
	GraphAggregate    GraphMode = "aggregate"
	GraphRegion       GraphMode = "region"
	GraphNeighborhood GraphMode = "neighborhood"
)

// GraphGroup selects the aggregate dimension.
type GraphGroup string

const (
	GroupKind    GraphGroup = "kind"
	GroupSurface GraphGroup = "surface"
)

// GraphNode can be one projected entity or one aggregate group.
type GraphNode struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind,omitempty"`
	Status    string `json:"status,omitempty"`
	Group     string `json:"group,omitempty"`
	Count     int    `json:"count,omitempty"`
	Aggregate bool   `json:"aggregate,omitempty"`
}

// GraphEdge is an entity edge or an aggregate edge with a stable count.
type GraphEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"`
	Count  int    `json:"count,omitempty"`
}

// Refinement explains why a graph request needs a narrower scope.
type Refinement struct {
	Code    string `json:"code"`
	Summary string `json:"summary"`
}

// GraphPage is a bounded progressive graph response.
type GraphPage struct {
	Mode       GraphMode   `json:"mode"`
	GroupBy    GraphGroup  `json:"group_by,omitempty"`
	Scope      string      `json:"scope,omitempty"`
	Depth      int         `json:"depth,omitempty"`
	Nodes      []GraphNode `json:"nodes"`
	Edges      []GraphEdge `json:"edges"`
	Refinement *Refinement `json:"refinement,omitempty"`
}

// HealthSeverity is the closed Explorer health vocabulary.
type HealthSeverity string

const (
	HealthError   HealthSeverity = "error"
	HealthWarning HealthSeverity = "warning"
	HealthInfo    HealthSeverity = "info"
)

// HealthFinding never carries a path, raw payload, or rejected value.
type HealthFinding struct {
	Code        string         `json:"code"`
	Severity    HealthSeverity `json:"severity"`
	EntityID    string         `json:"entity_id,omitempty"`
	Remediation string         `json:"remediation"`
}

// HealthReport is a bounded view of validation and drift state.
type HealthReport struct {
	OK       bool            `json:"ok"`
	Drift    string          `json:"drift"`
	Findings []HealthFinding `json:"findings"`
}

// ContentDigests binds each deterministic static data file.
type ContentDigests struct {
	Entities string `json:"entities"`
	Graph    string `json:"graph"`
	Health   string `json:"health"`
	Search   string `json:"search"`
}

// Manifest is the deterministic static Explorer root of trust.
type Manifest struct {
	SchemaVersion  int            `json:"schema_version"`
	Generator      string         `json:"generator"`
	ProductVersion string         `json:"product_version"`
	ProjectionMode ProjectionMode `json:"projection_mode"`
	SourceDigest   string         `json:"source_digest"`
	EntityCount    int            `json:"entity_count"`
	EdgeCount      int            `json:"edge_count"`
	FindingCount   int            `json:"finding_count"`
	Content        ContentDigests `json:"content"`
}

// ExportReceipt is the bounded CLI proof for a static export.
type ExportReceipt struct {
	Files         []string       `json:"files"`
	Content       ContentDigests `json:"content"`
	EntityCount   int            `json:"entity_count"`
	EdgeCount     int            `json:"edge_count"`
	FindingCount  int            `json:"finding_count"`
	SourceDigest  string         `json:"source_digest"`
	OutputChanged bool           `json:"output_changed"`
}

// InitReceipt reports only root-relative starter paths.
type InitReceipt struct {
	Template string   `json:"template"`
	DryRun   bool     `json:"dry_run"`
	Planned  []string `json:"planned"`
	Present  []string `json:"present"`
	Written  []string `json:"written"`
	Blocked  []string `json:"blocked"`
}

// ServeReceipt identifies a ready loopback server.
type ServeReceipt struct {
	URL         string `json:"url"`
	EntityCount int    `json:"entity_count"`
}
