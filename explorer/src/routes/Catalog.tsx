import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { Entity, SearchHit } from '../generated/contract'
import { DossierDrawer } from '../components/DossierDrawer'
import { EntityKind, EntityStatus } from '../components/EntityBits'
import { QueryState, StatePanel } from '../components/StatePanel'
import { useSource } from '../lib/source-context'
import { invalidCatalogParams } from '../url-state'
import { readSelection, writeSelection } from '../selection-store'

const route = getRouteApi('/catalog')

interface ViewResult {
  items: Entity[]
  hits?: Map<string, SearchHit>
  total: number
  truncated: boolean
  nextCursor?: string
}

export default function Catalog() {
  const source = useSource()
  const search = route.useSearch()
  const navigate = useNavigate({ from: '/catalog' })
  const restoredSelection = useRef(false)
  const [draft, setDraft] = useState(search.q ?? '')
  const invalid = useMemo(() => invalidCatalogParams(new URLSearchParams(location.search)), [search])
  const query = useQuery({
    queryKey: ['catalog', source.sourceDigest, search],
    queryFn: async (): Promise<ViewResult> => {
      if (search.q) {
        const result = await source.search({ ...search, limit: 50 })
        return { items: result.data.items.map((hit) => hit.entity), hits: new Map(result.data.items.map((hit) => [hit.entity.id, hit])), total: result.meta.total ?? result.data.items.length, truncated: result.meta.truncated }
      }
      const result = await source.entities({ ...search, limit: 50 })
      return { items: result.data.items, total: result.meta.total ?? result.data.items.length, truncated: result.meta.truncated, nextCursor: result.meta.next_cursor }
    },
  })

  useEffect(() => setDraft(search.q ?? ''), [search.q])
  useEffect(() => {
    if (restoredSelection.current) return
    restoredSelection.current = true
    if (search.selected) return
    const previous = readSelection()
    if (previous) void navigate({ search: (old) => ({ ...old, selected: previous }), replace: true })
  }, [navigate, search.selected])

  const closeDrawer = useCallback(() => {
    writeSelection(undefined)
    void navigate({ search: (old) => ({ ...old, selected: undefined }), replace: true })
  }, [navigate])

  function apply(event: FormEvent) {
    event.preventDefault()
    const q = draft.trim()
    void navigate({ search: (old) => ({ ...old, q: q || undefined, cursor: undefined }) })
  }

  function setFilter(key: 'kind' | 'status' | 'surface' | 'tag' | 'sort' | 'direction', value: string) {
    void navigate({ search: (old) => ({ ...old, [key]: value || undefined, cursor: undefined }) })
  }

  return (
    <main className="page" id="main-content" data-test="catalog" tabIndex={-1}>
      <header className="page-header compact-header">
        <p className="eyebrow">Catalog and search</p>
        <h1>Find the exact thing.</h1>
        <p>Search ranks projected text only. Filters and route state stay in the URL so you can share the same local view.</p>
      </header>
      {invalid.length > 0 && <StatePanel kind="notice" title="Some URL options were ignored" detail={`Unsupported or invalid options: ${invalid.join(', ')}.`} />}
      <form className="catalog-controls" onSubmit={apply} role="search" aria-label="Catalog results search">
        <div className="catalog-query"><label htmlFor="catalog-query">Search entities</label><input id="catalog-query" value={draft} onChange={(event) => setDraft(event.target.value)} maxLength={256} placeholder="Try: developer platform" /><button type="submit">Search</button></div>
        <div className="filter-grid">
          <Filter label="Kind" value={search.kind} onChange={(value) => setFilter('kind', value)} />
          <Filter label="Status" value={search.status} onChange={(value) => setFilter('status', value)} />
          <Filter label="Surface" value={search.surface} onChange={(value) => setFilter('surface', value)} />
          <Filter label="Tag" value={search.tag} onChange={(value) => setFilter('tag', value)} />
          <label>Sort<select value={search.sort ?? 'name'} onChange={(event) => setFilter('sort', event.target.value)}><option value="name">Name</option><option value="kind">Kind</option><option value="status">Status</option><option value="id">ID</option></select></label>
          <label>Direction<select value={search.direction ?? 'asc'} onChange={(event) => setFilter('direction', event.target.value)}><option value="asc">Ascending</option><option value="desc">Descending</option></select></label>
        </div>
        <button className="clear-button" type="button" onClick={() => void navigate({ search: {} })}>Clear filters</button>
      </form>

      <QueryState query={query} empty={(result) => result.items.length === 0}>{(result, refreshing) => (
        <section aria-labelledby="results-heading" aria-busy={refreshing}>
          <div className="section-heading-row"><h2 id="results-heading">{search.q ? 'Ranked results' : 'Catalog entities'}</h2><span>{result.total.toLocaleString()} matching</span></div>
          <div className="table-wrap">
            <table className="catalog-table">
              <thead><tr><th scope="col">Entity</th><th scope="col">Kind</th><th scope="col">Status</th><th scope="col">Surface</th><th scope="col">Match</th><th scope="col"><span className="sr-only">Open</span></th></tr></thead>
              <tbody>{result.items.map((entity) => {
                const hit = result.hits?.get(entity.id)
                return <tr key={entity.id} data-test="entity-row"><td><strong>{entity.name}</strong><code>{entity.id}</code></td><td><EntityKind value={entity.kind} /></td><td><EntityStatus value={entity.status} /></td><td>{entity.surface || <span className="quiet">—</span>}</td><td>{hit ? hit.matched_terms.join(', ') : <span className="quiet">—</span>}</td><td><button type="button" data-test="open-dossier" data-entity-id={entity.id} onClick={() => void navigate({ search: (old) => ({ ...old, selected: entity.id }) })} aria-label={`Open dossier for ${entity.name}`}>Open</button></td></tr>
              })}</tbody>
            </table>
          </div>
          {result.truncated && result.nextCursor && <div className="pagination"><span>Showing a bounded page.</span><button type="button" onClick={() => void navigate({ search: (old) => ({ ...old, cursor: result.nextCursor }) })}>Next page</button></div>}
          {result.truncated && !result.nextCursor && <p className="notice-copy">Search results are bounded. Add a more specific term or filter.</p>}
        </section>
      )}</QueryState>
      {search.selected && <DossierDrawer entityID={search.selected} onClose={closeDrawer} />}
    </main>
  )
}

function Filter({ label, value, onChange }: { label: string; value?: string; onChange(value: string): void }) {
  const id = `filter-${label.toLocaleLowerCase()}`
  return <label htmlFor={id}>{label}<input id={id} value={value ?? ''} onChange={(event) => onChange(event.target.value)} maxLength={128} placeholder="Any" /></label>
}
