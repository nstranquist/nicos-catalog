// Package catalog is a typed, local-first software-catalog engine.
//
// A host injects its own filesystem boundaries and providers; the engine
// supplies deterministic validation, indexing, BM25 full-text search, graph
// compilation, drift and reconcile gates, and a closed privacy-safe
// publication DTO. Nothing about a repository layout, home directory, or
// business model is assumed.
//
// # Pipeline
//
// Data moves through five stages, each of which is separately testable:
//
//	authored facts        files, APIs, or embedded fixtures
//	  → provider records  Provider.Provide
//	  → normalized set    Engine.Discover — trimmed, validated, ordered, deduped
//	  → derived index     Engine.Reindex — byte-deterministic, cached
//	  → public projection ProjectPublic — closed DTO, safe to publish
//
// # Host contract
//
// A host supplies a Layout and one or more Providers:
//
//	layout, err := catalog.DefaultLayout(root).Resolve(root)
//	engine, err := catalog.New(layout, catalog.WithProviders(myProvider))
//	report, err := engine.Reindex(ctx)
//	results, err := engine.Search(ctx, "ownership graph", catalog.SearchOptions{Limit: 5})
//
// Provider is deliberately small: a name and a Provide method. FilesystemProvider
// reads Markdown with YAML frontmatter, YAML, and JSON. StaticProvider serves
// embedded fixtures and API-backed hosts. Provider output is normalized and
// ordered by the engine, and duplicate ids fail closed across provider
// boundaries rather than shadowing one another.
//
// # Privacy boundary
//
// ProjectPublic produces a PublicEntity, which is a closed DTO: it is
// structurally incapable of representing source paths, host annotations, owner
// telemetry, sidecar data, valuation, or query text. That is a property of the
// type, not of a filter — the field set is frozen by a reflection test that
// also rejects maps, interfaces, pointers, and embedded structs anywhere in the
// reachable type graph.
//
// Publication should consume this DTO rather than filtering a private index
// after serialization. Two rules are easy to miss:
//
//   - ProjectionPolicy.AllowHosts must be non-empty whenever any projected
//     entity declares a PublicURL. An empty allowlist is a hard error, not an
//     implicit permit.
//   - Rejections never reproduce the value that was rejected. A PolicyError
//     names the entity, the field, and the rule, because the error itself
//     travels to logs and CI output.
//
// Hosts building their own publication gates should call ScanPublicText rather
// than reimplementing the patterns, so host and library cannot drift apart.
//
// # Determinism
//
// The index omits wall-clock time and sorts every collection, so identical
// inputs produce byte-identical output. Hosts are expected to depend on this:
// the usual pattern is to commit a generated artifact and fail a build when a
// fresh compile does not match it. Search scores are relative within one result
// set and are not comparable across queries or engines.
//
// # Errors
//
// Sentinels are matched with errors.Is; structured detail is recovered with
// errors.As:
//
//	if errors.Is(err, catalog.ErrIndexMissing) { /* run reindex */ }
//
//	var policy *catalog.PolicyError
//	if errors.As(err, &policy) {
//	    log.Printf("entity %s field %s violated %s", policy.EntityID, policy.Field, policy.Rule)
//	}
//
// # Stability
//
// The Go API follows SemVer. SchemaVersion is a separate contract governing the
// on-disk index; when it advances, Drift reports index_schema_mismatch so an
// upgrade prompts a reindex rather than failing. See docs/api-stability.md.
package catalog
