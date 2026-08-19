import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { FormEvent, useMemo, useState } from 'react'
import { GraphView } from '../components/GraphView'
import { QueryState, StatePanel } from '../components/StatePanel'
import { useSource } from '../lib/source-context'
import { invalidGraphParams } from '../url-state'

const route = getRouteApi('/graph')

export default function Graph() {
  const source = useSource()
  const search = route.useSearch()
  const navigate = useNavigate({ from: '/graph' })
  const [scope, setScope] = useState(search.id ?? search.group ?? '')
  const mode = search.mode ?? 'aggregate'
  const groupBy = search.group_by ?? 'kind'
  const invalid = useMemo(() => invalidGraphParams(new URLSearchParams(location.search)), [search])
  const query = useQuery({
    queryKey: ['graph', source.sourceDigest, mode, groupBy, search.group, search.id, search.depth],
    queryFn: () => source.graph({ mode, groupBy, group: search.group, id: search.id, depth: search.depth }),
  })

  function apply(event: FormEvent) {
    event.preventDefault()
    const value = scope.trim()
    if (mode === 'region') void navigate({ search: { mode, group_by: groupBy, group: value || undefined } })
    else if (mode === 'neighborhood') void navigate({ search: { mode, group_by: groupBy, id: value || undefined, depth: search.depth ?? 1 } })
    else void navigate({ search: { mode, group_by: groupBy } })
  }

  return (
    <main className="page graph-page" id="main-content" data-test="graph" tabIndex={-1}>
      <header className="page-header compact-header"><p className="eyebrow">Progressive graph</p><h1>Start with shape. Then focus.</h1><p>Explorer never sends an unbounded full graph. Move from aggregates to a region or to one entity's one- or two-hop neighborhood.</p></header>
      {invalid.length > 0 && <StatePanel kind="notice" title="Some URL options were ignored" detail={`Unsupported or invalid options: ${invalid.join(', ')}.`} />}
      <form className="graph-controls" onSubmit={apply}>
        <label>Level<select value={mode} onChange={(event) => { const value = event.target.value as 'aggregate' | 'region' | 'neighborhood'; setScope(''); void navigate({ search: { mode: value, group_by: groupBy, depth: value === 'neighborhood' ? 1 : undefined } }) }}><option value="aggregate">Aggregate</option><option value="region">Region</option><option value="neighborhood">Neighborhood</option></select></label>
        <label>Group by<select value={groupBy} onChange={(event) => void navigate({ search: { mode: 'aggregate', group_by: event.target.value as 'kind' | 'surface' } })}><option value="kind">Kind</option><option value="surface">Surface</option></select></label>
        {mode !== 'aggregate' && <label>{mode === 'region' ? 'Group name' : 'Entity ID'}<input value={scope} onChange={(event) => setScope(event.target.value)} maxLength={128} placeholder={mode === 'region' ? 'service' : 'service.seed-api'} required /></label>}
        {mode === 'neighborhood' && <label>Depth<select value={search.depth ?? 1} onChange={(event) => void navigate({ search: (old) => ({ ...old, depth: Number(event.target.value) as 1 | 2 }) })}><option value="1">One hop</option><option value="2">Two hops</option></select></label>}
        <button type="submit">Apply scope</button>
      </form>
      <QueryState query={query} empty={(result) => result.data.nodes.length === 0 && !result.data.refinement}>{(result) => <GraphView graph={result.data} />}</QueryState>
    </main>
  )
}
