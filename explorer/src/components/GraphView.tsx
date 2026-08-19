import { Link } from '@tanstack/react-router'
import type { GraphPage } from '../generated/contract'

export function GraphView({ graph }: { graph: GraphPage }) {
  if (graph.refinement) return <section className="refinement-panel"><p className="eyebrow">Refinement required</p><h2>The graph stayed bounded.</h2><p>{graph.refinement.summary}</p></section>
  const points = layout(graph.nodes.length)
  const byID = new Map(graph.nodes.map((node, index) => [node.id, points[index]]))
  return (
    <div className="graph-layout">
      <section className="graph-canvas" aria-labelledby="visual-heading">
        <div className="section-heading-row"><h2 id="visual-heading">Relationship map</h2><span>{graph.nodes.length} nodes · {graph.edges.length} edges</span></div>
        <svg viewBox="0 0 800 560" role="img" aria-labelledby="graph-title graph-description">
          <title id="graph-title">Bounded catalog relationship graph</title>
          <desc id="graph-description">A visual map. The node and relationship tables after the map contain the same information.</desc>
          <g className="graph-edges" aria-hidden="true">{graph.edges.map((edge, index) => {
            const source = byID.get(edge.source), target = byID.get(edge.target)
            if (!source || !target) return null
            return <line key={`${edge.source}-${edge.kind}-${edge.target}-${index}`} x1={source.x} y1={source.y} x2={target.x} y2={target.y} />
          })}</g>
          <g className="graph-nodes" aria-hidden="true">{graph.nodes.map((node, index) => {
            const point = points[index]
            const radius = node.aggregate ? Math.min(34, 10 + Math.sqrt(node.count ?? 1) * 2) : 7
            return <g key={node.id} transform={`translate(${point.x} ${point.y})`}><circle r={radius} /><text y={radius + 15}>{short(node.name)}</text></g>
          })}</g>
        </svg>
      </section>
      <section className="graph-tables" aria-labelledby="graph-data-heading">
        <h2 id="graph-data-heading">Graph data</h2>
        <details open><summary>Nodes ({graph.nodes.length})</summary><div className="table-wrap"><table><thead><tr><th scope="col">Name</th><th scope="col">Type</th><th scope="col">Count</th><th scope="col">Next</th></tr></thead><tbody>{graph.nodes.map((node) => <tr key={node.id}><td>{node.name}</td><td>{node.aggregate ? graph.group_by : node.kind || 'entity'}</td><td>{node.count ?? 1}</td><td>{node.aggregate && node.group ? <Link to="/graph" search={{ mode: 'region', group_by: graph.group_by ?? 'kind', group: node.group }}>Open region</Link> : <Link to="/graph" search={{ mode: 'neighborhood', id: node.id, depth: 1 }}>Open neighborhood</Link>}</td></tr>)}</tbody></table></div></details>
        <details><summary>Relationships ({graph.edges.length})</summary><div className="table-wrap"><table><thead><tr><th scope="col">Source</th><th scope="col">Kind</th><th scope="col">Target</th><th scope="col">Count</th></tr></thead><tbody>{graph.edges.map((edge, index) => <tr key={`${edge.source}-${edge.kind}-${edge.target}-${index}`}><td><code>{edge.source}</code></td><td>{edge.kind}</td><td><code>{edge.target}</code></td><td>{edge.count ?? 1}</td></tr>)}</tbody></table></div></details>
      </section>
    </div>
  )
}

function layout(count: number): Array<{ x: number; y: number }> {
  if (count === 0) return []
  const columns = Math.ceil(Math.sqrt(count * 1.4))
  const rows = Math.ceil(count / columns)
  return Array.from({ length: count }, (_, index) => ({
    x: 55 + (index % columns) * (690 / Math.max(1, columns - 1)),
    y: 48 + Math.floor(index / columns) * (460 / Math.max(1, rows - 1)),
  }))
}

function short(value: string): string { return value.length > 20 ? `${value.slice(0, 18)}…` : value }
