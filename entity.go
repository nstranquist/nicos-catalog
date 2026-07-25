package catalog

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// SchemaVersion is the on-disk contract version of the derived index.
//
// Version 2 changed Record.Digest from a whole-payload digest to a per-entity
// digest and introduced Record.SourceDigest, so a version 1 index cannot be
// compared against a version 2 discovery. Drift reports the difference as
// index_schema_mismatch rather than failing, which makes an engine upgrade a
// reindex prompt instead of a crash.
const SchemaVersion = 2

var entityIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

// EntityIDPattern returns the portable identifier grammar as a string.
//
// Hosts with a stricter grammar of their own should assert that theirs is a
// subset of this one rather than adopting it; loosening a host grammar to match
// can admit identifiers the host's own schema rejects.
func EntityIDPattern() string { return entityIDPattern.String() }

// ValidateEntityID reports whether id is a well-formed portable entity id.
func ValidateEntityID(id string) error {
	if !entityIDPattern.MatchString(id) {
		return fmt.Errorf("%w: id %q must match %s", ErrInvalidEntity, id, entityIDPattern)
	}
	return nil
}

// Visibility is the closed vocabulary controlling whether an entity may be
// published. Only VisibilityPublic is projectable.
type Visibility string

// The visibility vocabulary.
const (
	// VisibilityPublic marks an entity as publishable.
	VisibilityPublic Visibility = "public"
	// VisibilityInternal marks an entity as host-wide but unpublishable.
	VisibilityInternal Visibility = "internal"
	// VisibilityPrivate marks an entity as restricted to its owner.
	VisibilityPrivate Visibility = "private"
)

// Valid reports whether v is a recognized visibility. The empty value is valid
// and means "unset", which is never projectable.
func (v Visibility) Valid() bool {
	switch v {
	case "", VisibilityPublic, VisibilityInternal, VisibilityPrivate:
		return true
	}
	return false
}

// String renders the visibility as its wire value.
func (v Visibility) String() string { return string(v) }

