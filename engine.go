package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

// Engine is a host-bound catalog compiler. It is safe to construct more than
// one engine with different Layouts in the same process.
type Engine struct {
	layout    Layout
	providers []Provider
	logger    *slog.Logger
	limits    Limits
}

// New builds an engine for layout.
//
// With no WithProviders option the engine discovers through a default
// FilesystemProvider rooted at layout.CorpusDir. Provider names must be unique;
// duplicates fail closed rather than shadowing one another.
func New(layout Layout, opts ...Option) (*Engine, error) {
	if err := layout.Validate(); err != nil {
		return nil, err
	}
	config := engineConfig{limits: DefaultLimits()}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&config); err != nil {
			return nil, err
		}
	}
	providers := config.providers
	if len(providers) == 0 {
		providers = []Provider{FilesystemProvider{}}
	}
	seen := map[string]struct{}{}
	for _, provider := range providers {
		if provider == nil {
			return nil, fmt.Errorf("%w: provider must not be nil", ErrInvalidProvider)
		}
		name := strings.TrimSpace(provider.Name())
		if name == "" {
			return nil, fmt.Errorf("%w: provider name is required", ErrInvalidProvider)
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("%w: duplicate provider name %q", ErrInvalidProvider, name)
		}
		seen[name] = struct{}{}
	}
	logger := config.logger
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	return &Engine{
		layout:    layout,
		providers: append([]Provider(nil), providers...),
		logger:    logger,
		limits:    config.limits,
	}, nil
}

// Layout returns the host boundaries this engine was built with.
func (e *Engine) Layout() Layout { return e.layout }

// Severity classifies a validation issue.
type Severity string

// Issue severities.
const (
	// SeverityError fails a ValidationReport.
	SeverityError Severity = "error"
	// SeverityWarning is advisory and leaves a report OK.
	SeverityWarning Severity = "warning"
)

// ValidationIssueKind is the closed vocabulary of validation findings.
type ValidationIssueKind string

// Validation issue kinds.
const (
	// IssueDanglingReference is a reference whose target is not in the corpus.
	IssueDanglingReference ValidationIssueKind = "dangling_reference"
	// IssueSelfReference is an entity referencing itself.
	IssueSelfReference ValidationIssueKind = "self_reference"
	// IssueDuplicateReference is the same kind and target declared twice.
	IssueDuplicateReference ValidationIssueKind = "duplicate_reference"
)

// ValidationIssue is a single typed finding against the discovered corpus.
type ValidationIssue struct {
	EntityID string              `json:"entity_id"`
	Kind     ValidationIssueKind `json:"kind"`
	Severity Severity            `json:"severity"`
	Detail   string              `json:"detail"`
	_        struct{}
}

// ValidationReport summarizes a Validate run. OK is false whenever Errors is
// non-empty; warnings alone do not fail a report.
type ValidationReport struct {
	OK            bool              `json:"ok"`
	EntityCount   int               `json:"entity_count"`
	ProviderCount int               `json:"provider_count"`
	Warnings      []ValidationIssue `json:"warnings,omitempty"`
	Errors        []ValidationIssue `json:"errors,omitempty"`
	_             struct{}
}

// validateConfig is the resolved state behind the ValidateOption list.
type validateConfig struct {
	strictReferences bool
}

// ValidateOption tunes a Validate run.
type ValidateOption func(*validateConfig) error

// WithStrictReferences promotes dangling references from warnings to errors, so
// a corpus that points at entities it does not contain fails validation.
func WithStrictReferences() ValidateOption {
	return func(config *validateConfig) error {
		config.strictReferences = true
		return nil
	}
}

