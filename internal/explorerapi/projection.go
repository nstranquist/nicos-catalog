// Package explorerapi compiles closed Explorer projections and serves the
// bounded read-only query contract shared by HTTP, static export, and MCP.
package explorerapi

import (
	"context"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	catalog "github.com/nstranquist/nicos-catalog"
	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

const (
	maxSummaryBytes    = 320
	maxOwnerLabelBytes = 96
	maxEntryLabelBytes = 128
)

// Compile creates the one closed dataset consumed by every Explorer transport.
func Compile(ctx context.Context, index catalog.Index, mode explorercontract.ProjectionMode, policy catalog.ProjectionPolicy) (explorercontract.Dataset, error) {
	if err := ctx.Err(); err != nil {
		return explorercontract.Dataset{}, err
	}
	if !mode.Valid() {
		return explorercontract.Dataset{}, fmt.Errorf("invalid Explorer projection mode")
	}
	dataset := explorercontract.Dataset{
		SchemaVersion:  explorercontract.SchemaVersion,
		ProjectionMode: mode,
		SourceDigest:   index.SourceDigest,
		Entities:       make([]explorercontract.Entity, 0),
		Edges:          make([]explorercontract.Edge, 0),
		Findings:       make([]explorercontract.HealthFinding, 0),
	}

	switch mode {
	case explorercontract.ProjectionPublic:
		projection, err := catalog.ProjectPublic(ctx, index, policy)
		if err != nil {
			return explorercontract.Dataset{}, err
		}
		for _, item := range projection.Items {
			entity := explorercontract.Entity{
				ID: item.ID, Name: item.Name, Kind: item.Kind, Status: item.Status,
				Summary: item.Summary, Tags: cloneStrings(item.Tags), URL: item.URL,
			}
			if err := validateProjectedEntity(entity); err != nil {
				return explorercontract.Dataset{}, err
			}
			dataset.Entities = append(dataset.Entities, entity)
			for _, connection := range item.Connections {
				dataset.Edges = append(dataset.Edges, explorercontract.Edge{
					Source: item.ID, Target: connection.Target, Kind: connection.Kind,
				})
			}
		}
	case explorercontract.ProjectionLocal:
		included := make(map[string]struct{}, len(index.Entities))
		for _, item := range index.Entities {
			entity, err := localEntity(item)
			if err != nil {
				return explorercontract.Dataset{}, err
			}
			dataset.Entities = append(dataset.Entities, entity)
			included[item.ID] = struct{}{}
		}
		for _, item := range index.Entities {
			for _, ref := range item.Refs {
				if _, ok := included[ref.Target]; !ok {
					continue
				}
				if err := safeText("relationship", ref.Kind); err != nil {
					return explorercontract.Dataset{}, err
				}
				dataset.Edges = append(dataset.Edges, explorercontract.Edge{
					Source: item.ID, Target: ref.Target, Kind: ref.Kind,
				})
			}
		}
	}

	sort.Slice(dataset.Entities, func(i, j int) bool { return dataset.Entities[i].ID < dataset.Entities[j].ID })
	sort.Slice(dataset.Edges, func(i, j int) bool {
		if dataset.Edges[i].Source != dataset.Edges[j].Source {
			return dataset.Edges[i].Source < dataset.Edges[j].Source
		}
		if dataset.Edges[i].Kind != dataset.Edges[j].Kind {
			return dataset.Edges[i].Kind < dataset.Edges[j].Kind
		}
		return dataset.Edges[i].Target < dataset.Edges[j].Target
	})
	dataset.Findings = projectedFindings(dataset)
	if mode == explorercontract.ProjectionLocal {
		dataset.Findings = append(dataset.Findings, localDanglingFindings(index)...)
		sort.Slice(dataset.Findings, func(i, j int) bool {
			if dataset.Findings[i].Severity != dataset.Findings[j].Severity {
				return dataset.Findings[i].Severity < dataset.Findings[j].Severity
			}
			if dataset.Findings[i].Code != dataset.Findings[j].Code {
				return dataset.Findings[i].Code < dataset.Findings[j].Code
			}
			return dataset.Findings[i].EntityID < dataset.Findings[j].EntityID
		})
	}
	return dataset, nil
}

func localDanglingFindings(index catalog.Index) []explorercontract.HealthFinding {
	ids := make(map[string]struct{}, len(index.Entities))
	for _, entity := range index.Entities {
		ids[entity.ID] = struct{}{}
	}
	findings := make([]explorercontract.HealthFinding, 0)
	for _, entity := range index.Entities {
		for _, ref := range entity.Refs {
			if _, ok := ids[ref.Target]; ok {
				continue
			}
			findings = append(findings, explorercontract.HealthFinding{Code: "dangling_reference", Severity: explorercontract.HealthWarning, EntityID: entity.ID, Remediation: "Add the target entity or remove the relationship."})
		}
	}
	return findings
}

func localEntity(item catalog.Entity) (explorercontract.Entity, error) {
	entrypoint := boundedLabel(item.Entrypoint, maxEntryLabelBytes)
	if portableAbs(item.Entrypoint) {
		entrypoint = boundedLabel(portableBase(item.Entrypoint), maxEntryLabelBytes)
	} else {
		entrypoint = strings.TrimPrefix(filepath.ToSlash(entrypoint), "../")
	}
	entity := explorercontract.Entity{
		ID: item.ID, Name: item.Name, Kind: item.Kind, Status: item.Status,
		Summary: boundedLabel(item.Description, maxSummaryBytes), Surface: item.Surface,
		Tags: cloneStrings(item.Tags), URL: safeLocalURL(item.PublicURL),
		OwnerLabel: boundedLabel(item.Owner, maxOwnerLabelBytes), EntrypointLabel: entrypoint,
	}
	if err := validateProjectedEntity(entity); err != nil {
		return explorercontract.Entity{}, err
	}
	return entity, nil
}

func validateProjectedEntity(entity explorercontract.Entity) error {
	fields := []string{
		entity.ID, entity.Name, entity.Kind, entity.Status, entity.Summary,
		entity.Surface, entity.URL, entity.OwnerLabel, entity.EntrypointLabel,
	}
	fields = append(fields, entity.Tags...)
	for _, value := range fields {
		if err := safeText("Explorer field", value); err != nil {
			return err
		}
	}
	return nil
}

func safeText(field, value string) error {
	if err := catalog.ScanPublicText(field, value); err != nil {
		return fmt.Errorf("explorer projection rejected unsafe content")
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{".internal", ".localhost", ".localdomain", "private-host-canary"} {
		if strings.Contains(lower, marker) {
			return fmt.Errorf("explorer projection rejected unsafe content")
		}
	}
	return nil
}

func safeLocalURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	if err := safeText("public URL", raw); err != nil {
		return ""
	}
	return raw
}

func boundedLabel(value string, maxBytes int) string {
	value = strings.TrimSpace(strings.ToValidUTF8(value, ""))
	if len(value) <= maxBytes {
		return value
	}
	const suffix = "…"
	end := maxBytes - len(suffix)
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return strings.TrimSpace(value[:end]) + suffix
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func projectedFindings(dataset explorercontract.Dataset) []explorercontract.HealthFinding {
	ids := make(map[string]struct{}, len(dataset.Entities))
	for _, entity := range dataset.Entities {
		ids[entity.ID] = struct{}{}
	}
	seen := map[string]struct{}{}
	findings := make([]explorercontract.HealthFinding, 0)
	for _, edge := range dataset.Edges {
		key := edge.Source + "\x00" + edge.Kind + "\x00" + edge.Target
		if _, ok := seen[key]; ok {
			findings = append(findings, explorercontract.HealthFinding{
				Code: "duplicate_reference", Severity: explorercontract.HealthWarning,
				EntityID: edge.Source, Remediation: "Keep one copy of the repeated relationship.",
			})
		}
		seen[key] = struct{}{}
		if edge.Source == edge.Target {
			findings = append(findings, explorercontract.HealthFinding{
				Code: "self_reference", Severity: explorercontract.HealthWarning,
				EntityID: edge.Source, Remediation: "Remove the self-reference or target a different entity.",
			})
		}
		if _, ok := ids[edge.Target]; !ok {
			findings = append(findings, explorercontract.HealthFinding{
				Code: "dangling_reference", Severity: explorercontract.HealthWarning,
				EntityID: edge.Source, Remediation: "Add the target entity or remove the relationship.",
			})
		}
	}
	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Severity != findings[j].Severity {
			return findings[i].Severity < findings[j].Severity
		}
		if findings[i].Code != findings[j].Code {
			return findings[i].Code < findings[j].Code
		}
		return findings[i].EntityID < findings[j].EntityID
	})
	return findings
}

func portableAbs(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if filepath.IsAbs(path) || strings.HasPrefix(path, "/") || strings.HasPrefix(path, `\\`) {
		return true
	}
	return len(path) >= 3 && path[1] == ':' && (path[2] == '/' || path[2] == '\\')
}

func portableBase(path string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(path), `\`, "/")
	if i := strings.LastIndex(normalized, "/"); i >= 0 {
		return normalized[i+1:]
	}
	return normalized
}
