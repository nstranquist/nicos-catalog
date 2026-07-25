package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// DriftReport compares authored source against the derived index.
type DriftReport struct {
	OK             bool   `json:"ok"`
	Changed        bool   `json:"changed"`
	Reason         string `json:"reason,omitempty"`
	ExpectedDigest string `json:"expected_digest,omitempty"`
	ActualDigest   string `json:"actual_digest,omitempty"`
	_              struct{}
}

// Drift compares authored source against the derived index. A missing index or
// a schema advance is reported as drift to reconcile, not as an error.
func (e *Engine) Drift(ctx context.Context) (DriftReport, error) {
	records, err := e.Discover(ctx)
	if err != nil {
		return DriftReport{}, err
	}
	actual, err := recordsDigest(records)
	if err != nil {
		return DriftReport{}, err
	}
	index, err := e.LoadIndex(ctx)
	if err != nil {
		if errors.Is(err, ErrIndexMissing) {
			return DriftReport{Changed: true, Reason: "index_missing", ActualDigest: actual}, nil
		}
		// A schema advance is drift to be reconciled, not a hard failure.
		// Treating it as an error would turn every engine upgrade into a crash
		// on first run instead of a prompt to reindex.
		if errors.Is(err, ErrIndexSchema) {
			return DriftReport{Changed: true, Reason: "index_schema_mismatch", ActualDigest: actual}, nil
		}
		return DriftReport{}, err
	}
	changed := index.SourceDigest != actual
	report := DriftReport{OK: !changed, Changed: changed, ExpectedDigest: index.SourceDigest, ActualDigest: actual}
	if changed {
		report.Reason = "source_digest_mismatch"
	}
	return report, nil
}

// ReconcileMode selects whether Reconcile may write. The zero value is
// ReconcileDryRun, so a caller that forgets to choose cannot mutate anything.
type ReconcileMode int

// Reconcile modes.
const (
	// ReconcileDryRun reports drift without writing. It is the zero value.
	ReconcileDryRun ReconcileMode = iota
	// ReconcileApply rebuilds the index when drift exists.
	ReconcileApply
)

// String renders the mode as its wire value.
func (m ReconcileMode) String() string {
	switch m {
	case ReconcileApply:
		return "apply"
	case ReconcileDryRun:
		return "dry-run"
	}
	return "unknown"
}

// Valid reports whether m is a recognized mode.
func (m ReconcileMode) Valid() bool { return m == ReconcileDryRun || m == ReconcileApply }

// MarshalJSON renders the mode as a string and rejects unknown values.
func (m ReconcileMode) MarshalJSON() ([]byte, error) {
	if !m.Valid() {
		return nil, fmt.Errorf("%w: unknown reconcile mode %d", ErrPolicyViolation, int(m))
	}
	return []byte(`"` + m.String() + `"`), nil
}

// UnmarshalJSON accepts only the known mode names.
func (m *ReconcileMode) UnmarshalJSON(payload []byte) error {
	var raw string
	if err := json.Unmarshal(payload, &raw); err != nil {
		return err
	}
	switch raw {
	case "dry-run":
		*m = ReconcileDryRun
	case "apply":
		*m = ReconcileApply
	default:
		return fmt.Errorf("%w: unknown reconcile mode %q", ErrPolicyViolation, raw)
	}
	return nil
}

// ReconcileReport records what a reconcile observed and whether it wrote.
type ReconcileReport struct {
	Drift   DriftReport    `json:"drift"`
	Mode    ReconcileMode  `json:"mode"`
	Applied bool           `json:"applied"`
	Reindex *ReindexReport `json:"reindex,omitempty"`
	_       struct{}
}

// Reconcile reports drift and, in ReconcileApply mode, rewrites the index.
func (e *Engine) Reconcile(ctx context.Context, mode ReconcileMode) (ReconcileReport, error) {
	if !mode.Valid() {
		return ReconcileReport{}, fmt.Errorf("%w: unknown reconcile mode %d", ErrPolicyViolation, int(mode))
	}
	drift, err := e.Drift(ctx)
	if err != nil {
		return ReconcileReport{}, err
	}
	report := ReconcileReport{Drift: drift, Mode: mode}
	if !drift.Changed || mode != ReconcileApply {
		return report, nil
	}
	reindexed, err := e.Reindex(ctx)
	if err != nil {
		return ReconcileReport{}, err
	}
	report.Applied = true
	report.Reindex = &reindexed
	return report, nil
}
