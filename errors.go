package catalog

import (
	"errors"
	"fmt"
)

// Sentinel errors. Callers match these with errors.Is rather than comparing
// error strings. Every sentinel is prefixed "catalog:" so a wrapped error
// remains self-identifying when it is logged by a host.
var (
	ErrInvalidLayout         = errors.New("catalog: invalid layout")
	ErrInvalidProvider       = errors.New("catalog: invalid provider")
	ErrProviderFailed        = errors.New("catalog: provider failed")
	ErrInvalidEntity         = errors.New("catalog: invalid entity")
	ErrDuplicateEntityID     = errors.New("catalog: duplicate entity id")
	ErrDecode                = errors.New("catalog: decode failed")
	ErrCorpusEscape          = errors.New("catalog: corpus path escapes the corpus root")
	ErrIndexMissing          = errors.New("catalog: index missing")
	ErrIndexSchema           = errors.New("catalog: unsupported index schema")
	ErrIndexCorrupt          = errors.New("catalog: index is corrupt")
	ErrEmptyQuery            = errors.New("catalog: query has no usable terms")
	ErrPolicyViolation       = errors.New("catalog: projection policy violation")
	ErrHostAllowlistRequired = errors.New("catalog: public_url requires a non-empty host allowlist")
	ErrPublicURLRejected     = errors.New("catalog: public_url rejected")
	ErrProhibitedContent     = errors.New("catalog: prohibited public content")
)

// PolicyRule names the publication rule a value violated. It is a closed
// vocabulary so hosts can branch on the cause without parsing error text.
type PolicyRule string

// Publication rules reported by PolicyError.
const (
	RulePathDisclosure PolicyRule = "path-disclosure"
	RuleInternalPath   PolicyRule = "internal-path"
	RuleCredentialPair PolicyRule = "credential-assignment"
	RuleTokenShape     PolicyRule = "token-shape"
	RuleInvalidUTF8    PolicyRule = "invalid-utf8"
	RuleURLScheme      PolicyRule = "url-scheme"
	RuleURLCredentials PolicyRule = "url-credentials"
	RuleURLQuery       PolicyRule = "url-query-or-fragment"
	RuleURLPort        PolicyRule = "url-port"
	RuleURLHost        PolicyRule = "url-host-not-allowed"
	RuleVisibility     PolicyRule = "visibility"
)

// PolicyError reports that a value was refused publication.
//
// It deliberately carries no copy of the offending text. The rejected value is
// the exact thing publication is meant to contain, and this error travels to
// stderr, CI logs, and host error paths; reproducing the match there would
// defeat the boundary the projection exists to enforce. Callers get the entity,
// the field, and the rule, which is enough to locate the value in the source
// they already control.
type PolicyError struct {
	// EntityID is the entity that failed, when the check was entity-scoped.
	EntityID string
	// Field names the offending field, such as summary or tags[2].
	Field string
	// Rule is the publication rule that was violated.
	Rule PolicyRule
	// Err is the underlying sentinel.
	Err error
}

// Error renders the entity, field, and rule. It never includes the rejected value.
func (e *PolicyError) Error() string {
	if e.EntityID == "" {
		return fmt.Sprintf("public field %s violates %s", e.Field, e.Rule)
	}
	return fmt.Sprintf("entity %s: public field %s violates %s", e.EntityID, e.Field, e.Rule)
}

// Unwrap exposes the underlying sentinel.
func (e *PolicyError) Unwrap() error { return e.Err }

// RecordOrigin identifies where a record entered the engine.
type RecordOrigin struct {
	// Provider named the record's source provider.
	Provider string
	// Source located the record within that provider.
	Source string
	_      struct{}
}

// ProviderError wraps a failure attributed to a named provider.
type ProviderError struct {
	// Provider is the failing provider's name.
	Provider string
	// Source is the record location, when the failure is attributable to one.
	Source string
	// Err is the provider's own error.
	Err error
}

// Error names the provider and, when known, the source.
func (e *ProviderError) Error() string {
	if e.Source == "" {
		return fmt.Sprintf("provider %s: %v", e.Provider, e.Err)
	}
	return fmt.Sprintf("provider %s (%s): %v", e.Provider, e.Source, e.Err)
}

// Unwrap exposes the provider's own error.
func (e *ProviderError) Unwrap() error { return e.Err }

// EntityError wraps a failure attributed to a single entity.
type EntityError struct {
	// EntityID is the offending entity, when it could be read.
	EntityID string
	// Provider supplied the entity.
	Provider string
	// Source located it within that provider.
	Source string
	// Field names the offending field, when the failure is field-scoped.
	Field string
	// Err is the underlying cause.
	Err error
}

// Error names the entity and, when known, the offending field.
func (e *EntityError) Error() string {
	id := e.EntityID
	if id == "" {
		id = "<unnamed>"
	}
	if e.Field == "" {
		return fmt.Sprintf("entity %s: %v", id, e.Err)
	}
	return fmt.Sprintf("entity %s field %s: %v", id, e.Field, e.Err)
}

// Unwrap exposes the underlying cause.
func (e *EntityError) Unwrap() error { return e.Err }

// DuplicateIDError reports the same entity id arriving from two origins. Both
// origins are retained so a host can report the collision without re-running
// discovery.
type DuplicateIDError struct {
	// EntityID is the id claimed twice.
	EntityID string
	// First is the origin encountered first in deterministic order.
	First RecordOrigin
	// Second is the colliding origin.
	Second RecordOrigin
}

// Error names the id and both origins that claimed it.
func (e *DuplicateIDError) Error() string {
	return fmt.Sprintf("duplicate entity id %s from %s (%s) and %s (%s)",
		e.EntityID, e.First.Provider, e.First.Source, e.Second.Provider, e.Second.Source)
}

// Unwrap reports ErrDuplicateEntityID.
func (e *DuplicateIDError) Unwrap() error { return ErrDuplicateEntityID }

// DecodeError reports a corpus payload that could not be decoded.
type DecodeError struct {
	// Path is the payload that failed to decode.
	Path string
	// Err is the decoder's error.
	Err error
}

// Error names the undecodable path.
func (e *DecodeError) Error() string { return fmt.Sprintf("decode %s: %v", e.Path, e.Err) }

// Unwrap exposes the decoder's error.
func (e *DecodeError) Unwrap() error { return e.Err }

// IndexError reports a failure reading or writing the derived index.
type IndexError struct {
	// Path is the index file involved.
	Path string
	// Err is the underlying cause, wrapping an Err* sentinel.
	Err error
}

// Error names the index path.
func (e *IndexError) Error() string { return fmt.Sprintf("index %s: %v", e.Path, e.Err) }

// Unwrap exposes the underlying cause.
func (e *IndexError) Unwrap() error { return e.Err }
