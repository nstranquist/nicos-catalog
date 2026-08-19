import { createHash } from 'node:crypto'
import { describe, expect, it, vi } from 'vitest'
import type { Edge, Entity, GraphPage, HealthReport, Manifest, StaticCatalog } from '../generated/contract'
import { discoverSource, ExplorerDataError, filterEntities, rankEntities, type Fetcher } from './data'

const catalogEntities: Entity[] = [
  { id: 'service.api', name: 'Seed API', kind: 'service', status: 'active', surface: 'runtime', tags: ['platform'], summary: 'Developer platform API' },
  { id: 'repository.api', name: 'Seed repository', kind: 'repository', status: 'stable', surface: 'source', tags: ['go'], summary: 'Source for the API' },
  { id: 'document.runbook', name: 'API runbook', kind: 'document', status: 'draft', surface: 'docs', tags: ['platform'], summary: 'Recovery guide' },
]
const catalogEdges: Edge[] = [
  { source: 'repository.api', target: 'service.api', kind: 'builds' },
  { source: 'document.runbook', target: 'service.api', kind: 'documents' },
]
const aggregate: GraphPage = {
  mode: 'aggregate', group_by: 'kind',
  nodes: [{ id: 'group:service', name: 'service', group: 'service', count: 1, aggregate: true }],
  edges: [],
}
const health: HealthReport = {
  ok: false, drift: 'changed', findings: [
    { code: 'missing_summary', severity: 'warning', entity_id: 'service.api', remediation: 'Add a public summary.' },
    { code: 'catalog_ready', severity: 'info', remediation: 'No action is required.' },
  ],
}

describe('projected entity filtering and ranking', () => {
  it('filters every projected facet and sorts in both directions', () => {
    expect(filterEntities(catalogEntities, { kind: 'SERVICE', status: 'active', surface: 'runtime', tag: 'PLATFORM', q: 'developer API' }).map((item) => item.id)).toEqual(['service.api'])
    expect(filterEntities(catalogEntities, { sort: 'id', direction: 'desc' }).map((item) => item.id)).toEqual(['service.api', 'repository.api', 'document.runbook'])
    expect(filterEntities(catalogEntities, { q: 'does-not-exist' })).toHaveLength(0)
  })

  it('ranks exact names and IDs above ordinary projected text matches', () => {
    const ranked = rankEntities(catalogEntities, 'seed api')
    expect(ranked[0].entity.id).toBe('service.api')
    expect(ranked[0].matched_terms).toEqual(['seed', 'api'])
    expect(rankEntities(catalogEntities, ' ')).toEqual([])
  })
})

