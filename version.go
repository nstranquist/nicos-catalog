package catalog

var (
	Version = "v0.1.0"
	Commit  = "unknown"
)

type BuildInfo struct {
	Version       string   `json:"version"`
	Commit        string   `json:"commit"`
	SchemaVersion int      `json:"schema_version"`
	Capabilities  []string `json:"capabilities"`
}

func VersionInfo() BuildInfo {
	return BuildInfo{
		Version: Version, Commit: Commit, SchemaVersion: SchemaVersion,
		Capabilities: []string{"providers", "layout", "validate", "reindex", "bm25-search", "graph", "drift", "reconcile", "public-projection", "synthetic-demo"},
	}
}
