package explorerapi

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	catalog "github.com/nstranquist/nicos-catalog"
	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

const (
	// MaxGraphNodes is the largest node set that one service response can carry.
	MaxGraphNodes = 500
	// MaxGraphEdges is the largest edge set that one service response can carry.
	MaxGraphEdges = 1500
	// MaxGraphDepth is the largest supported neighborhood traversal depth.
	MaxGraphDepth = 2
)

var searchTokenPattern = regexp.MustCompile(`[a-z0-9][a-z0-9._+-]*`)

// QueryError is a stable transport-safe service failure.
type QueryError struct {
	Code    string
	Summary string
	Status  int
}

func (e *QueryError) Error() string { return e.Code + ": " + e.Summary }

func usageError(code, summary string) error {
	return &QueryError{Code: code, Summary: summary, Status: 400}
}
func missingError() error {
	return &QueryError{Code: "not_found", Summary: "The requested entity was not found.", Status: 404}
}

// Service provides bounded deterministic reads over one immutable projection.
type Service struct {
	dataset  explorercontract.Dataset
	byID     map[string]explorercontract.Entity
	incoming map[string][]explorercontract.Edge
	outgoing map[string][]explorercontract.Edge
	docs     []searchDocument
	avgLen   float64
}

type searchDocument struct {
	entity explorercontract.Entity
	terms  map[string]int
	length int
}

// NewService builds read indexes from projected fields only.
func NewService(dataset explorercontract.Dataset) (*Service, error) {
	if !dataset.ProjectionMode.Valid() || dataset.SchemaVersion != explorercontract.SchemaVersion {
		return nil, fmt.Errorf("invalid Explorer dataset")
	}
	s := &Service{
		dataset: dataset, byID: make(map[string]explorercontract.Entity, len(dataset.Entities)),
		incoming: map[string][]explorercontract.Edge{}, outgoing: map[string][]explorercontract.Edge{},
	}
	var total int
	for _, entity := range dataset.Entities {
		if err := catalog.ValidateEntityID(entity.ID); err != nil {
			return nil, fmt.Errorf("invalid Explorer dataset")
		}
		if _, exists := s.byID[entity.ID]; exists {
			return nil, fmt.Errorf("invalid Explorer dataset")
		}
		s.byID[entity.ID] = entity
		terms := tokenCounts(searchableEntityText(entity))
		length := 0
		for _, count := range terms {
			length += count
		}
		total += length
		s.docs = append(s.docs, searchDocument{entity: entity, terms: terms, length: length})
	}
	for _, edge := range dataset.Edges {
		if _, ok := s.byID[edge.Source]; !ok {
			return nil, fmt.Errorf("invalid Explorer dataset")
		}
		if _, ok := s.byID[edge.Target]; !ok {
			return nil, fmt.Errorf("invalid Explorer dataset")
		}
		s.outgoing[edge.Source] = append(s.outgoing[edge.Source], edge)
		s.incoming[edge.Target] = append(s.incoming[edge.Target], edge)
	}
	if len(s.docs) > 0 {
		s.avgLen = float64(total) / float64(len(s.docs))
	}
	if s.avgLen == 0 {
		s.avgLen = 1
	}
	return s, nil
}

// Dataset returns the immutable projection that backs this service.
func (s *Service) Dataset() explorercontract.Dataset { return s.dataset }

// Status returns bounded build and corpus counts for the current projection.
func (s *Service) Status(productVersion string) explorercontract.Status {
	return explorercontract.Status{
		ProductVersion: productVersion, APISchema: explorercontract.SchemaVersion,
		EntityCount: len(s.dataset.Entities), EdgeCount: len(s.dataset.Edges), FindingCount: len(s.dataset.Findings),
	}
}

// ListOptions defines bounded filters, ordering, and pagination for List.
type ListOptions struct {
	Query                   string
	Kinds, Statuses         []string
	Surfaces, Tags          []string
	Limit                   int
	Cursor, Sort, Direction string
}

