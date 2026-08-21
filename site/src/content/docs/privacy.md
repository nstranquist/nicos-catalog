---
title: Privacy and public DTO
description: Closed PublicEntity publication boundary — what the engine cannot encode.
---

The public core deliberately excludes personal telemetry, business valuation, private query text, runtime credentials, and host-specific portfolio policy. Those stay in host adapters.

`ProjectPublic` produces a closed `PublicEntity` DTO. It cannot encode source paths, annotations, owner fields, telemetry, query text, valuation, or sidecar data. Hosts may further restrict visibility, kinds, tags, URL hosts, and summary length. Publication should consume this DTO rather than filtering the private index after serialization.

Do not publish the private index.

## Guarantees

**Rejections never echo the rejected value.** A `*PolicyError` names the entity, the field, and the rule — never the offending text.

**The corpus boundary is not followed outward.** `FilesystemProvider` refuses symlinked corpus files. In strict mode this is `ErrCorpusEscape`; in lax mode the file is skipped.

**The publication DTO is closed by construction.** `PublicEntity` has no map, interface, pointer, or embedded member anywhere in its reachable type graph. Tests fail the build if a leaking field is added.

**The engine logs no content.** A logger supplied through `WithLogger` receives counts, durations, provider names, entity ids, and corpus-relative paths only.

**A URL allowlist is required, not implied.** Projecting any entity that declares a `PublicURL` with an empty `AllowHosts` is a hard error.

Accepted public URLs use HTTPS, an allowlisted host, no user information, no
at-sign credential delimiter, no query or fragment, and no non-default port.

## Before you publish

- require an explicit public visibility value;
- allowlist URL hosts;
- use synthetic fixtures in examples and tests;
- scan the resulting artifact for absolute paths, credentials, query text, and organization-confidential identifiers;
- keep deployment and repository publication as explicit operator actions.

Adding a field to `PublicEntity` is a privacy decision, not a routine change. It must update the golden field list, golden JSON, and `SECURITY.md` in the same commit.
