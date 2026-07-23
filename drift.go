package catalog

import (
	"context"
	"errors"
	"os"
)

type DriftReport struct {
	OK             bool   `json:"ok"`
	Changed        bool   `json:"changed"`
	Reason         string `json:"reason,omitempty"`
	ExpectedDigest string `json:"expected_digest,omitempty"`
	ActualDigest   string `json:"actual_digest,omitempty"`
}

func (e *Engine) Drift(ctx context.Context) (DriftReport, error) {
	records, err := e.Discover(ctx)
	if err != nil {
		return DriftReport{}, err
	}
	actual, err := recordsDigest(records)
	if err != nil {
		return DriftReport{}, err
	}
	index, err := e.LoadIndex()
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return DriftReport{Changed: true, Reason: "index_missing", ActualDigest: actual}, nil
		}
		// LoadIndex wraps the missing-file error with actionable context.
		if _, statErr := os.Stat(e.layout.indexPath()); errors.Is(statErr, os.ErrNotExist) {
			return DriftReport{Changed: true, Reason: "index_missing", ActualDigest: actual}, nil
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

type ReconcileReport struct {
	Drift   DriftReport    `json:"drift"`
	Applied bool           `json:"applied"`
	Reindex *ReindexReport `json:"reindex,omitempty"`
}

func (e *Engine) Reconcile(ctx context.Context, apply bool) (ReconcileReport, error) {
	drift, err := e.Drift(ctx)
	if err != nil {
		return ReconcileReport{}, err
	}
	report := ReconcileReport{Drift: drift}
	if !drift.Changed || !apply {
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