// List returns one deterministic page of projected entities.
func (s *Service) List(options ListOptions) (explorercontract.EntityPage, explorercontract.Meta, error) {
	if len(options.Query) > 256 {
		return explorercontract.EntityPage{}, explorercontract.Meta{}, usageError("query_too_long", "The query must be 256 bytes or less.")
	}
	limit := options.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 {
		return explorercontract.EntityPage{}, explorercontract.Meta{}, usageError("invalid_limit", "The entity limit must be from 1 to 100.")
	}
	sortKey := options.Sort
	if sortKey == "" {
		sortKey = "name"
	}
	if sortKey != "name" && sortKey != "kind" && sortKey != "status" && sortKey != "id" {
		return explorercontract.EntityPage{}, explorercontract.Meta{}, usageError("invalid_sort", "The sort value is not supported.")
	}
	direction := options.Direction
	if direction == "" {
		direction = "asc"
	}
	if direction != "asc" && direction != "desc" {
		return explorercontract.EntityPage{}, explorercontract.Meta{}, usageError("invalid_direction", "The direction must be asc or desc.")
	}
	items := make([]explorercontract.Entity, 0, len(s.dataset.Entities))
	scores := map[string]float64{}
	if strings.TrimSpace(options.Query) != "" {
		for _, hit := range s.score(options.Query) {
			scores[hit.Entity.ID] = hit.Score
		}
	}
	for _, entity := range s.dataset.Entities {
		if len(scores) > 0 {
			if _, ok := scores[entity.ID]; !ok {
				continue
			}
		} else if strings.TrimSpace(options.Query) != "" {
			continue
		}
		if !matchesFold(entity.Kind, options.Kinds) || !matchesFold(entity.Status, options.Statuses) ||
			!matchesFold(entity.Surface, options.Surfaces) || !matchesAny(entity.Tags, options.Tags) {
			continue
		}
		items = append(items, entity)
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := sortValue(items[i], sortKey), sortValue(items[j], sortKey)
		less := left < right || (left == right && items[i].ID < items[j].ID)
		if direction == "desc" {
			return left > right || (left == right && items[i].ID > items[j].ID)
		}
		return less
	})
	offset, err := s.decodeCursor(options.Cursor)
	if err != nil || offset > len(items) {
		return explorercontract.EntityPage{}, explorercontract.Meta{}, usageError("invalid_cursor", "The cursor is invalid for this catalog version.")
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	pageItems := append([]explorercontract.Entity(nil), items[offset:end]...)
	meta := explorercontract.Meta{Total: len(items), Truncated: end < len(items)}
	if meta.Truncated {
		meta.NextCursor = s.encodeCursor(end)
	}
	return explorercontract.EntityPage{Items: pageItems}, meta, nil
}

// SearchOptions defines bounded filters and pagination for Search.
type SearchOptions struct {
	Query           string
	Kinds, Statuses []string
	Surfaces, Tags  []string
	Limit           int
}

// Search returns one deterministic BM25-ranked page from projected fields.
func (s *Service) Search(options SearchOptions) (explorercontract.SearchPage, explorercontract.Meta, error) {
	query := strings.TrimSpace(options.Query)
	if query == "" {
		return explorercontract.SearchPage{}, explorercontract.Meta{}, usageError("empty_query", "A search query is required.")
	}
	if len(query) > 256 {
		return explorercontract.SearchPage{}, explorercontract.Meta{}, usageError("query_too_long", "The query must be 256 bytes or less.")
	}
	limit := options.Limit
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 50 {
		return explorercontract.SearchPage{}, explorercontract.Meta{}, usageError("invalid_limit", "The search limit must be from 1 to 50.")
	}
	hits := s.score(query)
	filtered := make([]explorercontract.SearchHit, 0, len(hits))
	for _, hit := range hits {
		entity := hit.Entity
		if !matchesFold(entity.Kind, options.Kinds) || !matchesFold(entity.Status, options.Statuses) ||
			!matchesFold(entity.Surface, options.Surfaces) || !matchesAny(entity.Tags, options.Tags) {
			continue
		}
		filtered = append(filtered, hit)
	}
	meta := explorercontract.Meta{Total: len(filtered), Truncated: len(filtered) > limit}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}
	return explorercontract.SearchPage{Items: filtered}, meta, nil
}

