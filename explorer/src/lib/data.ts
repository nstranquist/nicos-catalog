import type {
  Dossier,
  Edge,
  Entity,
  EntityPage,
  Envelope,
  GraphGroup,
  GraphPage,
  HealthReport,
  HealthSeverity,
  Manifest,
  Meta,
  ProjectionMode,
  SearchHit,
  SearchPage,
  StaticCatalog,
  Status,
} from '../generated/contract'
import { aggregateGraph, neighborhoodGraph, regionGraph } from './graph'

export type Fetcher = (input: RequestInfo | URL, init?: RequestInit) => Promise<Response>

export interface CatalogQuery {
  q?: string
  kind?: string
  status?: string
  surface?: string
  tag?: string
  sort?: 'name' | 'kind' | 'status' | 'id'
  direction?: 'asc' | 'desc'
  cursor?: string
  limit?: number
}

export interface GraphQuery {
  mode?: 'aggregate' | 'region' | 'neighborhood'
  groupBy?: GraphGroup
  group?: string
  id?: string
  depth?: 1 | 2
}

export interface Result<T> {
  data: T
  meta: Meta
}

export interface ExplorerSource {
  kind: 'live' | 'static'
  projection: ProjectionMode
  sourceDigest: string
  status(): Promise<Status>
  entities(query: CatalogQuery): Promise<Result<EntityPage>>
  search(query: CatalogQuery): Promise<Result<SearchPage>>
  dossier(id: string): Promise<Result<Dossier>>
  graph(query: GraphQuery): Promise<Result<GraphPage>>
  health(severity?: HealthSeverity): Promise<Result<HealthReport>>
}

export class ExplorerDataError extends Error {
  constructor(public code: string, message: string, public status?: number) {
    super(message)
    this.name = 'ExplorerDataError'
  }
}

export async function discoverSource(
  fetcher: Fetcher = (input, init) => globalThis.fetch(input, init),
  preference: 'auto' | 'static' = 'auto',
): Promise<ExplorerSource> {
  if (preference === 'static') return discoverStaticSource(fetcher)
  try {
    const response = await fetcher('/api/v1/status', { headers: { Accept: 'application/json' } })
    const payload = await response.json() as unknown
    const envelope = parseEnvelope<Status>(payload)
    if (!response.ok || !envelope.ok || !isStatus(envelope.data)) throw envelopeError(envelope, response.status)
    return new LiveSource(fetcher, envelope.projection_mode, envelope.source_digest, envelope.data)
  } catch {
    return discoverStaticSource(fetcher)
  }
}

async function discoverStaticSource(fetcher: Fetcher): Promise<ExplorerSource> {
  const response = await fetcher('/data/manifest.json', { headers: { Accept: 'application/json' } })
  if (!response.ok) throw new ExplorerDataError('discovery_failed', 'Explorer could not find a live API or a static catalog.', response.status)
  const manifest = await response.json() as unknown
  if (!isManifest(manifest)) throw new ExplorerDataError('contract_mismatch', 'The static Explorer manifest is not compatible with this application.')
  return new StaticSource(fetcher, manifest)
}

class LiveSource implements ExplorerSource {
  readonly kind = 'live' as const

  constructor(
    private fetcher: Fetcher,
    public projection: ProjectionMode,
    public sourceDigest: string,
    private initialStatus: Status,
  ) {}

  async status(): Promise<Status> { return this.initialStatus }

  entities(query: CatalogQuery): Promise<Result<EntityPage>> {
    return this.get('/api/v1/entities', queryParams(query))
  }

  search(query: CatalogQuery): Promise<Result<SearchPage>> {
    return this.get('/api/v1/search', queryParams(query))
  }

  dossier(id: string): Promise<Result<Dossier>> {
    return this.get(`/api/v1/entities/${encodeURIComponent(id)}`)
  }

  graph(query: GraphQuery): Promise<Result<GraphPage>> {
    const params = new URLSearchParams()
    if (query.mode) params.set('mode', query.mode)
    if (query.groupBy) params.set('group_by', query.groupBy)
    if (query.group) params.set('group', query.group)
    if (query.id) params.set('id', query.id)
    if (query.depth) params.set('depth', String(query.depth))
    return this.get('/api/v1/graph', params)
  }

