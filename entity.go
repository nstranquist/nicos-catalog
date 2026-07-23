package catalog

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const SchemaVersion = 1

var entityIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// Entity is the portable catalog record. Host-only business, telemetry, and
// operator fields belong in host adapters rather than this public contract.
type Entity struct {
	ID          string            `json:"id" yaml:"id"`
	Name        string            `json:"name" yaml:"name"`
	Kind        string            `json:"kind" yaml:"kind"`
	Surface     string            `json:"surface,omitempty" yaml:"surface,omitempty"`
	Status      string            `json:"status,omitempty" yaml:"status,omitempty"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Entrypoint  string            `json:"entrypoint,omitempty" yaml:"entrypoint,omitempty"`
	Owner       string            `json:"owner,omitempty" yaml:"owner,omitempty"`
	Tags        []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	Refs        []Ref             `json:"refs,omitempty" yaml:"refs,omitempty"`
	PublicURL   string            `json:"public_url,omitempty" yaml:"public_url,omitempty"`
	Visibility  string            `json:"visibility,omitempty" yaml:"visibility,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
}

type Ref struct {
	Kind   string `json:"kind" yaml:"kind"`
	Target string `json:"target" yaml:"target"`
}

// Record binds a portable entity to source provenance. Source paths never
// appear in public projections.
type Record struct {
	Entity   Entity `json:"entity"`
	Provider string `json:"provider"`
	Source   string `json:"source"`
	Digest   string `json:"digest"`
}

func normalizeEntity(entity Entity) Entity {
	entity.ID = strings.TrimSpace(entity.ID)
	entity.Name = strings.TrimSpace(entity.Name)
	entity.Kind = strings.TrimSpace(entity.Kind)
	entity.Surface = strings.TrimSpace(entity.Surface)
	entity.Status = strings.TrimSpace(entity.Status)
	entity.Description = strings.TrimSpace(entity.Description)
	entity.Entrypoint = strings.TrimSpace(entity.Entrypoint)
	entity.Owner = strings.TrimSpace(entity.Owner)
	entity.PublicURL = strings.TrimSpace(entity.PublicURL)
	entity.Visibility = strings.TrimSpace(entity.Visibility)
	entity.Tags = normalizeStrings(entity.Tags)
	for i := range entity.Refs {
		entity.Refs[i].Kind = strings.TrimSpace(entity.Refs[i].Kind)
		entity.Refs[i].Target = strings.TrimSpace(entity.Refs[i].Target)
	}
	sort.Slice(entity.Refs, func(i, j int) bool {
		if entity.Refs[i].Kind == entity.Refs[j].Kind {
			return entity.Refs[i].Target < entity.Refs[j].Target
		}
		return entity.Refs[i].Kind < entity.Refs[j].Kind
	})
	return entity
}

func validateEntity(entity Entity) error {
	if !entityIDPattern.MatchString(entity.ID) {
		return fmt.Errorf("id %q must match %s", entity.ID, entityIDPattern)
	}
	if entity.Name == "" {
		return fmt.Errorf("entity %s: name is required", entity.ID)
	}
	if entity.Kind == "" {
		return fmt.Errorf("entity %s: kind is required", entity.ID)
	}
	for _, ref := range entity.Refs {
		if ref.Kind == "" || ref.Target == "" {
			return fmt.Errorf("entity %s: references require kind and target", entity.ID)
		}
	}
	return nil
}

func normalizeStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
