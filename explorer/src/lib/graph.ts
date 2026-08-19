import type { Edge, Entity, GraphGroup, GraphPage } from '../generated/contract'

export const MAX_GRAPH_NODES = 500
export const MAX_GRAPH_EDGES = 1500

function groupValue(entity: Entity, groupBy: GraphGroup): string {
  const value = groupBy === 'surface' ? entity.surface : entity.kind
  return value?.trim() || 'unassigned'
}

function aggregateID(value: string): string {
  return `group:${encodeURIComponent(value)}`
}

export function aggregateGraph(entities: Entity[], edges: Edge[], groupBy: GraphGroup): GraphPage {
  const groups = new Map<string, number>()
  const byID = new Map<string, string>()
  for (const entity of entities) {
    const group = groupValue(entity, groupBy)
    groups.set(group, (groups.get(group) ?? 0) + 1)
    byID.set(entity.id, group)
  }
  const nodes = [...groups].sort(([a], [b]) => a.localeCompare(b)).map(([name, count]) => ({
    id: aggregateID(name), name, group: name, count, aggregate: true,
  }))
  const counts = new Map<string, { source: string; target: string; kind: string; count: number }>()
  for (const edge of edges) {
    const source = byID.get(edge.source)
    const target = byID.get(edge.target)
    if (!source || !target) continue
    const key = `${source}\u0000${target}\u0000${edge.kind}`
    const current = counts.get(key)
    if (current) current.count += 1
    else counts.set(key, { source: aggregateID(source), target: aggregateID(target), kind: edge.kind, count: 1 })
  }
  return {
    mode: 'aggregate',
    group_by: groupBy,
    nodes,
    edges: [...counts.values()].sort((a, b) => `${a.source}\u0000${a.target}\u0000${a.kind}`.localeCompare(`${b.source}\u0000${b.target}\u0000${b.kind}`)),
  }
}

export function regionGraph(entities: Entity[], edges: Edge[], groupBy: GraphGroup, group: string): GraphPage {
  const selected = entities.filter((entity) => groupValue(entity, groupBy) === group)
  if (selected.length > MAX_GRAPH_NODES) {
    const aggregate = aggregateGraph(entities, edges, groupBy)
    return { ...aggregate, mode: 'region', scope: group, refinement: { code: 'refinement_required', summary: 'Choose an entity to open a bounded neighborhood.' } }
  }
  const ids = new Set(selected.map((entity) => entity.id))
  const selectedEdges = edges.filter((edge) => ids.has(edge.source) && ids.has(edge.target))
  if (selectedEdges.length > MAX_GRAPH_EDGES) {
    return { mode: 'region', group_by: groupBy, scope: group, nodes: [], edges: [], refinement: { code: 'refinement_required', summary: 'This region has too many relationships. Choose an entity.' } }
  }
  return {
    mode: 'region', group_by: groupBy, scope: group,
    nodes: selected.map((entity) => ({ id: entity.id, name: entity.name, kind: entity.kind, status: entity.status })),
    edges: selectedEdges.map((edge) => ({ ...edge })),
  }
}

export function neighborhoodGraph(entities: Entity[], edges: Edge[], root: string, depth: 1 | 2): GraphPage {
  const byID = new Map(entities.map((entity) => [entity.id, entity]))
  if (!byID.has(root)) throw new Error('The requested entity was not found.')
  const selected = new Set([root])
  let frontier = new Set([root])
  for (let level = 0; level < depth; level += 1) {
    const next = new Set<string>()
    for (const edge of edges) {
      if (frontier.has(edge.source) && byID.has(edge.target)) next.add(edge.target)
      if (frontier.has(edge.target) && byID.has(edge.source)) next.add(edge.source)
    }
    for (const value of next) selected.add(value)
    frontier = next
    if (selected.size > MAX_GRAPH_NODES) {
      return { mode: 'neighborhood', scope: root, depth, nodes: [], edges: [], refinement: { code: 'refinement_required', summary: 'Use depth one or choose a more specific entity.' } }
    }
  }
  const selectedEdges = edges.filter((edge) => selected.has(edge.source) && selected.has(edge.target))
  if (selectedEdges.length > MAX_GRAPH_EDGES) {
    return { mode: 'neighborhood', scope: root, depth, nodes: [], edges: [], refinement: { code: 'refinement_required', summary: 'Use depth one or choose a more specific entity.' } }
  }
  const nodes = [...selected].sort().map((id) => byID.get(id)!).map((entity) => ({ id: entity.id, name: entity.name, kind: entity.kind, status: entity.status }))
  return { mode: 'neighborhood', scope: root, depth, nodes, edges: selectedEdges.map((edge) => ({ ...edge })) }
}
