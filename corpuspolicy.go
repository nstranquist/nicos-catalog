package catalog

import "strings"

// SkipReason names why a corpus path was excluded. It is a closed vocabulary so
// a host can report and assert on the decision rather than re-deriving it.
type SkipReason string

// Corpus skip reasons.
const (
	// SkipNone means the path is a candidate entity record.
	SkipNone SkipReason = ""
	// SkipDotDir excludes a dot-prefixed directory.
	SkipDotDir SkipReason = "dot-prefixed-dir"
	// SkipUnderscoreDir excludes an underscore-prefixed directory.
	SkipUnderscoreDir SkipReason = "underscore-prefixed-dir"
	// SkipDeniedDir excludes a directory named in SkipDirNames.
	SkipDeniedDir SkipReason = "denied-dir-name"
	// SkipUnderscoreFile excludes an underscore-prefixed file.
	SkipUnderscoreFile SkipReason = "underscore-prefixed-file"
	// SkipUnknownExtension excludes a file whose extension is not accepted.
	SkipUnknownExtension SkipReason = "unaccepted-extension"
)

// CorpusDecision is the outcome of a corpus-membership test.
type CorpusDecision struct {
	// Skip reports whether the path is excluded from the corpus.
	Skip bool `json:"skip"`
	// Reason names the rule that excluded it, empty when Skip is false.
	Reason SkipReason `json:"reason,omitempty"`
	_      struct{}
}

// CorpusPolicy decides which directories and files under Layout.CorpusDir are
// candidate entity records.
//
// It is data rather than behavior, so a host can declare its corpus shape once
// and have both the engine and the host's own loader consult the same decision.
// That matters because corpus membership is a privacy and correctness boundary:
// a tombstoned or generated tree that is skipped by one reader and walked by
// another silently resurrects entities the host believes are gone.
type CorpusPolicy struct {
	// SkipDotPrefixedDirs excludes directories beginning with ".", which
	// conventionally hold tooling state rather than authored records.
	SkipDotPrefixedDirs bool
	// SkipUnderscorePrefixedDirs excludes directories beginning with "_".
	// This generalizes the older hardcoded _archive rule and is what makes an
	// archive tree structurally invisible rather than invisible by coincidence
	// of naming.
	SkipUnderscorePrefixedDirs bool
	// SkipUnderscorePrefixedFiles excludes files beginning with "_", which
	// hosts commonly use for generated indexes sitting beside authored records.
	SkipUnderscorePrefixedFiles bool
	// SkipDirNames excludes directories by exact name.
	SkipDirNames []string
	// Extensions are the accepted file extensions, lowercase and dot-prefixed.
	// Empty accepts every extension.
	Extensions []string
	// CaseFoldExtensions matches extensions case-insensitively. Hosts whose
	// corpus is compared byte-for-byte against a generated artifact may need
	// this off, so that a newly-added Foo.MD cannot silently join the corpus.
	CaseFoldExtensions bool
	_                  struct{}
}

// DefaultCorpusPolicy is the general-purpose policy: skip tooling, dependency,
// and archive trees, and accept the three authored entity formats.
func DefaultCorpusPolicy() CorpusPolicy {
	return CorpusPolicy{
		SkipDotPrefixedDirs:         true,
		SkipUnderscorePrefixedDirs:  true,
		SkipUnderscorePrefixedFiles: false,
		SkipDirNames:                []string{"node_modules", "vendor"},
		Extensions:                  []string{".md", ".yaml", ".yml", ".json"},
		CaseFoldExtensions:          true,
	}
}

// StrictMarkdownCorpusPolicy is for hosts whose corpus is Markdown-only and
// whose generated indexes live beside the authored records under an underscore
// prefix. Extension matching is case-sensitive so the accepted set cannot grow
// by accident.
func StrictMarkdownCorpusPolicy() CorpusPolicy {
	return CorpusPolicy{
		SkipDotPrefixedDirs:         false,
		SkipUnderscorePrefixedDirs:  true,
		SkipUnderscorePrefixedFiles: true,
		SkipDirNames:                []string{".cache"},
		Extensions:                  []string{".md"},
		CaseFoldExtensions:          false,
	}
}

// Normalize returns a policy with its name and extension sets trimmed and
// deduplicated. It is idempotent.
func (p CorpusPolicy) Normalize() CorpusPolicy {
	p.SkipDirNames = normalizeStrings(p.SkipDirNames)
	extensions := make([]string, 0, len(p.Extensions))
	for _, ext := range p.Extensions {
		ext = strings.TrimSpace(ext)
		if ext == "" {
			continue
		}
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		if p.CaseFoldExtensions {
			ext = strings.ToLower(ext)
		}
		extensions = append(extensions, ext)
	}
	p.Extensions = normalizeStrings(extensions)
	return p
}

// DecideDir reports whether a directory with the given base name is excluded.
func (p CorpusPolicy) DecideDir(name string) CorpusDecision {
	if p.SkipDotPrefixedDirs && strings.HasPrefix(name, ".") {
		return CorpusDecision{Skip: true, Reason: SkipDotDir}
	}
	if p.SkipUnderscorePrefixedDirs && strings.HasPrefix(name, "_") {
		return CorpusDecision{Skip: true, Reason: SkipUnderscoreDir}
	}
	for _, denied := range p.SkipDirNames {
		if name == denied {
			return CorpusDecision{Skip: true, Reason: SkipDeniedDir}
		}
	}
	return CorpusDecision{}
}

// DecideFile reports whether a file with the given base name is excluded.
func (p CorpusPolicy) DecideFile(name string) CorpusDecision {
	if p.SkipUnderscorePrefixedFiles && strings.HasPrefix(name, "_") {
		return CorpusDecision{Skip: true, Reason: SkipUnderscoreFile}
	}
	if len(p.Extensions) == 0 {
		return CorpusDecision{}
	}
	ext := fileExtension(name)
	if p.CaseFoldExtensions {
		ext = strings.ToLower(ext)
	}
	for _, accepted := range p.Extensions {
		if ext == accepted {
			return CorpusDecision{}
		}
	}
	return CorpusDecision{Skip: true, Reason: SkipUnknownExtension}
}

// fileExtension returns the final dot-suffix of name, or the empty string.
// It deliberately does not use filepath.Ext so the decision is identical on
// every platform for a base name that is already separated.
func fileExtension(name string) string {
	index := strings.LastIndex(name, ".")
	if index < 0 {
		return ""
	}
	return name[index:]
}