func (s *Service) score(query string) []explorercontract.SearchHit {
	terms := uniqueTokens(query)
	if len(terms) == 0 {
		return []explorercontract.SearchHit{}
	}
	df := map[string]int{}
	for _, doc := range s.docs {
		for _, term := range terms {
			if doc.terms[term] > 0 {
				df[term]++
			}
		}
	}
	const k1, b = 1.2, 0.75
	hits := make([]explorercontract.SearchHit, 0)
	for _, doc := range s.docs {
		var score float64
		matched := make([]string, 0, len(terms))
		for _, term := range terms {
			tf := doc.terms[term]
			if tf == 0 {
				continue
			}
			idf := math.Log(1 + (float64(len(s.docs)-df[term])+0.5)/(float64(df[term])+0.5))
			denominator := float64(tf) + k1*(1-b+b*float64(doc.length)/s.avgLen)
			score += idf * (float64(tf) * (k1 + 1)) / denominator
			matched = append(matched, term)
		}
		if score == 0 {
			continue
		}
		lowerID, lowerName := strings.ToLower(doc.entity.ID), strings.ToLower(doc.entity.Name)
		for _, term := range terms {
			switch {
			case lowerID == term || lowerName == term:
				score += 4
			case strings.Contains(lowerID, term) || strings.Contains(lowerName, term):
				score += 1.5
			}
		}
		hits = append(hits, explorercontract.SearchHit{Entity: doc.entity, Score: score, MatchedTerms: matched})
	}
	sort.Slice(hits, func(i, j int) bool {
		if math.Abs(hits[i].Score-hits[j].Score) < 1e-9 {
			return hits[i].Entity.ID < hits[j].Entity.ID
		}
		return hits[i].Score > hits[j].Score
	})
	return hits
}

// EntityDetail returns one entity and its bounded direct relationships.
func (s *Service) EntityDetail(id string, edgeLimit int) (explorercontract.EntityDetail, explorercontract.Meta, error) {
	if err := catalog.ValidateEntityID(id); err != nil {
		return explorercontract.EntityDetail{}, explorercontract.Meta{}, usageError("invalid_entity_id", "The entity ID is invalid.")
	}
	entity, ok := s.byID[id]
	if !ok {
		return explorercontract.EntityDetail{}, explorercontract.Meta{}, missingError()
	}
	if edgeLimit <= 0 {
		edgeLimit = 200
	}
	incoming := append([]explorercontract.Edge(nil), s.incoming[id]...)
	outgoing := append([]explorercontract.Edge(nil), s.outgoing[id]...)
	total := len(incoming) + len(outgoing)
	truncated := false
	if total > edgeLimit {
		truncated = true
		if len(outgoing) >= edgeLimit {
			outgoing = outgoing[:edgeLimit]
			incoming = []explorercontract.Edge{}
		} else {
			incoming = incoming[:edgeLimit-len(outgoing)]
		}
	}
	return explorercontract.EntityDetail{Entity: entity, Incoming: incoming, Outgoing: outgoing}, explorercontract.Meta{Total: total, Truncated: truncated}, nil
}

// GraphOptions defines one bounded aggregate, region, or neighborhood graph.
type GraphOptions struct {
	Mode               explorercontract.GraphMode
	GroupBy            explorercontract.GraphGroup
	Group              string
	Entity             string
	Depth              int
	MaxNodes, MaxEdges int
}

// Graph returns a deterministic progressive graph at the requested level.
func (s *Service) Graph(options GraphOptions) (explorercontract.GraphPage, explorercontract.Meta, error) {
	if options.MaxNodes == 0 {
		options.MaxNodes = MaxGraphNodes
	}
	if options.MaxEdges == 0 {
		options.MaxEdges = MaxGraphEdges
	}
	if options.MaxNodes < 1 || options.MaxNodes > MaxGraphNodes || options.MaxEdges < 1 || options.MaxEdges > MaxGraphEdges {
		return explorercontract.GraphPage{}, explorercontract.Meta{}, usageError("graph_limit_exceeded", "The graph request exceeds the supported limits.")
	}
	if options.Mode == "" {
		options.Mode = explorercontract.GraphAggregate
	}
	if options.GroupBy == "" {
		options.GroupBy = explorercontract.GroupKind
	}
	if options.GroupBy != explorercontract.GroupKind && options.GroupBy != explorercontract.GroupSurface {
		return explorercontract.GraphPage{}, explorercontract.Meta{}, usageError("invalid_group", "The graph group is not supported.")
	}
	switch options.Mode {
	case explorercontract.GraphAggregate:
		page := constrainGraph(s.aggregateGraph(options.GroupBy), options, "Choose a narrower aggregate dimension before rendering this graph.")
		meta := explorercontract.Meta{Total: len(s.dataset.Entities), Truncated: false}
		if page.Refinement != nil {
			meta.Notice = "refinement_required"
		}
		return page, meta, nil
	case explorercontract.GraphRegion:
		return s.regionGraph(options)
	case explorercontract.GraphNeighborhood:
		return s.neighborhoodGraph(options)
	default:
		return explorercontract.GraphPage{}, explorercontract.Meta{}, usageError("invalid_graph_mode", "The graph mode is not supported.")
	}
}

