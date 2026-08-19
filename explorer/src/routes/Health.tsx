import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import type { HealthSeverity } from '../generated/contract'
import { QueryState } from '../components/StatePanel'
import { useSource } from '../lib/source-context'

const route = getRouteApi('/health')

export default function Health() {
  const source = useSource()
  const search = route.useSearch()
  const navigate = useNavigate({ from: '/health' })
  const health = useQuery({ queryKey: ['health', source.sourceDigest, search.severity], queryFn: () => source.health(search.severity as HealthSeverity | undefined) })

  return (
    <main className="page" id="main-content" tabIndex={-1}>
      <header className="page-header compact-header"><p className="eyebrow">Validation and drift</p><h1>Health without leakage.</h1><p>Findings use stable codes, projected entity IDs, and safe remediation text. Source paths and rejected values do not enter this view.</p></header>
      <div className="health-toolbar"><label htmlFor="severity">Severity</label><select id="severity" value={search.severity ?? ''} onChange={(event) => void navigate({ search: { severity: event.target.value as HealthSeverity || undefined } })}><option value="">All findings</option><option value="error">Errors</option><option value="warning">Warnings</option><option value="info">Information</option></select><span className="projection-chip">{source.projection} projection · {source.kind}</span></div>
      <QueryState query={health}>{(result) => (
        <>
          <section className={`health-verdict ${result.data.ok ? 'verdict-ok' : 'verdict-attention'}`}><span aria-hidden="true">{result.data.ok ? '✓' : '!'}</span><div><p className="eyebrow">Current verdict</p><h2>{result.data.ok ? 'The projected catalog is healthy.' : 'The projected catalog needs attention.'}</h2><p>Drift: {result.data.drift}</p></div></section>
          <section aria-labelledby="findings-heading"><div className="section-heading-row"><h2 id="findings-heading">Findings</h2><span>{result.data.findings.length} shown</span></div>
            {result.data.findings.length === 0 ? <div className="clean-bill"><span aria-hidden="true">✓</span><div><strong>No matching findings</strong><p>Choose another severity to inspect a different bounded view.</p></div></div> : <div className="table-wrap"><table className="health-table"><thead><tr><th scope="col">Severity</th><th scope="col">Code</th><th scope="col">Entity</th><th scope="col">Remediation</th></tr></thead><tbody>{result.data.findings.map((finding, index) => <tr key={`${finding.code}-${finding.entity_id ?? index}`}><td><span className={`severity severity-${finding.severity}`}>{finding.severity}</span></td><td><code>{finding.code}</code></td><td>{finding.entity_id ? <code>{finding.entity_id}</code> : <span className="quiet">Catalog</span>}</td><td>{finding.remediation}</td></tr>)}</tbody></table></div>}
          </section>
        </>
      )}</QueryState>
    </main>
  )
}