// Entity is the portable catalog record. Host-only business, telemetry, and
// operator fields belong in host adapters rather than this public contract.
type Entity struct {
	// ID is the stable identifier, unique across every provider.
	ID string `json:"id" yaml:"id"`
	// Name is the human-facing label.
	Name string `json:"name" yaml:"name"`
	// Kind is the host's classification, such as service or system. The engine
	// does not constrain the vocabulary.
	Kind string `json:"kind" yaml:"kind"`
	// Surface is an optional host-internal grouping. It is never published.
	Surface string `json:"surface,omitempty" yaml:"surface,omitempty"`
	// Status is an optional lifecycle label such as shipped or experimental.
	Status string `json:"status,omitempty" yaml:"status,omitempty"`
	// Description is prose. A markdown provider fills it from the body when the
	// frontmatter omits it. Publication truncates and scans it into Summary.
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// Entrypoint locates the entity in the host's own tree. It is never
	// published, because it discloses a filesystem or command layout.
	Entrypoint string `json:"entrypoint,omitempty" yaml:"entrypoint,omitempty"`
	// Owner attributes the entity inside the host. It is never published.
	Owner string `json:"owner,omitempty" yaml:"owner,omitempty"`
	// Tags are free-form labels. They are normalized, deduped, and sorted, and
	// they drive the projection include and exclude filters.
	Tags []string `json:"tags,omitempty" yaml:"tags,omitempty"`
	// Refs are typed relationships to other entities. Targets need not exist;
	// Validate reports the dangling ones.
	Refs []Ref `json:"refs,omitempty" yaml:"refs,omitempty"`
	// PublicURL is the entity's canonical public link. Projecting one requires a
	// non-empty ProjectionPolicy.AllowHosts.
	PublicURL string `json:"public_url,omitempty" yaml:"public_url,omitempty"`
	// Visibility governs publication. Only VisibilityPublic is projectable.
	Visibility Visibility `json:"visibility,omitempty" yaml:"visibility,omitempty"`
	// Annotations carry arbitrary host data. They are never published, and the
	// closed public DTO is structurally unable to represent them.
	Annotations map[string]string `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	_           struct{}
}

// Ref is a typed relationship from one entity to another.
type Ref struct {
	// Kind names the relationship, such as contains or depends_on.
	Kind string `json:"kind" yaml:"kind"`
	// Target is the id of the referenced entity.
	Target string `json:"target" yaml:"target"`
	_      struct{}
}

// Record binds a portable entity to source provenance. Source paths never
// appear in public projections.
type Record struct {
	// Entity is the normalized portable record.
	Entity Entity `json:"entity"`
	// Provider names the provider that supplied it.
	Provider string `json:"provider"`
	// Source locates the record within that provider, such as a corpus-relative
	// path. It is provenance for the host and is never published.
	Source string `json:"source"`
	// Digest is the canonical digest of the normalized entity, assigned by the
	// engine during Discover. It changes only when this entity changes.
	Digest string `json:"digest"`
	// SourceDigest is the digest of the whole payload the entity was read from.
	// Entities sharing one multi-entity file share a SourceDigest.
	SourceDigest string `json:"source_digest,omitempty"`
	_            struct{}
}

func normalizeEntity(entity Entity) Entity {
	entity.ID = strings.TrimSpace(entity.ID)
	entity.Name = strings.TrimSpace(entity.Name)
	entity.Kind = strings.TrimSpace(entity.Kind)
	entity.Surface = strings.TrimSpace(entity.Surface)
	entity.Status = strings.TrimSpace(entity.Status)
	entity.Description = strings.TrimSpace(entity.Description)
	entity.Entrypoint = strings.TrimSpace(entity.Entrypoint)
	entity.Owner = strings.TrimSpace(entity.Owner)
	entity.PublicURL = strings.TrimSpace(entity.PublicURL)
	entity.Visibility = Visibility(strings.TrimSpace(string(entity.Visibility)))
	entity.Tags = normalizeStrings(entity.Tags)
	// Refs and Annotations are reference types, so the struct copy above still
	// shares their backing storage with the provider's own data. Trimming and
	// sorting in place would silently rewrite and reorder the caller's slice.
	if len(entity.Refs) > 0 {
		refs := make([]Ref, len(entity.Refs))
		copy(refs, entity.Refs)
		entity.Refs = refs
	}
	if len(entity.Annotations) > 0 {
		annotations := make(map[string]string, len(entity.Annotations))
		for key, value := range entity.Annotations {
			annotations[key] = value
		}
		entity.Annotations = annotations
	}
	for i := range entity.Refs {
		entity.Refs[i].Kind = strings.TrimSpace(entity.Refs[i].Kind)
		entity.Refs[i].Target = strings.TrimSpace(entity.Refs[i].Target)
	}
	sort.Slice(entity.Refs, func(i, j int) bool {
		if entity.Refs[i].Kind == entity.Refs[j].Kind {
			return entity.Refs[i].Target < entity.Refs[j].Target
		}
		return entity.Refs[i].Kind < entity.Refs[j].Kind
	})
	return entity
}

func validateEntity(entity Entity) error {
	if err := ValidateEntityID(entity.ID); err != nil {
		return err
	}
	if entity.Name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidEntity)
	}
	if entity.Kind == "" {
		return fmt.Errorf("%w: kind is required", ErrInvalidEntity)
	}
	if !entity.Visibility.Valid() {
		return fmt.Errorf("%w: unknown visibility %q", ErrInvalidEntity, entity.Visibility)
	}
	for _, ref := range entity.Refs {
		if ref.Kind == "" || ref.Target == "" {
			return fmt.Errorf("%w: references require kind and target", ErrInvalidEntity)
		}
	}
	return nil
}

func normalizeStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
