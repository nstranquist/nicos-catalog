import { getRouteApi, Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { DossierContent } from '../components/DossierContent'
import { QueryState } from '../components/StatePanel'
import { useSource } from '../lib/source-context'
import { writeSelection } from '../selection-store'

const route = getRouteApi('/entity/$entityId')

export default function EntityPage() {
  const source = useSource()
  const { entityId } = route.useParams()
  const search = route.useSearch()
  const query = useQuery({ queryKey: ['dossier', source.sourceDigest, entityId], queryFn: () => source.dossier(entityId) })
  writeSelection(entityId)

  return (
    <main className="page entity-page" id="main-content" data-test="entity" tabIndex={-1}>
      <nav className="breadcrumb" aria-label="Breadcrumb"><Link to={search.from ?? '/catalog'} search={{}}>← Back to {label(search.from)}</Link></nav>
      <QueryState query={query}>{(result) => <><DossierContent dossier={result.data} meta={result.meta} /><div className="entity-actions"><Link to="/graph" search={{ mode: 'neighborhood', id: entityId, depth: 1 }}>Open bounded neighborhood</Link></div></>}</QueryState>
    </main>
  )
}

function label(path?: string): string {
  switch (path) { case '/': return 'overview'; case '/graph': return 'graph'; case '/health': return 'health'; default: return 'catalog' }
}
