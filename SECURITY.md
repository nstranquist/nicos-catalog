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

Report vulnerabilities privately through the repository security advisory
flow. Do not include secrets or private catalog data in a public issue.
