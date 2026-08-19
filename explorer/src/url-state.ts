export type SortKey = 'name' | 'kind' | 'status' | 'id'
export type Direction = 'asc' | 'desc'

export interface CatalogSearchState {
  q?: string
  kind?: string
  status?: string
  surface?: string
  tag?: string
  sort?: SortKey
  direction?: Direction
  cursor?: string
  selected?: string
}

export interface GraphSearchState {
  mode?: 'aggregate' | 'region' | 'neighborhood'
  group_by?: 'kind' | 'surface'
  group?: string
  id?: string
  depth?: 1 | 2
}

const entityID = /^[a-z0-9](?:[a-z0-9._-]{0,126}[a-z0-9])?$/
const scalarLimit = 128

function scalar(value: unknown, max = scalarLimit): string | undefined {
  if (Array.isArray(value)) value = value[0]
  if (typeof value !== 'string') return undefined
  const trimmed = value.trim()
  return trimmed && trimmed.length <= max ? trimmed : undefined
}

function member<T extends string>(value: unknown, allowed: readonly T[]): T | undefined {
  const parsed = scalar(value)
  return parsed && allowed.includes(parsed as T) ? parsed as T : undefined
}

function id(value: unknown): string | undefined {
  const parsed = scalar(value)
  return parsed && entityID.test(parsed) ? parsed : undefined
}

export function parseCatalogSearch(raw: Record<string, unknown>): CatalogSearchState {
  return compact({
    q: scalar(raw.q, 256),
    kind: scalar(raw.kind),
    status: scalar(raw.status),
    surface: scalar(raw.surface),
    tag: scalar(raw.tag),
    sort: member(raw.sort, ['name', 'kind', 'status', 'id'] as const),
    direction: member(raw.direction, ['asc', 'desc'] as const),
    cursor: scalar(raw.cursor, 512),
    selected: id(raw.selected),
  })
}

export function parseGraphSearch(raw: Record<string, unknown>): GraphSearchState {
  const depth = scalar(raw.depth) === '2' || raw.depth === 2 ? 2 : scalar(raw.depth) === '1' || raw.depth === 1 ? 1 : undefined
  return compact({
    mode: member(raw.mode, ['aggregate', 'region', 'neighborhood'] as const),
    group_by: member(raw.group_by, ['kind', 'surface'] as const),
    group: scalar(raw.group),
    id: id(raw.id),
    depth,
  })
}

export function invalidCatalogParams(params: URLSearchParams): string[] {
  const allowed = new Set(['q', 'kind', 'status', 'surface', 'tag', 'sort', 'direction', 'cursor', 'selected'])
  const raw = Object.fromEntries(params)
  const parsed = parseCatalogSearch(raw)
  const invalid = new Set<string>()
  for (const key of params.keys()) {
    if (!allowed.has(key) || params.getAll(key).length > 1) invalid.add(key)
  }
  for (const key of allowed) {
    if (params.has(key) && !(key in parsed)) invalid.add(key)
  }
  return [...invalid].sort()
}

export function invalidGraphParams(params: URLSearchParams): string[] {
  const allowed = new Set(['mode', 'group_by', 'group', 'id', 'depth'])
  const raw = Object.fromEntries(params)
  const parsed = parseGraphSearch(raw)
  const invalid = new Set<string>()
  for (const key of params.keys()) {
    if (!allowed.has(key) || params.getAll(key).length > 1) invalid.add(key)
  }
  for (const key of allowed) {
    if (params.has(key) && !(key in parsed)) invalid.add(key)
  }
  return [...invalid].sort()
}

function compact<T extends object>(value: T): T {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined)) as T
}
