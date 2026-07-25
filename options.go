package catalog

import (
	"fmt"
	"log/slog"
)

// Limits bounds the work an engine will accept. Every zero value means
// "unlimited" for that dimension, except MaxSummaryBytes, which falls back to
// the projection default.
type Limits struct {
	// MaxEntities caps the total number of records Discover will return.
	MaxEntities int
	// MaxRecordsPerProvider caps the records a single provider may contribute.
	MaxRecordsPerProvider int
	// MaxSourceBytes caps the size of an individual corpus file.
	MaxSourceBytes int64
	// MaxSummaryBytes is the default projection summary bound when a
	// ProjectionPolicy does not set its own.
	MaxSummaryBytes int
	// MaxSearchResults caps the result count a single Search may return.
	MaxSearchResults int
	_                struct{}
}

// DefaultLimits returns the unbounded configuration the engine uses when a host
// declares none.
func DefaultLimits() Limits { return Limits{} }

// Validate rejects negative bounds.
func (l Limits) Validate() error {
	for name, value := range map[string]int{
		"max_entities":             l.MaxEntities,
		"max_records_per_provider": l.MaxRecordsPerProvider,
		"max_summary_bytes":        l.MaxSummaryBytes,
		"max_search_results":       l.MaxSearchResults,
	} {
		if value < 0 {
			return fmt.Errorf("%w: %s must not be negative", ErrInvalidLayout, name)
		}
	}
	if l.MaxSourceBytes < 0 {
		return fmt.Errorf("%w: max_source_bytes must not be negative", ErrInvalidLayout)
	}
	return nil
}

// engineConfig is the resolved construction state behind the Option list.
type engineConfig struct {
	providers []Provider
	logger    *slog.Logger
	limits    Limits
}

// Option configures an Engine at construction time. Options exist so the
// constructor can grow without another breaking signature change.
type Option func(*engineConfig) error

// WithProviders registers the providers the engine discovers from. Passing no
// providers leaves the engine with its default FilesystemProvider.
func WithProviders(providers ...Provider) Option {
	return func(config *engineConfig) error {
		config.providers = append(config.providers, providers...)
		return nil
	}
}

// WithLogger attaches a structured logger.
//
// The engine logs only counts, durations, provider names, entity ids, and
// corpus-relative paths. It never logs entity descriptions, annotations, public
// URLs, owners, or entrypoints, so a host can route this logger anywhere its
// operational logs already go.
func WithLogger(logger *slog.Logger) Option {
	return func(config *engineConfig) error {
		config.logger = logger
		return nil
	}
}

// WithLimits bounds engine work.
func WithLimits(limits Limits) Option {
	return func(config *engineConfig) error {
		if err := limits.Validate(); err != nil {
			return err
		}
		config.limits = limits
		return nil
	}
}