  health(severity?: HealthSeverity): Promise<Result<HealthReport>> {
    const params = new URLSearchParams({ limit: '100' })
    if (severity) params.set('severity', severity)
    return this.get('/api/v1/health', params)
  }

  private async get<T>(path: string, params?: URLSearchParams): Promise<Result<T>> {
    const url = params && params.size ? `${path}?${params}` : path
    const response = await this.fetcher(url, { headers: { Accept: 'application/json' } })
    const payload = await response.json() as unknown
    const envelope = parseEnvelope<T>(payload)
    if (!response.ok || !envelope.ok || envelope.data === undefined) throw envelopeError(envelope, response.status)
    if (envelope.source_digest !== this.sourceDigest) throw new ExplorerDataError('source_changed', 'The catalog changed. Reload Explorer to use one consistent source version.')
    return { data: envelope.data, meta: envelope.meta }
  }
}

class StaticSource implements ExplorerSource {
  readonly kind = 'static' as const
  readonly projection = 'public' as const
  readonly sourceDigest: string
  private cache = new Map<string, Promise<unknown>>()

  constructor(private fetcher: Fetcher, private manifest: Manifest) {
    this.sourceDigest = manifest.source_digest
  }

  async status(): Promise<Status> {
    return {
      product_version: this.manifest.product_version,
      api_schema_version: this.manifest.schema_version,
      entity_count: this.manifest.entity_count,
      edge_count: this.manifest.edge_count,
      finding_count: this.manifest.finding_count,
    }
  }

  async entities(query: CatalogQuery): Promise<Result<EntityPage>> {
    const page = await this.searchAsset()
    const filtered = filterEntities(page.items, query)
    return paginate(filtered, query.limit ?? 50, query.cursor)
  }

  async search(query: CatalogQuery): Promise<Result<SearchPage>> {
    const page = await this.searchAsset()
    const entities = filterEntities(page.items, { ...query, q: undefined })
    const hits = rankEntities(entities, query.q ?? '').slice(0, clamp(query.limit ?? 20, 1, 100))
    return { data: { items: hits }, meta: { truncated: hits.length < entities.length && hits.length > 0, total: hits.length } }
  }

  async dossier(id: string): Promise<Result<Dossier>> {
    const catalog = await this.catalogAsset()
    const entity = catalog.items.find((item) => item.id === id)
    if (!entity) throw new ExplorerDataError('not_found', 'The requested entity was not found.', 404)
    const incoming = catalog.edges.filter((edge) => edge.target === id)
    const outgoing = catalog.edges.filter((edge) => edge.source === id)
    const total = incoming.length + outgoing.length
    const boundedOutgoing = outgoing.slice(0, 200)
    const boundedIncoming = incoming.slice(0, Math.max(0, 200 - boundedOutgoing.length))
    return { data: { entity, incoming: boundedIncoming, outgoing: boundedOutgoing }, meta: { total, truncated: total > 200 } }
  }

  async graph(query: GraphQuery): Promise<Result<GraphPage>> {
    const mode = query.mode ?? 'aggregate'
    const groupBy = query.groupBy ?? 'kind'
    if (mode === 'aggregate' && groupBy === 'kind') {
      const graph = await this.load<GraphPage>('graph', this.manifest.content.graph)
      return { data: graph, meta: { truncated: false, total: this.manifest.entity_count } }
    }
    const catalog = await this.catalogAsset()
    let graph: GraphPage
    if (mode === 'aggregate') graph = aggregateGraph(catalog.items, catalog.edges, groupBy)
    else if (mode === 'region' && query.group) graph = regionGraph(catalog.items, catalog.edges, groupBy, query.group)
    else if (mode === 'neighborhood' && query.id) graph = neighborhoodGraph(catalog.items, catalog.edges, query.id, query.depth ?? 1)
    else throw new ExplorerDataError('invalid_graph_scope', 'Choose a region or entity before opening this graph.')
    return { data: graph, meta: { truncated: false, total: graph.nodes.length, notice: graph.refinement?.code } }
  }