describe('live Explorer source', () => {
  it('keeps the browser fetch receiver bound for later reads', async () => {
    const nativeLikeFetch = vi.fn(function (this: typeof globalThis, input: RequestInfo | URL) {
      if (this !== globalThis) throw new TypeError('Illegal invocation')
      if (String(input) === '/api/v1/status') return Promise.resolve(jsonResponse(envelope({ product_version: 'v0.3.0', api_schema_version: 1, entity_count: 1, edge_count: 0, finding_count: 0 })))
      return Promise.resolve(jsonResponse(envelope({ items: [catalogEntities[0]] }, { total: 1 })))
    })
    vi.stubGlobal('fetch', nativeLikeFetch)
    try {
      const source = await discoverSource()
      expect((await source.entities({ limit: 1 })).data.items).toHaveLength(1)
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('discovers the API and maps every bounded read', async () => {
    const requests: string[] = []
    const fetcher: Fetcher = vi.fn(async (input) => {
      const url = String(input)
      requests.push(url)
      if (url === '/api/v1/status') return jsonResponse(envelope({ product_version: 'v0.3.0', api_schema_version: 1, entity_count: 3, edge_count: 2, finding_count: 2 }))
      if (url.startsWith('/api/v1/entities?')) return jsonResponse(envelope({ items: [catalogEntities[0]] }, { total: 1 }))
      if (url === '/api/v1/entities/service.api') return jsonResponse(envelope({ entity: catalogEntities[0], incoming: catalogEdges, outgoing: [] }, { total: 2 }))
      if (url.startsWith('/api/v1/search?')) return jsonResponse(envelope({ items: [{ entity: catalogEntities[0], score: 5, matched_terms: ['api'] }] }, { total: 1 }))
      if (url.startsWith('/api/v1/graph?')) return jsonResponse(envelope(aggregate, { total: 3 }))
      if (url.startsWith('/api/v1/health?')) return jsonResponse(envelope(health, { total: 2 }))
      return jsonResponse(envelope(undefined, {}, false, { code: 'not_found', summary: 'Missing' }), 404)
    })

    const source = await discoverSource(fetcher)
    expect(source.kind).toBe('live')
    expect((await source.status()).product_version).toBe('v0.3.0')
    expect((await source.entities({ q: 'api', kind: 'service', status: 'active', surface: 'runtime', tag: 'platform', sort: 'id', direction: 'desc', cursor: 'next', limit: 5 })).data.items).toHaveLength(1)
    expect((await source.search({ q: 'api', limit: 4 })).data.items[0].score).toBe(5)
    expect((await source.dossier('service.api')).meta.total).toBe(2)
    expect((await source.graph({ mode: 'region', groupBy: 'kind', group: 'service', id: 'service.api', depth: 2 })).data.mode).toBe('aggregate')
    expect((await source.health('warning')).data.drift).toBe('changed')
    expect(requests).toContain('/api/v1/entities?q=api&kind=service&status=active&surface=runtime&tag=platform&sort=id&direction=desc&cursor=next&limit=5')
    expect(requests).toContain('/api/v1/graph?mode=region&group_by=kind&group=service&id=service.api&depth=2')
  })

  it('rejects error envelopes and a source digest change', async () => {
    let mode: 'error' | 'changed' = 'error'
    const fetcher: Fetcher = async (input) => {
      if (String(input) === '/api/v1/status') return jsonResponse(envelope({ product_version: 'v0.3.0', api_schema_version: 1, entity_count: 0, edge_count: 0, finding_count: 0 }))
      if (mode === 'error') return jsonResponse(envelope(undefined, {}, false, { code: 'invalid_query', summary: 'Fix the query.' }), 400)
      return jsonResponse({ ...envelope({ items: [] }), source_digest: 'sha256:changed' })
    }
    const source = await discoverSource(fetcher)
    await expect(source.entities({})).rejects.toMatchObject({ code: 'invalid_query', status: 400 })
    mode = 'changed'
    await expect(source.entities({})).rejects.toMatchObject({ code: 'source_changed' })
  })
})

describe('static Explorer source', () => {
  it('uses an explicit static preference without a failed API probe', async () => {
    const fixture = staticFixture()
    const fetcher = staticFetcher(fixture)
    const source = await discoverSource(fetcher, 'static')
    expect(source.kind).toBe('static')
    expect(fetcher).toHaveBeenCalledWith('/data/manifest.json', expect.anything())
    expect(fetcher).not.toHaveBeenCalledWith('/api/v1/status', expect.anything())
  })

  it('verifies assets and implements catalog, search, dossier, graph, and health reads', async () => {
    const fixture = staticFixture()
    const fetcher = staticFetcher(fixture)
    const source = await discoverSource(fetcher)
    expect(source.kind).toBe('static')
    expect((await source.status()).entity_count).toBe(3)

    const first = await source.entities({ sort: 'id', limit: 1 })
    expect(first.data.items).toHaveLength(1)
    expect(first.meta.next_cursor).toMatch(/^static-/)
    const second = await source.entities({ sort: 'id', limit: 1, cursor: first.meta.next_cursor })
    expect(second.data.items[0].id).not.toBe(first.data.items[0].id)
    expect((await source.entities({ kind: 'service', tag: 'platform' })).data.items.map((item) => item.id)).toEqual(['service.api'])

    expect((await source.search({ q: 'seed api', status: 'active', limit: 1 })).data.items[0].entity.id).toBe('service.api')
    expect((await source.dossier('service.api')).data.incoming).toHaveLength(2)
    await expect(source.dossier('missing')).rejects.toMatchObject({ code: 'not_found' })

    expect((await source.graph({})).data).toEqual(aggregate)
    expect((await source.graph({ mode: 'aggregate', groupBy: 'surface' })).data.group_by).toBe('surface')
    expect((await source.graph({ mode: 'region', groupBy: 'kind', group: 'service' })).data.nodes).toHaveLength(1)
    expect((await source.graph({ mode: 'neighborhood', id: 'service.api', depth: 1 })).data.nodes).toHaveLength(3)
    await expect(source.graph({ mode: 'region', groupBy: 'kind' })).rejects.toMatchObject({ code: 'invalid_graph_scope' })

    expect((await source.health('warning')).data.findings).toHaveLength(1)
    expect((await source.health()).data.findings).toHaveLength(2)
    expect(fetcher).toHaveBeenCalled()
  })

  it('fails closed for invalid cursors, missing assets, digest drift, and bad envelopes', async () => {
    const fixture = staticFixture()
    const source = await discoverSource(staticFetcher(fixture))
    await expect(source.entities({ cursor: 'wrong' })).rejects.toMatchObject({ code: 'invalid_cursor' })
    await expect(source.entities({ cursor: 'static-zzzzzzzzzzzzzzzzzzzzzz' })).rejects.toMatchObject({ code: 'invalid_cursor' })

    const missing = await discoverSource(staticFetcher(fixture, { missing: 'entities' }))
    await expect(missing.dossier('service.api')).rejects.toMatchObject({ code: 'asset_missing' })

    const changed = await discoverSource(staticFetcher(fixture, { changed: 'entities' }))
    await expect(changed.dossier('service.api')).rejects.toMatchObject({ code: 'asset_digest_mismatch' })

    const badEnvelope = staticFixture()
    badEnvelope.assets.entities = bytes(JSON.stringify({ hello: 'world' }))
    badEnvelope.manifest.content.entities = sha(badEnvelope.assets.entities)
    const incompatible = await discoverSource(staticFetcher(badEnvelope))
    await expect(incompatible.dossier('service.api')).rejects.toMatchObject({ code: 'contract_mismatch' })
  })

  it('reports discovery failure when neither transport has a compatible root', async () => {
    const absent: Fetcher = async () => new Response('missing', { status: 404 })
    await expect(discoverSource(absent)).rejects.toMatchObject({ code: 'discovery_failed' })

    let calls = 0
    const invalid: Fetcher = async () => calls++ === 0 ? new Response('<html />') : jsonResponse({ schema_version: 2 })
    await expect(discoverSource(invalid)).rejects.toMatchObject({ code: 'contract_mismatch' })
  })
})

function envelope(data: unknown, meta: Record<string, unknown> = {}, ok = true, error?: { code: string; summary: string }) {
  return { schema_version: 1, ok, projection_mode: 'local', source_digest: 'sha256:source', data, error, meta: { truncated: false, ...meta } }
}

function jsonResponse(value: unknown, status = 200): Response {
  return new Response(JSON.stringify(value), { status, headers: { 'Content-Type': 'application/json' } })
}

interface StaticFixture {
  manifest: Manifest
  assets: Record<'entities' | 'graph' | 'health' | 'search', Uint8Array>
}

function staticFixture(): StaticFixture {
  const values = {
    entities: { items: catalogEntities, edges: catalogEdges } satisfies StaticCatalog,
    graph: aggregate,
    health,
    search: { items: catalogEntities },
  }
  const assets = Object.fromEntries(Object.entries(values).map(([name, value]) => [name, bytes(JSON.stringify({ ...envelope(value), projection_mode: 'public' }))])) as StaticFixture['assets']
  const manifest: Manifest = {
    schema_version: 1, generator: 'nicos-catalog-explorer', product_version: 'v0.3.0', projection_mode: 'public', source_digest: 'sha256:source',
    entity_count: 3, edge_count: 2, finding_count: 2,
    content: { entities: sha(assets.entities), graph: sha(assets.graph), health: sha(assets.health), search: sha(assets.search) },
  }
  return { manifest, assets }
}

function staticFetcher(fixture: StaticFixture, option: { missing?: string; changed?: string } = {}): Fetcher & ReturnType<typeof vi.fn> {
  return vi.fn(async (input) => {
    const url = String(input)
    if (url === '/api/v1/status') return new Response('<html />', { headers: { 'Content-Type': 'text/html' } })
    if (url === '/data/manifest.json') return jsonResponse(fixture.manifest)
    const match = url.match(/^\/data\/(entities|graph|health|search)\.json$/)
    if (!match) return new Response('missing', { status: 404 })
    const name = match[1] as keyof StaticFixture['assets']
    if (option.missing === name) return new Response('missing', { status: 404 })
    const body = option.changed === name ? bytes('changed') : fixture.assets[name]
    return new Response(body as BodyInit, { headers: { 'Content-Type': 'application/json' } })
  }) as Fetcher & ReturnType<typeof vi.fn>
}

function bytes(value: string): Uint8Array { return new TextEncoder().encode(value) }
function sha(value: Uint8Array): string { return `sha256:${createHash('sha256').update(value).digest('hex')}` }

void ExplorerDataError