func (s *Service) aggregateGraph(groupBy explorercontract.GraphGroup) explorercontract.GraphPage {
	groups := map[string]int{}
	entityGroup := map[string]string{}
	for _, entity := range s.dataset.Entities {
		group := graphGroup(entity, groupBy)
		groups[group]++
		entityGroup[entity.ID] = group
	}
	groupNames := make([]string, 0, len(groups))
	for group := range groups {
		groupNames = append(groupNames, group)
	}
	sort.Strings(groupNames)
	nodes := make([]explorercontract.GraphNode, 0, len(groupNames))
	for _, group := range groupNames {
		nodes = append(nodes, explorercontract.GraphNode{
			ID: aggregateID(group), Name: group, Group: group, Count: groups[group], Aggregate: true,
		})
	}
	type edgeKey struct{ source, target, kind string }
	counts := map[edgeKey]int{}
	for _, edge := range s.dataset.Edges {
		source, target := entityGroup[edge.Source], entityGroup[edge.Target]
		counts[edgeKey{source: source, target: target, kind: edge.Kind}]++
	}
	keys := make([]edgeKey, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].source != keys[j].source {
			return keys[i].source < keys[j].source
		}
		if keys[i].target != keys[j].target {
			return keys[i].target < keys[j].target
		}
		return keys[i].kind < keys[j].kind
	})
	edges := make([]explorercontract.GraphEdge, 0, len(keys))
	for _, key := range keys {
		edges = append(edges, explorercontract.GraphEdge{
			Source: aggregateID(key.source), Target: aggregateID(key.target), Kind: key.kind, Count: counts[key],
		})
	}
	return explorercontract.GraphPage{Mode: explorercontract.GraphAggregate, GroupBy: groupBy, Nodes: nodes, Edges: edges}
}

func (s *Service) regionGraph(options GraphOptions) (explorercontract.GraphPage, explorercontract.Meta, error) {
	if strings.TrimSpace(options.Group) == "" || len(options.Group) > 128 {
		return explorercontract.GraphPage{}, explorercontract.Meta{}, usageError("invalid_region", "A bounded graph region is required.")
	}
	ids := map[string]struct{}{}
	entities := make([]explorercontract.Entity, 0)
	for _, entity := range s.dataset.Entities {
		if graphGroup(entity, options.GroupBy) == options.Group {
			ids[entity.ID] = struct{}{}
			entities = append(entities, entity)
		}
	}
	if len(entities) == 0 {
		return explorercontract.GraphPage{}, explorercontract.Meta{}, missingError()
	}
	if len(entities) > options.MaxNodes {
		page := s.aggregateGraph(options.GroupBy)
		page.Mode, page.Scope = explorercontract.GraphRegion, options.Group
		page.Refinement = &explorercontract.Refinement{Code: "refinement_required", Summary: "Choose a smaller region before rendering entity nodes."}
		page = constrainGraph(page, options, page.Refinement.Summary)
		return page, explorercontract.Meta{Total: len(entities), Truncated: false, Notice: "refinement_required"}, nil
	}
	nodes := entityNodes(entities)
	edges := make([]explorercontract.GraphEdge, 0)
	for _, edge := range s.dataset.Edges {
		_, source := ids[edge.Source]
		_, target := ids[edge.Target]
		if source && target {
			edges = append(edges, explorercontract.GraphEdge{Source: edge.Source, Target: edge.Target, Kind: edge.Kind})
		}
	}
	if len(edges) > options.MaxEdges {
		page := s.aggregateGraph(options.GroupBy)
		page.Mode, page.Scope = explorercontract.GraphRegion, options.Group
		page.Refinement = &explorercontract.Refinement{Code: "refinement_required", Summary: "Choose a smaller region before rendering relationships."}
		page = constrainGraph(page, options, page.Refinement.Summary)
		return page, explorercontract.Meta{Total: len(entities), Truncated: false, Notice: "refinement_required"}, nil
	}
	return explorercontract.GraphPage{Mode: explorercontract.GraphRegion, GroupBy: options.GroupBy, Scope: options.Group, Nodes: nodes, Edges: edges}, explorercontract.Meta{Total: len(nodes), Truncated: false}, nil
}

