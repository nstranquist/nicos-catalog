# Security and privacy

Nicos Catalog is local-first and does not perform network requests. Providers
are host code and should be reviewed with the same care as any filesystem or API
integration.

For public output, use `ProjectPublic` or the `project` command. Do not publish
the private index: it may contain entrypoints, owners, annotations, internal
descriptions, and host-specific identifiers. The public DTO intentionally cannot
represent those fields.

Before publishing a projection:

- require an explicit public visibility value;
- allowlist URL hosts;
- use synthetic fixtures in examples and tests;
- scan the resulting artifact for absolute paths, credentials, query text, and
  organization-confidential identifiers;
- keep deployment and repository publication as explicit operator actions.

## Guarantees the engine makes

**Rejections never echo the rejected value.** When publication refuses a field,
the returned `*PolicyError` names the entity, the field, and the rule — never
the offending text. These errors reach stderr, CI logs, and host error paths, so
reproducing a rejected credential or path there would defeat the boundary the
projection exists to enforce. Hosts wrapping these errors should preserve that
property.

**The corpus boundary is not followed outward.** `FilesystemProvider` refuses
symlinked corpus files rather than resolving them. A link pointing at a file
outside the corpus resolves to content that decodes into perfectly valid
entities, which would otherwise be indistinguishable from authored input. In
strict mode this is `ErrCorpusEscape`; in lax mode the file is skipped.

**The publication DTO is closed by construction.** `PublicEntity` cannot
represent a source path, an owner, an annotation, or any host field, because it
has no map, interface, pointer, or embedded member anywhere in its reachable
type graph. That is asserted structurally by tests rather than by a value
denylist over a fixture, so adding a leaking field fails the build.

**The engine logs no content.** A logger supplied through `WithLogger` receives
counts, durations, provider names, entity ids, and corpus-relative paths only —
never descriptions, annotations, public URLs, owners, or entrypoints.

**A URL allowlist is required, not implied.** Projecting any entity that
declares a `PublicURL` with an empty `AllowHosts` is a hard error. Forgetting to
configure the allowlist cannot publish an unreviewed link.

## Adding a field to the public DTO

Treat this as a privacy review, not a routine change. It means new data leaves
the host. The change must update the frozen golden field list, the golden JSON
artifact, and this document in the same commit; the tests fail until it does.

Report vulnerabilities privately through the repository security advisory
flow. Do not include secrets or private catalog data in a public issue.
