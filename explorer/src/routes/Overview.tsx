import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useSource } from '../lib/source-context'
import { QueryState } from '../components/StatePanel'

export default function Overview() {
  const source = useSource()
  const status = useQuery({ queryKey: ['status', source.sourceDigest], queryFn: () => source.status(), staleTime: Infinity })
  const health = useQuery({ queryKey: ['health', source.sourceDigest, 'overview'], queryFn: () => source.health() })
  const graph = useQuery({ queryKey: ['graph', source.sourceDigest, 'aggregate', 'kind'], queryFn: () => source.graph({ mode: 'aggregate', groupBy: 'kind' }) })

  return (
    <main className="page" id="main-content" tabIndex={-1}>
      <header className="page-header overview-hero">
        <p className="eyebrow">Read the system before changing it</p>
        <h1>One clear view of your software catalog.</h1>
        <p>Browse typed entities, follow their relationships, and inspect health without exposing the private source records behind the projection.</p>
      </header>
      <QueryState query={status}>{(value) => (
        <section className="metric-grid" aria-label="Catalog summary">
          <Metric value={value.entity_count} label="entities" />
          <Metric value={value.edge_count} label="relationships" />
          <Metric value={value.finding_count} label="health findings" />
          <Metric value={value.api_schema_version} label="contract version" prefix="v" />
        </section>
      )}</QueryState>

      <div className="overview-columns">
        <section aria-labelledby="shape-heading">
          <div className="section-heading-row"><h2 id="shape-heading">Catalog shape</h2><Link to="/graph" search={{}}>Open graph</Link></div>
          <QueryState query={graph} empty={(result) => result.data.nodes.length === 0}>{(result) => (
            <ol className="shape-list">
              {result.data.nodes.slice(0, 8).map((node) => (
                <li key={node.id}>
                  <span>{node.name}</span>
                  <span className="shape-rule"><i style={{ width: `${Math.max(4, (node.count ?? 0) / Math.max(...result.data.nodes.map((item) => item.count ?? 1)) * 100)}%` }} /></span>
                  <strong>{node.count ?? 0}</strong>
                </li>
              ))}
            </ol>
          )}</QueryState>
        </section>
        <section aria-labelledby="urgent-heading">
          <div className="section-heading-row"><h2 id="urgent-heading">Needs attention</h2><Link to="/health" search={{}}>Open health</Link></div>
          <QueryState query={health}>{(result) => result.data.findings.length === 0 ? (
            <div className="clean-bill"><span aria-hidden="true">✓</span><div><strong>No projected findings</strong><p>The bounded health view is clear.</p></div></div>
          ) : (
            <ul className="finding-preview">
              {result.data.findings.slice(0, 6).map((finding, index) => (
                <li key={`${finding.code}-${finding.entity_id ?? index}`}><span className={`severity severity-${finding.severity}`}>{finding.severity}</span><div><strong>{humanize(finding.code)}</strong><p>{finding.remediation}</p></div></li>
              ))}
            </ul>
          )}</QueryState>
        </section>
      </div>

      <section className="start-panel" aria-labelledby="start-heading">
        <p className="eyebrow">Choose a starting point</p>
        <h2 id="start-heading">What do you need to understand?</h2>
        <div className="start-links">
          <Link to="/catalog" search={{}}><span>01</span><strong>Find an entity</strong><small>Browse, search, and filter the projected catalog.</small></Link>
          <Link to="/graph" search={{}}><span>02</span><strong>Trace a relationship</strong><small>Move from aggregates to a bounded neighborhood.</small></Link>
          <Link to="/health" search={{}}><span>03</span><strong>Review health</strong><small>Read redacted findings and clear remediation.</small></Link>
        </div>
      </section>
    </main>
  )
}

function Metric({ value, label, prefix = '' }: { value: number; label: string; prefix?: string }) {
  return <div className="metric"><strong>{prefix}{value.toLocaleString()}</strong><span>{label}</span></div>
}

function humanize(value: string): string {
  return value.replaceAll('_', ' ').replace(/\b\w/g, (letter) => letter.toUpperCase())
}