func (s *Service) neighborhoodGraph(options GraphOptions) (explorercontract.GraphPage, explorercontract.Meta, error) {
	if err := catalog.ValidateEntityID(options.Entity); err != nil {
		return explorercontract.GraphPage{}, explorercontract.Meta{}, usageError("invalid_entity_id", "The entity ID is invalid.")
	}
	if _, ok := s.byID[options.Entity]; !ok {
		return explorercontract.GraphPage{}, explorercontract.Meta{}, missingError()
	}
	depth := options.Depth
	if depth == 0 {
		depth = 1
	}
	if depth < 1 || depth > MaxGraphDepth {
		return explorercontract.GraphPage{}, explorercontract.Meta{}, usageError("invalid_depth", "Graph depth must be one or two.")
	}
	seen := map[string]struct{}{options.Entity: {}}
	frontier := []string{options.Entity}
	for level := 0; level < depth; level++ {
		next := make([]string, 0)
		for _, id := range frontier {
			for _, edge := range append(append([]explorercontract.Edge(nil), s.outgoing[id]...), s.incoming[id]...) {
				for _, candidate := range []string{edge.Source, edge.Target} {
					if _, ok := seen[candidate]; ok {
						continue
					}
					seen[candidate] = struct{}{}
					next = append(next, candidate)
				}
			}
		}
		frontier = next
	}
	if len(seen) > options.MaxNodes {
		page := s.aggregateGraph(explorercontract.GroupKind)
		page.Mode, page.Scope, page.Depth = explorercontract.GraphNeighborhood, options.Entity, depth
		page.Refinement = &explorercontract.Refinement{Code: "refinement_required", Summary: "Use a smaller depth or a narrower region before rendering this neighborhood."}
		page = constrainGraph(page, options, page.Refinement.Summary)
		return page, explorercontract.Meta{Total: len(seen), Truncated: false, Notice: "refinement_required"}, nil
	}
	entities := make([]explorercontract.Entity, 0, len(seen))
	for id := range seen {
		entities = append(entities, s.byID[id])
	}
	sort.Slice(entities, func(i, j int) bool { return entities[i].ID < entities[j].ID })
	edges := make([]explorercontract.GraphEdge, 0)
	for _, edge := range s.dataset.Edges {
		_, source := seen[edge.Source]
		_, target := seen[edge.Target]
		if source && target {
			edges = append(edges, explorercontract.GraphEdge{Source: edge.Source, Target: edge.Target, Kind: edge.Kind})
		}
	}
	if len(edges) > options.MaxEdges {
		page := s.aggregateGraph(explorercontract.GroupKind)
		page.Mode, page.Scope, page.Depth = explorercontract.GraphNeighborhood, options.Entity, depth
		page.Refinement = &explorercontract.Refinement{Code: "refinement_required", Summary: "Use a smaller depth before rendering this neighborhood."}
		page = constrainGraph(page, options, page.Refinement.Summary)
		return page, explorercontract.Meta{Total: len(seen), Truncated: false, Notice: "refinement_required"}, nil
	}
	return explorercontract.GraphPage{Mode: explorercontract.GraphNeighborhood, Scope: options.Entity, Depth: depth, Nodes: entityNodes(entities), Edges: edges}, explorercontract.Meta{Total: len(entities), Truncated: false}, nil
}

func constrainGraph(page explorercontract.GraphPage, options GraphOptions, summary string) explorercontract.GraphPage {
	if len(page.Nodes) <= options.MaxNodes && len(page.Edges) <= options.MaxEdges {
		return page
	}
	page.Nodes = []explorercontract.GraphNode{}
	page.Edges = []explorercontract.GraphEdge{}
	page.Refinement = &explorercontract.Refinement{Code: "refinement_required", Summary: summary}
	return page
}

