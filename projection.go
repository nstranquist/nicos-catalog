package catalog

import (
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var prohibitedPublicContent = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:^|[\s"'(])(?:/users/|/home/|file://|~/|[a-z]:\\)`),
	regexp.MustCompile(`(?i)\b(?:` + "private-admin-" + `evidence|sources/(?:originals|extracted-text)|\.jobkit|\.ssh)\b`),
	regexp.MustCompile(`(?i)\b(?:api[_-]?key|access[_-]?token|secret|password)\s*[:=]\s*\S+`),
	regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|sk-[A-Za-z0-9_-]{20,})\b`),
}

// PublicEntity is a closed publication DTO. It cannot represent source paths,
// host annotations, owner telemetry, sidecars, valuation, or query text.
type PublicEntity struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Kind        string             `json:"kind"`
	Status      string             `json:"status,omitempty"`
	Summary     string             `json:"summary,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	URL         string             `json:"url,omitempty"`
	Connections []PublicConnection `json:"connections,omitempty"`
}

type PublicConnection struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
}

type PublicProjection struct {
	SchemaVersion int            `json:"schema_version"`
	Items         []PublicEntity `json:"items"`
}

type ProjectionPolicy struct {
	RequireVisibility string
	IncludeKinds      []string
	IncludeTags       []string
	AllowHosts        []string
	MaxSummaryBytes   int
}

func ProjectPublic(index Index, policy ProjectionPolicy) (PublicProjection, error) {
	kinds := stringSet(policy.IncludeKinds)
	tags := stringSet(policy.IncludeTags)
	hosts := hostSet(policy.AllowHosts)
	maxSummary := policy.MaxSummaryBytes
	if maxSummary <= 0 {
		maxSummary = 320
	}
	requireVisibility := strings.TrimSpace(policy.RequireVisibility)
	if requireVisibility == "" {
		requireVisibility = "public"
	}
	if requireVisibility != "public" {
		return PublicProjection{}, fmt.Errorf("public projection requires visibility %q, got %q", "public", requireVisibility)
	}
	projection := PublicProjection{SchemaVersion: SchemaVersion}
	included := map[string]struct{}{}
	for _, entity := range index.Entities {
		if entity.Visibility != requireVisibility {
			continue
		}
		if len(kinds) > 0 {
			if _, ok := kinds[entity.Kind]; !ok {
				continue
			}
		}
		if len(tags) > 0 && !hasAny(entity.Tags, tags) {
			continue
		}
		if err := validatePublicURL(entity.PublicURL, hosts); err != nil {
			return PublicProjection{}, fmt.Errorf("entity %s: %w", entity.ID, err)
		}
		if err := validatePublicEntityContent(entity); err != nil {
			return PublicProjection{}, fmt.Errorf("entity %s: %w", entity.ID, err)
		}
		summary := strings.TrimSpace(entity.Description)
		if len(summary) > maxSummary {
			summary = truncateUTF8(summary, maxSummary) + "…"
		}
		projection.Items = append(projection.Items, PublicEntity{
			ID: entity.ID, Name: entity.Name, Kind: entity.Kind, Status: entity.Status,
			Summary: summary, Tags: append([]string(nil), entity.Tags...), URL: entity.PublicURL,
		})
		included[entity.ID] = struct{}{}
	}
	for i := range projection.Items {
		entity := findEntity(index.Entities, projection.Items[i].ID)
		for _, ref := range entity.Refs {
			if _, ok := included[ref.Target]; ok {
				projection.Items[i].Connections = append(projection.Items[i].Connections, PublicConnection{Kind: ref.Kind, Target: ref.Target})
			}
		}
	}
	sort.Slice(projection.Items, func(i, j int) bool { return projection.Items[i].ID < projection.Items[j].ID })
	return projection, nil
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return strings.TrimSpace(value)
	}
	end := maxBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return strings.TrimSpace(value[:end])
}

func validatePublicURL(raw string, allowed map[string]struct{}) error {
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.Opaque != "" {
		return fmt.Errorf("public_url must be an absolute https URL")
	}
	if parsed.User != nil {
		return fmt.Errorf("public_url must not contain credentials")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("public_url must not contain a query or fragment")
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return fmt.Errorf("public_url must not use non-HTTPS port %q", port)
	}
	if len(allowed) == 0 {
		return fmt.Errorf("public_url requires an explicit hostname allowlist")
	}
	if _, ok := allowed[strings.ToLower(parsed.Hostname())]; !ok {
		return fmt.Errorf("public_url host %q is not allowed", parsed.Hostname())
	}
	return nil
}

func validatePublicEntityContent(entity Entity) error {
	values := []struct {
		field string
		value string
	}{
		{"id", entity.ID}, {"name", entity.Name}, {"kind", entity.Kind},
		{"status", entity.Status}, {"summary", entity.Description},
	}
	for i, tag := range entity.Tags {
		values = append(values, struct {
			field string
			value string
		}{fmt.Sprintf("tags[%d]", i), tag})
	}
	for i, ref := range entity.Refs {
		values = append(values,
			struct {
				field string
				value string
			}{fmt.Sprintf("refs[%d].kind", i), ref.Kind},
			struct {
				field string
				value string
			}{fmt.Sprintf("refs[%d].target", i), ref.Target},
		)
	}
	for _, item := range values {
		if !utf8.ValidString(item.value) {
			return fmt.Errorf("public field %s is not valid UTF-8", item.field)
		}
		for _, pattern := range prohibitedPublicContent {
			if match := pattern.FindString(item.value); match != "" {
				return fmt.Errorf("public field %s contains prohibited content %q", item.field, match)
			}
		}
	}
	return nil
}

func stringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func hostSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func hasAny(values []string, expected map[string]struct{}) bool {
	for _, value := range values {
		if _, ok := expected[value]; ok {
			return true
		}
	}
	return false
}

func findEntity(entities []Entity, id string) Entity {
	for _, entity := range entities {
		if entity.ID == id {
			return entity
		}
	}
	return Entity{}
}
