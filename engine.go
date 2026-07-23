package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Engine is a host-bound catalog compiler. It is safe to construct more than
// one engine with different Layouts in the same process.
type Engine struct {
	layout    Layout
	providers []Provider
}

func New(layout Layout, providers ...Provider) (*Engine, error) {
	if err := layout.Validate(); err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		providers = []Provider{FilesystemProvider{}}
	}
	seen := map[string]struct{}{}
	for _, provider := range providers {
		if provider == nil {
			return nil, fmt.Errorf("provider must not be nil")
		}
		name := strings.TrimSpace(provider.Name())
		if name == "" {
			return nil, fmt.Errorf("provider name is required")
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("duplicate provider name %q", name)
		}
		seen[name] = struct{}{}
	}
	return &Engine{layout: layout, providers: append([]Provider(nil), providers...)}, nil
}

func (e *Engine) Layout() Layout { return e.layout }

type ValidationReport struct {
	OK            bool     `json:"ok"`
	EntityCount   int      `json:"entity_count"`
	ProviderCount int      `json:"provider_count"`
	Warnings      []string `json:"warnings,omitempty"`
}

// Discover collects, normalizes, validates, and deterministically orders all
// provider records. Duplicate IDs fail closed even across providers.
func (e *Engine) Discover(ctx context.Context) ([]Record, error) {
	var records []Record
	for _, provider := range e.providers {
		provided, err := provider.Provide(ctx, e.layout)
		if err != nil {
			return nil, fmt.Errorf("provider %s: %w", provider.Name(), err)
		}
		for _, record := range provided {
			record.Provider = strings.TrimSpace(record.Provider)
			if record.Provider == "" {
				record.Provider = provider.Name()
			}
			record.Entity = normalizeEntity(record.Entity)
			if err := validateEntity(record.Entity); err != nil {
				return nil, fmt.Errorf("provider %s source %s: %w", provider.Name(), record.Source, err)
			}
			records = append(records, record)
		}
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].Entity.ID == records[j].Entity.ID {
			if records[i].Provider == records[j].Provider {
				return records[i].Source < records[j].Source
			}
			return records[i].Provider < records[j].Provider
		}
		return records[i].Entity.ID < records[j].Entity.ID
	})
	for i := 1; i < len(records); i++ {
		if records[i-1].Entity.ID == records[i].Entity.ID {
			return nil, fmt.Errorf(
				"duplicate entity id %q from %s:%s and %s:%s",
				records[i].Entity.ID,
				records[i-1].Provider, records[i-1].Source,
				records[i].Provider, records[i].Source,
			)
		}
	}
	return records, nil
}

func (e *Engine) Validate(ctx context.Context) (ValidationReport, error) {
	records, err := e.Discover(ctx)
	if err != nil {
		return ValidationReport{}, err
	}
	ids := make(map[string]struct{}, len(records))
	for _, record := range records {
		ids[record.Entity.ID] = struct{}{}
	}
	var warnings []string
	for _, record := range records {
		for _, ref := range record.Entity.Refs {
			if _, ok := ids[ref.Target]; !ok {
				warnings = append(warnings, fmt.Sprintf("entity %s reference %s:%s has no local target", record.Entity.ID, ref.Kind, ref.Target))
			}
		}
	}
	sort.Strings(warnings)
	return ValidationReport{OK: true, EntityCount: len(records), ProviderCount: len(e.providers), Warnings: warnings}, nil
}

func recordsDigest(records []Record) (string, error) {
	canonical := make([]struct {
		Entity   Entity `json:"entity"`
		Provider string `json:"provider"`
		Source   string `json:"source"`
		Digest   string `json:"digest"`
	}, 0, len(records))
	for _, record := range records {
		canonical = append(canonical, struct {
			Entity   Entity `json:"entity"`
			Provider string `json:"provider"`
			Source   string `json:"source"`
			Digest   string `json:"digest"`
		}{record.Entity, record.Provider, record.Source, record.Digest})
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
