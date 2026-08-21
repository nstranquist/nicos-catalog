package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

// prohibitedRule pairs a detection pattern with the closed rule it reports.
type prohibitedRule struct {
	pattern *regexp.Regexp
	rule    PolicyRule
}

// prohibitedPublicContent detects values that must never reach a public
// projection. The home-directory patterns require a following path segment
// (/users/<name>/ rather than bare /users/) so that an ordinary site route is
// not mistaken for a leaked local path, and they are deliberately not anchored
// to a word boundary: a leaked path embedded mid-token is still a leaked path.
var prohibitedPublicContent = []prohibitedRule{
	{regexp.MustCompile(`(?i)(?:/users/[^/\s]+/|/home/[^/\s]+/|file://|~/|(?:^|[\s"'(])[a-z]:\\)`), RulePathDisclosure},
	{regexp.MustCompile(`(?i)\b(?:` + "private-admin-" + `evidence|sources/(?:originals|extracted-text)|\.jobkit|\.ssh)\b`), RuleInternalPath},
	{regexp.MustCompile(`(?i)\b(?:api[_-]?key|access[_-]?token|secret|password)\s*[:=]\s*\S+`), RuleCredentialPair},
	{regexp.MustCompile(`\b(?:gh[pousr]_[A-Za-z0-9]{20,}|sk-[A-Za-z0-9_-]{20,})\b`), RuleTokenShape},
}

// PublicEntity is a closed publication DTO. It cannot represent source paths,
// host annotations, owner telemetry, sidecars, valuation, or query text.
//
// The field set is frozen by TestPublicEntityShapeIsFrozen. Adding a field is a
// deliberate privacy decision, not a routine change.
type PublicEntity struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Kind        string             `json:"kind"`
	Status      string             `json:"status,omitempty"`
	Summary     string             `json:"summary,omitempty"`
	Tags        []string           `json:"tags,omitempty"`
	URL         string             `json:"url,omitempty"`
	Connections []PublicConnection `json:"connections,omitempty"`
	_           struct{}
}

// PublicConnection is a reference between two entities that both survived the
// projection filter. References to excluded entities are dropped entirely.
type PublicConnection struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	_      struct{}
}

// PublicProjection is the closed publication artifact. Publication should
// consume this DTO rather than filtering a private index after serialization.
type PublicProjection struct {
	SchemaVersion int            `json:"schema_version"`
	Items         []PublicEntity `json:"items"`
	_             struct{}
}

// URLMode selects what happens to an entity URL that fails the allowlist. The
// zero value rejects, so a caller that forgets to choose cannot publish one.
type URLMode int

// URL handling modes.
const (
	// URLModeAllowlist fails the projection when a URL is not allowlisted.
	URLModeAllowlist URLMode = iota
	// URLModeDrop omits the URL and keeps the entity.
	URLModeDrop
)

// ProjectionPolicy constrains what ProjectPublic will emit.
type ProjectionPolicy struct {
	// RequireVisibility is the visibility an entity must declare to be
	// projected. The empty value means VisibilityPublic; no other value is
	// accepted, because only public entities are publishable.
	RequireVisibility Visibility
	// IncludeKinds limits the projection to these kinds. Matching is
	// case-insensitive, consistent with Search. Empty means every kind.
	IncludeKinds []string
	// IncludeTags limits the projection to entities carrying at least one of
	// these tags. Empty means no tag filtering.
	IncludeTags []string
	// ExcludeTags removes entities carrying any of these tags. A denylist match
	// beats an IncludeTags match, so a tag can be used to withhold an entity
	// that would otherwise qualify.
	ExcludeTags []string
	// AllowHosts is the exact-match hostname allowlist for PublicURL. It must
	// be non-empty whenever any projected entity declares a PublicURL;
	// otherwise projection fails closed. Subdomains are not implied.
	AllowHosts []string
	// URLMode selects allowlist enforcement. The zero value rejects.
	URLMode URLMode
	// MaxSummaryBytes bounds the emitted summary, including the truncation
	// marker. Zero or negative selects the 320-byte default.
	MaxSummaryBytes int
	// TruncationSuffix marks a shortened summary. Empty selects "…". Its bytes
	// are charged against MaxSummaryBytes rather than added after it.
	TruncationSuffix string
	_                struct{}
}