// Discover collects, normalizes, validates, and deterministically orders all
// provider records. Duplicate IDs fail closed even across providers.
func (e *Engine) Discover(ctx context.Context) ([]Record, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var records []Record
	for _, provider := range e.providers {
		// Checked per provider as well as inside providers, so a chain of
		// providers that never consult ctx is still cancellable between them.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		provided, err := provider.Provide(ctx, e.layout)
		if err != nil {
			return nil, &ProviderError{Provider: provider.Name(), Err: err}
		}
		if limit := e.limits.MaxRecordsPerProvider; limit > 0 && len(provided) > limit {
			return nil, &ProviderError{
				Provider: provider.Name(),
				Err:      fmt.Errorf("%w: %d records exceeds max_records_per_provider %d", ErrInvalidProvider, len(provided), limit),
			}
		}
		for _, record := range provided {
			record.Provider = strings.TrimSpace(record.Provider)
			if record.Provider == "" {
				record.Provider = provider.Name()
			}
			record.Entity = normalizeEntity(record.Entity)
			if err := validateEntity(record.Entity); err != nil {
				return nil, &EntityError{
					EntityID: record.Entity.ID, Provider: provider.Name(),
					Source: record.Source, Err: err,
				}
			}
			// The digest is computed here, after normalization, so it is a
			// property of the entity rather than of whichever file happened to
			// carry it. Providers that pack many entities into one payload
			// previously gave every one of them the same digest.
			digest, err := entityDigest(record.Entity)
			if err != nil {
				return nil, &EntityError{EntityID: record.Entity.ID, Provider: provider.Name(), Source: record.Source, Err: err}
			}
			record.Digest = digest
			records = append(records, record)
		}
	}
	if limit := e.limits.MaxEntities; limit > 0 && len(records) > limit {
		return nil, fmt.Errorf("%w: %d entities exceeds max_entities %d", ErrInvalidEntity, len(records), limit)
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
			return nil, &DuplicateIDError{
				EntityID: records[i].Entity.ID,
				First:    RecordOrigin{Provider: records[i-1].Provider, Source: records[i-1].Source},
				Second:   RecordOrigin{Provider: records[i].Provider, Source: records[i].Source},
			}
		}
	}
	e.logger.Debug("catalog discover", "records", len(records), "providers", len(e.providers))
	return records, nil
}

// Validate checks reference integrity across the discovered corpus.
func (e *Engine) Validate(ctx context.Context, opts ...ValidateOption) (ValidationReport, error) {
	config := validateConfig{}
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(&config); err != nil {
			return ValidationReport{}, err
		}
	}
	records, err := e.Discover(ctx)
	if err != nil {
		return ValidationReport{}, err
	}
	ids := make(map[string]struct{}, len(records))
	for _, record := range records {
		ids[record.Entity.ID] = struct{}{}
	}
	report := ValidationReport{EntityCount: len(records), ProviderCount: len(e.providers)}
	add := func(issue ValidationIssue) {
		if issue.Severity == SeverityError {
			report.Errors = append(report.Errors, issue)
			return
		}
		report.Warnings = append(report.Warnings, issue)
	}
	referenceSeverity := SeverityWarning
	if config.strictReferences {
		referenceSeverity = SeverityError
	}
	for _, record := range records {
		seenRefs := map[string]struct{}{}
		for _, ref := range record.Entity.Refs {
			key := ref.Kind + "\x00" + ref.Target
			if _, duplicate := seenRefs[key]; duplicate {
				add(ValidationIssue{
					EntityID: record.Entity.ID, Kind: IssueDuplicateReference, Severity: SeverityWarning,
					Detail: "reference " + ref.Kind + ":" + ref.Target + " is declared more than once",
				})
			}
			seenRefs[key] = struct{}{}
			if ref.Target == record.Entity.ID {
				add(ValidationIssue{
					EntityID: record.Entity.ID, Kind: IssueSelfReference, Severity: SeverityWarning,
					Detail: "reference " + ref.Kind + " targets its own entity",
				})
				continue
			}
			if _, ok := ids[ref.Target]; !ok {
				add(ValidationIssue{
					EntityID: record.Entity.ID, Kind: IssueDanglingReference, Severity: referenceSeverity,
					Detail: "reference " + ref.Kind + ":" + ref.Target + " has no local target",
				})
			}
		}
	}
	sortIssues(report.Warnings)
	sortIssues(report.Errors)
	report.OK = len(report.Errors) == 0
	return report, nil
}

func sortIssues(issues []ValidationIssue) {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].EntityID != issues[j].EntityID {
			return issues[i].EntityID < issues[j].EntityID
		}
		if issues[i].Kind != issues[j].Kind {
			return issues[i].Kind < issues[j].Kind
		}
		return issues[i].Detail < issues[j].Detail
	})
}

// entityDigest is the canonical per-entity content digest.
func entityDigest(entity Entity) (string, error) {
	payload, err := json.Marshal(entity)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func recordsDigest(records []Record) (string, error) {
	type canonicalRecord struct {
		Entity   Entity `json:"entity"`
		Provider string `json:"provider"`
		Source   string `json:"source"`
		Digest   string `json:"digest"`
	}
	canonical := make([]canonicalRecord, 0, len(records))
	for _, record := range records {
		canonical = append(canonical, canonicalRecord{record.Entity, record.Provider, record.Source, record.Digest})
	}
	payload, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
