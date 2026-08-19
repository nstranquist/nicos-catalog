package catalog

import (
	_ "embed"
	"runtime/debug"
	"sort"
	"strings"
)

// versionFile is the single source of the module's released version. Every
// other site that needs it — the CLI, the Makefile release check, the README
// pins, the CHANGELOG head — is verified against this file rather than
// carrying its own copy.
//
//go:embed VERSION
var versionFile string

var (
	version = strings.TrimSpace(versionFile)
	// commit is injected at build time with
	//   -ldflags "-X github.com/nstranquist/nicos-catalog.commit=<sha>"
	// and otherwise recovered from the embedded VCS stamp.
	commit string
)

// Version reports the module version. It is a function rather than a variable
// so an importing host cannot rewrite the identity the library reports.
func Version() string { return version }

// Commit reports the source revision this binary was built from, or the empty
// string when it cannot be determined.
func Commit() string {
	if commit != "" {
		return commit
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return ""
}

// Capability names an engine feature a host can depend on. It is a closed
// vocabulary so consumers branch on constants rather than string literals.
type Capability string

// Capabilities advertised by this build.
const (
	CapabilityProviders        Capability = "providers"
	CapabilityLayout           Capability = "layout"
	CapabilityValidate         Capability = "validate"
	CapabilityReindex          Capability = "reindex"
	CapabilityBM25Search       Capability = "bm25-search"
	CapabilityGraph            Capability = "graph"
	CapabilityDrift            Capability = "drift"
	CapabilityReconcile        Capability = "reconcile"
	CapabilityPublicProjection Capability = "public-projection"
	CapabilitySyntheticDemo    Capability = "synthetic-demo"
	CapabilityExplorer         Capability = "explorer"
	CapabilityExplorerExport   Capability = "explorer-static-export"
	CapabilityReadOnlyMCP      Capability = "read-only-mcp"
)

// Capabilities returns the sorted capability set of this build. The returned
// slice is a fresh copy; mutating it does not affect the engine.
func Capabilities() []Capability {
	out := []Capability{
		CapabilityProviders, CapabilityLayout, CapabilityValidate, CapabilityReindex,
		CapabilityBM25Search, CapabilityGraph, CapabilityDrift, CapabilityReconcile,
		CapabilityPublicProjection, CapabilitySyntheticDemo, CapabilityExplorer,
		CapabilityExplorerExport, CapabilityReadOnlyMCP,
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// BuildInfo identifies the engine and the contract it implements.
type BuildInfo struct {
	// Version is the module's released version.
	Version string `json:"version"`
	// Commit is the source revision, when it can be determined.
	Commit string `json:"commit,omitempty"`
	// Modified reports whether the build tree had uncommitted changes.
	Modified bool `json:"modified,omitempty"`
	// SchemaVersion is the on-disk index contract this build reads and writes.
	SchemaVersion int `json:"schema_version"`
	// Capabilities lists the engine features this build advertises.
	Capabilities []Capability `json:"capabilities"`
	_            struct{}
}

// Has reports whether the build advertises the named capability.
func (b BuildInfo) Has(capability Capability) bool {
	for _, candidate := range b.Capabilities {
		if candidate == capability {
			return true
		}
	}
	return false
}

// VersionInfo describes the running engine.
func VersionInfo() BuildInfo {
	info := BuildInfo{
		Version:       Version(),
		Commit:        Commit(),
		SchemaVersion: SchemaVersion,
		Capabilities:  Capabilities(),
	}
	if build, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range build.Settings {
			if setting.Key == "vcs.modified" && setting.Value == "true" {
				info.Modified = true
			}
		}
	}
	return info
}