  async health(severity?: HealthSeverity): Promise<Result<HealthReport>> {
    const report = await this.load<HealthReport>('health', this.manifest.content.health)
    const findings = severity ? report.findings.filter((finding) => finding.severity === severity) : report.findings
    return { data: { ...report, findings }, meta: { truncated: false, total: findings.length } }
  }

  private searchAsset(): Promise<EntityPage> {
    return this.load<EntityPage>('search', this.manifest.content.search)
  }

  private catalogAsset(): Promise<StaticCatalog> {
    return this.load<StaticCatalog>('entities', this.manifest.content.entities)
  }

  private load<T>(name: 'entities' | 'graph' | 'health' | 'search', expected: string): Promise<T> {
    let request = this.cache.get(name)
    if (!request) {
      request = loadStaticEnvelope<T>(this.fetcher, name, expected, this.sourceDigest)
      this.cache.set(name, request)
    }
    return request as Promise<T>
  }
}

async function loadStaticEnvelope<T>(fetcher: Fetcher, name: string, expected: string, sourceDigest: string): Promise<T> {
  const response = await fetcher(`/data/${name}.json`, { headers: { Accept: 'application/json' } })
  if (!response.ok) throw new ExplorerDataError('asset_missing', `The static ${name} asset is missing.`, response.status)
  const bytes = new Uint8Array(await response.arrayBuffer())
  const actual = await digest(bytes)
  if (actual !== expected) throw new ExplorerDataError('asset_digest_mismatch', `The static ${name} asset did not match its manifest digest.`)
  const payload = JSON.parse(new TextDecoder().decode(bytes)) as unknown
  const envelope = parseEnvelope<T>(payload)
  if (!envelope.ok || envelope.data === undefined || envelope.projection_mode !== 'public' || envelope.source_digest !== sourceDigest) {
    throw new ExplorerDataError('contract_mismatch', `The static ${name} asset is not compatible with its manifest.`)
  }
  return envelope.data
}

async function digest(bytes: Uint8Array): Promise<string> {
  const hash = await crypto.subtle.digest('SHA-256', bytes as Uint8Array<ArrayBuffer>)
  return `sha256:${[...new Uint8Array(hash)].map((part) => part.toString(16).padStart(2, '0')).join('')}`
}

function parseEnvelope<T>(value: unknown): Envelope<T> {
  if (!isRecord(value) || value.schema_version !== 1 || typeof value.ok !== 'boolean' ||
    (value.projection_mode !== 'local' && value.projection_mode !== 'public') || typeof value.source_digest !== 'string' || !isMeta(value.meta)) {
    throw new ExplorerDataError('contract_mismatch', 'Explorer received an incompatible data contract.')
  }
  return value as unknown as Envelope<T>
}

function envelopeError(envelope: Envelope<unknown>, status?: number): ExplorerDataError {
  return new ExplorerDataError(envelope.error?.code ?? 'request_failed', envelope.error?.summary ?? 'Explorer could not complete the request.', status)
}

function isManifest(value: unknown): value is Manifest {
  if (!isRecord(value) || !isRecord(value.content)) return false
  const content = value.content
  return value.schema_version === 1 && value.generator === 'nicos-catalog-explorer' &&
    value.projection_mode === 'public' && typeof value.product_version === 'string' && typeof value.source_digest === 'string' &&
    isNonnegative(value.entity_count) && isNonnegative(value.edge_count) && isNonnegative(value.finding_count) &&
    ['entities', 'graph', 'health', 'search'].every((key) => /^sha256:[a-f0-9]{64}$/.test(String(content[key])))
}

function isStatus(value: unknown): value is Status {
  return isRecord(value) && typeof value.product_version === 'string' && value.api_schema_version === 1 &&
    isNonnegative(value.entity_count) && isNonnegative(value.edge_count) && isNonnegative(value.finding_count)
}

