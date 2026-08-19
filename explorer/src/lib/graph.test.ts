import { describe, expect, it } from 'vitest'
import type { Edge, Entity } from '../generated/contract'
import { aggregateGraph, MAX_GRAPH_EDGES, MAX_GRAPH_NODES, neighborhoodGraph, regionGraph } from './graph'

const entities: Entity[] = [
  { id: 'service.api', name: 'API', kind: 'service', surface: 'runtime', status: 'active' },
  { id: 'repo.api', name: 'API repository', kind: 'repository', surface: 'source', status: 'active' },
  { id: 'service.web', name: 'Web', kind: 'service', surface: 'runtime' },
]
const edges: Edge[] = [
  { source: 'repo.api', target: 'service.api', kind: 'builds' },
  { source: 'service.web', target: 'service.api', kind: 'calls' },
]

describe('progressive graph', () => {
  it('aggregates deterministically and ignores edges with unknown endpoints', () => {
    const graph = aggregateGraph(entities, [...edges, { source: 'missing', target: 'service.api', kind: 'bad' }], 'kind')
    expect(graph.nodes.map((node) => [node.name, node.count])).toEqual([['repository', 1], ['service', 2]])
    expect(graph.edges).toHaveLength(2)
    expect(graph.edges[0].source.localeCompare(graph.edges[1].source)).toBeLessThanOrEqual(0)
  })

  it('builds a region and returns refinement when node or edge caps are crossed', () => {
    const region = regionGraph(entities, edges, 'surface', 'runtime')
    expect(region.nodes).toHaveLength(2)
    expect(region.edges).toHaveLength(1)

    const many = Array.from({ length: MAX_GRAPH_NODES + 1 }, (_, index) => ({ id: `service.item-${index}`, name: `Item ${index}`, kind: 'service' }))
    expect(regionGraph(many, [], 'kind', 'service').refinement?.code).toBe('refinement_required')

    const denseEntities = Array.from({ length: 80 }, (_, index) => ({ id: `service.dense-${index}`, name: `Dense ${index}`, kind: 'service' }))
    const denseEdges = Array.from({ length: MAX_GRAPH_EDGES + 1 }, (_, index) => ({ source: denseEntities[index % 80].id, target: denseEntities[(index + 1) % 80].id, kind: `edge-${index}` }))
    expect(regionGraph(denseEntities, denseEdges, 'kind', 'service').nodes).toHaveLength(0)
  })

  it('builds one- and two-hop neighborhoods and fails closed for invalid scope', () => {
    expect(neighborhoodGraph(entities, edges, 'service.api', 1).nodes.map((node) => node.id)).toEqual(['repo.api', 'service.api', 'service.web'])
    expect(() => neighborhoodGraph(entities, edges, 'missing', 1)).toThrow('not found')

    const many = Array.from({ length: MAX_GRAPH_NODES + 1 }, (_, index) => ({ id: `service.item-${index}`, name: `Item ${index}`, kind: 'service' }))
    const star = many.slice(1).map((entity) => ({ source: many[0].id, target: entity.id, kind: 'calls' }))
    expect(neighborhoodGraph(many, star, many[0].id, 1).refinement).toBeDefined()

    const dense = Array.from({ length: 60 }, (_, index) => ({ id: `service.node-${index}`, name: `Node ${index}`, kind: 'service' }))
    const denseEdges = Array.from({ length: MAX_GRAPH_EDGES + 1 }, (_, index) => ({ source: dense[index % 60].id, target: dense[Math.floor(index / 60) % 60].id, kind: `edge-${index}` }))
    expect(neighborhoodGraph(dense, denseEdges, dense[0].id, 2).refinement).toBeDefined()
  })
})
