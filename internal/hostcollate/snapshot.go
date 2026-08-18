package hostcollate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	catalog "github.com/nstranquist/nicos-catalog"
)

// SnapshotFileName is stored next to the derived index in CacheDir.
const SnapshotFileName = "collation-snapshot.json"

// Snapshot is the last explicit collation report plus a content digest.
type Snapshot struct {
	SchemaVersion int              `json:"schema_version"`
	Digest        string           `json:"digest"`
	Path          string           `json:"path,omitempty"`
	Report        Report           `json:"report"`
	Records       []catalog.Record `json:"records"`
}

// SnapshotPath joins cacheDir with the snapshot file name.
func SnapshotPath(cacheDir string) string {
	return filepath.Join(cacheDir, SnapshotFileName)
}

// WriteSnapshot persists report next to derived catalog state.
func WriteSnapshot(cacheDir string, report Report, records []catalog.Record) (Snapshot, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return Snapshot{}, fmt.Errorf("create snapshot dir: %w", err)
	}
	copied := append([]catalog.Record(nil), records...)
	if copied == nil {
		copied = []catalog.Record{}
	}
	snap := Snapshot{SchemaVersion: 1, Report: reportWithoutSnapshot(report), Records: copied}
	digest, err := snapshotDigest(snap)
	if err != nil {
		return Snapshot{}, err
	}
	snap.Digest = digest
	path := SnapshotPath(cacheDir)
	snap.Path = path
	payload, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return Snapshot{}, fmt.Errorf("encode snapshot: %w", err)
	}
	if err := os.WriteFile(path, append(payload, '\n'), 0o644); err != nil {
		return Snapshot{}, fmt.Errorf("write snapshot: %w", err)
	}
	return snap, nil
}

// ReadSnapshot loads the last explicit refresh without walking roots.
func ReadSnapshot(cacheDir string) (Snapshot, error) {
	path := SnapshotPath(cacheDir)
	payload, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, fmt.Errorf("read snapshot: %w", err)
	}
	var snap Snapshot
	if err := json.Unmarshal(payload, &snap); err != nil {
		return Snapshot{}, fmt.Errorf("decode snapshot: %w", err)
	}
	snap.Path = path
	return snap, nil
}

func reportWithoutSnapshot(report Report) Report {
	report.SnapshotPath = ""
	report.SnapshotDigest = ""
	return report
}

func snapshotDigest(snap Snapshot) (string, error) {
	payload, err := json.Marshal(struct {
		Report  Report           `json:"report"`
		Records []catalog.Record `json:"records"`
	}{Report: reportWithoutSnapshot(snap.Report), Records: snap.Records})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