function isMeta(value: unknown): value is Meta {
  return isRecord(value) && typeof value.truncated === 'boolean'
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isNonnegative(value: unknown): value is number {
  return Number.isInteger(value) && Number(value) >= 0
}

function queryParams(query: CatalogQuery): URLSearchParams {
  const params = new URLSearchParams()
  for (const key of ['q', 'kind', 'status', 'surface', 'tag', 'sort', 'direction', 'cursor'] as const) {
    if (query[key]) params.set(key, query[key]!)
  }
  if (query.limit) params.set('limit', String(query.limit))
  return params
}

export function filterEntities(items: Entity[], query: CatalogQuery): Entity[] {
  const exact = (value: string | undefined, filter: string | undefined) => !filter || (value ?? '').toLocaleLowerCase() === filter.toLocaleLowerCase()
  const filtered = items.filter((entity) => {
    if (!exact(entity.kind, query.kind) || !exact(entity.status, query.status) || !exact(entity.surface, query.surface)) return false
    if (query.tag && !(entity.tags ?? []).some((tag) => tag.toLocaleLowerCase() === query.tag!.toLocaleLowerCase())) return false
    if (query.q && rankEntity(entity, tokenize(query.q)).score === 0) return false
    return true
  })
  const sort = query.sort ?? 'name'
  const direction = query.direction === 'desc' ? -1 : 1
  return [...filtered].sort((left, right) => {
    const a = String(left[sort] ?? '').toLocaleLowerCase()
    const b = String(right[sort] ?? '').toLocaleLowerCase()
    return (a.localeCompare(b) || left.id.localeCompare(right.id)) * direction
  })
}

export function rankEntities(items: Entity[], query: string): SearchHit[] {
  const terms = tokenize(query)
  if (terms.length === 0) return []
  return items.map((entity) => rankEntity(entity, terms)).filter((hit) => hit.score > 0).sort((a, b) => b.score - a.score || a.entity.id.localeCompare(b.entity.id))
}

function rankEntity(entity: Entity, terms: string[]): SearchHit {
  const name = entity.name.toLocaleLowerCase()
  const id = entity.id.toLocaleLowerCase()
  const fields = [name, id, entity.kind, entity.status, entity.summary, entity.surface, entity.owner_label, entity.entrypoint_label, ...(entity.tags ?? [])]
    .filter(Boolean).join(' ').toLocaleLowerCase()
  const matched = terms.filter((term) => fields.includes(term))
  if (matched.length !== terms.length) return { entity, score: 0, matched_terms: matched }
  let score = matched.length
  const phrase = terms.join(' ')
  if (name === phrase || id === phrase) score += 6
  for (const term of matched) {
    if (id === term || name === term) score += 4
    else if (id.includes(term) || name.includes(term)) score += 1.5
  }
  return { entity, score, matched_terms: matched }
}

function tokenize(value: string): string[] {
  return [...new Set(value.toLocaleLowerCase().match(/[a-z0-9][a-z0-9._+-]*/g) ?? [])]
}

function paginate(items: Entity[], limit: number, cursor?: string): Result<EntityPage> {
  const safeLimit = clamp(limit, 1, 100)
  const offset = decodeStaticCursor(cursor)
  if (offset > items.length) throw new ExplorerDataError('invalid_cursor', 'The cursor is invalid for this catalog version.')
  const end = Math.min(offset + safeLimit, items.length)
  return {
    data: { items: items.slice(offset, end) },
    meta: { total: items.length, truncated: end < items.length, next_cursor: end < items.length ? encodeStaticCursor(end) : undefined },
  }
}

function encodeStaticCursor(offset: number): string {
  return `static-${offset.toString(36)}`
}

function decodeStaticCursor(cursor?: string): number {
  if (!cursor) return 0
  if (!/^static-[0-9a-z]+$/.test(cursor)) throw new ExplorerDataError('invalid_cursor', 'The cursor is invalid for this catalog version.')
  const value = Number.parseInt(cursor.slice(7), 36)
  if (!Number.isSafeInteger(value) || value < 0) throw new ExplorerDataError('invalid_cursor', 'The cursor is invalid for this catalog version.')
  return value
}

function clamp(value: number, minimum: number, maximum: number): number {
  return Math.max(minimum, Math.min(maximum, value))
}