// Validate rejects policies that cannot be satisfied, so a host can fail at
// config load rather than at publication time.
func (p ProjectionPolicy) Validate() error {
	if visibility := p.RequireVisibility; visibility != "" && visibility != VisibilityPublic {
		return &PolicyError{Field: "require_visibility", Rule: RuleVisibility, Err: ErrPolicyViolation}
	}
	if p.MaxSummaryBytes < 0 {
		return fmt.Errorf("%w: max_summary_bytes must not be negative", ErrPolicyViolation)
	}
	if p.URLMode != URLModeAllowlist && p.URLMode != URLModeDrop {
		return fmt.Errorf("%w: unknown url mode %d", ErrPolicyViolation, int(p.URLMode))
	}
	overlap := stringSet(p.IncludeTags)
	for _, tag := range p.ExcludeTags {
		if _, ok := overlap[strings.TrimSpace(tag)]; ok {
			return fmt.Errorf("%w: tag %q is both included and excluded", ErrPolicyViolation, tag)
		}
	}
	return nil
}

const defaultTruncationSuffix = "…"

// defaultMaxSummaryBytes bounds a projected summary when the policy does not.
const defaultMaxSummaryBytes = 320

// ScanPublicText reports whether value is safe to publish for the named field.
// It returns a *PolicyError naming the violated rule, and never reproduces the
// offending text. Hosts building their own publication gates should call this
// rather than reimplementing the patterns, so library and host cannot drift.
func ScanPublicText(field, value string) error {
	if !utf8.ValidString(value) {
		return &PolicyError{Field: field, Rule: RuleInvalidUTF8, Err: ErrProhibitedContent}
	}
	for _, candidate := range prohibitedPublicContent {
		if candidate.pattern.MatchString(value) {
			return &PolicyError{Field: field, Rule: candidate.rule, Err: ErrProhibitedContent}
		}
	}
	return nil
}

// withEntity attaches an entity id to a policy error raised by a field-scoped
// check, so the caller reports one fully-identified error.
func withEntity(id string, err error) error {
	if err == nil {
		return nil
	}
	var policy *PolicyError
	if errors.As(err, &policy) {
		policy.EntityID = id
		return policy
	}
	return err
}

// ProjectPublic compiles the closed public projection for index under policy.
// It fails closed: any entity that violates the policy aborts the projection
// rather than being silently dropped.
func ProjectPublic(ctx context.Context, index Index, policy ProjectionPolicy) (PublicProjection, error) {
	if err := ctx.Err(); err != nil {
		return PublicProjection{}, err
	}
	if err := policy.Validate(); err != nil {
		return PublicProjection{}, err
	}
	kinds := foldedSet(policy.IncludeKinds)
	tags := stringSet(policy.IncludeTags)
	excluded := stringSet(policy.ExcludeTags)
	hosts := hostSet(policy.AllowHosts)
	maxSummary := policy.MaxSummaryBytes
	if maxSummary <= 0 {
		maxSummary = defaultMaxSummaryBytes
	}
	suffix := policy.TruncationSuffix
	if suffix == "" {
		suffix = defaultTruncationSuffix
	}
	requireVisibility := Visibility(strings.TrimSpace(string(policy.RequireVisibility)))
	if requireVisibility == "" {
		requireVisibility = VisibilityPublic
	}

	projection := PublicProjection{SchemaVersion: SchemaVersion}
	included := map[string]struct{}{}
	for _, entity := range index.Entities {
		if entity.Visibility != requireVisibility {
			continue
		}
		if len(kinds) > 0 {
			if _, ok := kinds[strings.ToLower(strings.TrimSpace(entity.Kind))]; !ok {
				continue
			}
		}
		if len(excluded) > 0 && hasAny(entity.Tags, excluded) {
			continue
		}
		if len(tags) > 0 && !hasAny(entity.Tags, tags) {
			continue
		}
		publicURL := entity.PublicURL
		if err := validatePublicURL(publicURL, hosts); err != nil {
			if policy.URLMode != URLModeDrop {
				return PublicProjection{}, withEntity(entity.ID, err)
			}
			publicURL = ""
		}
		if err := validatePublicEntityContent(entity); err != nil {
			return PublicProjection{}, withEntity(entity.ID, err)
		}
		projection.Items = append(projection.Items, PublicEntity{
			ID: entity.ID, Name: entity.Name, Kind: entity.Kind, Status: entity.Status,
			Summary: boundedSummary(entity.Description, maxSummary, suffix),
			Tags:    append([]string(nil), entity.Tags...), URL: publicURL,
		})
		included[entity.ID] = struct{}{}
	}

	// Index entities by id once. Resolving each item's refs by scanning the
	// entity slice made this loop quadratic in corpus size.
	byID := make(map[string]int, len(index.Entities))
	for i := range index.Entities {
		if _, seen := byID[index.Entities[i].ID]; !seen {
			byID[index.Entities[i].ID] = i
		}
	}
	for i := range projection.Items {
		position, ok := byID[projection.Items[i].ID]
		if !ok {
			continue
		}
		for _, ref := range index.Entities[position].Refs {
			if _, ok := included[ref.Target]; ok {
				projection.Items[i].Connections = append(projection.Items[i].Connections,
					PublicConnection{Kind: ref.Kind, Target: ref.Target})
			}
		}
	}
	sort.Slice(projection.Items, func(i, j int) bool { return projection.Items[i].ID < projection.Items[j].ID })
	return projection, nil
}