// Health returns bounded redacted findings for the immutable projection.
func (s *Service) Health(severity explorercontract.HealthSeverity, limit int) (explorercontract.HealthReport, explorercontract.Meta, error) {
	if severity != "" && severity != explorercontract.HealthError && severity != explorercontract.HealthWarning && severity != explorercontract.HealthInfo {
		return explorercontract.HealthReport{}, explorercontract.Meta{}, usageError("invalid_severity", "The health severity is not supported.")
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 100 {
		return explorercontract.HealthReport{}, explorercontract.Meta{}, usageError("invalid_limit", "The health limit must be from 1 to 100.")
	}
	findings := make([]explorercontract.HealthFinding, 0)
	for _, finding := range s.dataset.Findings {
		if severity == "" || finding.Severity == severity {
			findings = append(findings, finding)
		}
	}
	meta := explorercontract.Meta{Total: len(findings), Truncated: len(findings) > limit}
	if len(findings) > limit {
		findings = findings[:limit]
	}
	return explorercontract.HealthReport{OK: len(findings) == 0, Drift: "clean", Findings: findings}, meta, nil
}

func searchableEntityText(entity explorercontract.Entity) string {
	parts := []string{entity.ID, entity.Name, entity.Kind, entity.Status, entity.Summary, entity.Surface, entity.OwnerLabel, entity.EntrypointLabel}
	parts = append(parts, entity.Tags...)
	return strings.Join(parts, " ")
}

func tokenCounts(value string) map[string]int {
	counts := map[string]int{}
	for _, term := range searchTokenPattern.FindAllString(strings.ToLower(value), -1) {
		counts[term]++
	}
	return counts
}

func uniqueTokens(value string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, term := range searchTokenPattern.FindAllString(strings.ToLower(value), -1) {
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		out = append(out, term)
	}
	sort.Strings(out)
	return out
}

func matchesFold(value string, expected []string) bool {
	if len(expected) == 0 {
		return true
	}
	for _, candidate := range expected {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

func matchesAny(values, expected []string) bool {
	if len(expected) == 0 {
		return true
	}
	for _, value := range values {
		for _, candidate := range expected {
			if strings.EqualFold(value, strings.TrimSpace(candidate)) {
				return true
			}
		}
	}
	return false
}

func sortValue(entity explorercontract.Entity, key string) string {
	switch key {
	case "kind":
		return strings.ToLower(entity.Kind)
	case "status":
		return strings.ToLower(entity.Status)
	case "id":
		return entity.ID
	default:
		return strings.ToLower(entity.Name)
	}
}

type cursor struct {
	Digest string `json:"d"`
	Offset int    `json:"o"`
}

func (s *Service) encodeCursor(offset int) string {
	payload, _ := json.Marshal(cursor{Digest: s.dataset.SourceDigest, Offset: offset})
	return base64.RawURLEncoding.EncodeToString(payload)
}
func (s *Service) decodeCursor(raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(payload) > 512 {
		return 0, errors.New("invalid cursor")
	}
	var value cursor
	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.Digest != s.dataset.SourceDigest || value.Offset < 0 {
		return 0, errors.New("invalid cursor")
	}
	return value.Offset, nil
}

func graphGroup(entity explorercontract.Entity, by explorercontract.GraphGroup) string {
	value := entity.Kind
	if by == explorercontract.GroupSurface {
		value = entity.Surface
	}
	if strings.TrimSpace(value) == "" {
		return "Unspecified"
	}
	return value
}
func aggregateID(group string) string {
	return "group:" + base64.RawURLEncoding.EncodeToString([]byte(group))
}
func entityNodes(entities []explorercontract.Entity) []explorercontract.GraphNode {
	nodes := make([]explorercontract.GraphNode, 0, len(entities))
	for _, entity := range entities {
		nodes = append(nodes, explorercontract.GraphNode{ID: entity.ID, Name: entity.Name, Kind: entity.Kind, Status: entity.Status})
	}
	return nodes
}

func parseInt(raw string, fallback int) (int, error) {
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, usageError("invalid_number", "A numeric query value is invalid.")
	}
	return value, nil
}