// boundedSummary trims value to at most maxBytes total. The ellipsis is charged
// against the budget rather than appended after it, so the returned string
// never exceeds the caller's declared bound.
func boundedSummary(value string, maxBytes int, suffix string) string {
	// Sanitize before measuring. ProjectPublic rejects invalid UTF-8 upstream,
	// but this helper must hold its own invariant: a caller that reorders the
	// checks must not be able to publish malformed bytes. truncateUTF8 only
	// avoids splitting a rune; it cannot repair input that was already invalid.
	summary := strings.TrimSpace(strings.ToValidUTF8(value, ""))
	if len(summary) <= maxBytes {
		return summary
	}
	budget := maxBytes - len(suffix)
	if budget <= 0 {
		return truncateUTF8(summary, maxBytes)
	}
	return truncateUTF8(summary, budget) + suffix
}

func truncateUTF8(value string, maxBytes int) string {
	if maxBytes <= 0 || len(value) <= maxBytes {
		return strings.TrimSpace(value)
	}
	end := maxBytes
	for end > 0 && !utf8.ValidString(value[:end]) {
		end--
	}
	return strings.TrimSpace(value[:end])
}

// validatePublicURL enforces the URL shape and hostname allowlist, and scans
// the URL's own text. The path segment is attacker- and author-controlled
// content that is published verbatim, so it is scanned like any other field.
func validatePublicURL(raw string, allowed map[string]struct{}) error {
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.Opaque != "" {
		return &PolicyError{Field: "public_url", Rule: RuleURLScheme, Err: ErrPublicURLRejected}
	}
	// Reject an at-sign anywhere, not only parsed user-info. A raw or decoded
	// path at-sign is ambiguous in copied URLs and can hide a credential-like
	// authority marker from a later parser or proxy.
	if parsed.User != nil || strings.Contains(raw, "@") || strings.Contains(parsed.Path, "@") {
		return &PolicyError{Field: "public_url", Rule: RuleURLCredentials, Err: ErrPublicURLRejected}
	}
	// url.Parse treats a trailing empty fragment ("…#") as Fragment==""; still reject
	// any raw query or fragment marker so fuzz and policy stay fail-closed.
	if parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawFragment != "" ||
		strings.Contains(raw, "?") || strings.Contains(raw, "#") {
		return &PolicyError{Field: "public_url", Rule: RuleURLQuery, Err: ErrPublicURLRejected}
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return &PolicyError{Field: "public_url", Rule: RuleURLPort, Err: ErrPublicURLRejected}
	}
	if len(allowed) == 0 {
		return &PolicyError{Field: "public_url", Rule: RuleURLHost, Err: ErrHostAllowlistRequired}
	}
	if _, ok := allowed[strings.ToLower(parsed.Hostname())]; !ok {
		return &PolicyError{Field: "public_url", Rule: RuleURLHost, Err: ErrPublicURLRejected}
	}
	if err := ScanPublicText("public_url", parsed.Path); err != nil {
		return err
	}
	return ScanPublicText("public_url", raw)
}

func validatePublicEntityContent(entity Entity) error {
	type fieldValue struct {
		field string
		value string
	}
	values := []fieldValue{
		{"id", entity.ID}, {"name", entity.Name}, {"kind", entity.Kind},
		{"status", entity.Status}, {"summary", entity.Description},
	}
	for i, tag := range entity.Tags {
		values = append(values, fieldValue{"tags[" + itoa(i) + "]", tag})
	}
	for i, ref := range entity.Refs {
		values = append(values,
			fieldValue{"refs[" + itoa(i) + "].kind", ref.Kind},
			fieldValue{"refs[" + itoa(i) + "].target", ref.Target},
		)
	}
	for _, item := range values {
		if err := ScanPublicText(item.field, item.value); err != nil {
			return err
		}
	}
	return nil
}

// itoa avoids pulling fmt into the projection hot path for index formatting.
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := [20]byte{}
	position := len(digits)
	for value > 0 {
		position--
		digits[position] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[position:])
}

func stringSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

// foldedSet lowercases as well as trims, so kind filtering matches Search.
func foldedSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func hostSet(values []string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			out[value] = struct{}{}
		}
	}
	return out
}

func hasAny(values []string, expected map[string]struct{}) bool {
	for _, value := range values {
		if _, ok := expected[value]; ok {
			return true
		}
	}
	return false
}
