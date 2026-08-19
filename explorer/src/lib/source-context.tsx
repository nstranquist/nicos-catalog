import { useQuery } from '@tanstack/react-query'
import { createContext, useContext, type ReactNode } from 'react'
import { discoverSource, type ExplorerSource } from './data'

const SourceContext = createContext<ExplorerSource | undefined>(undefined)

export function SourceProvider({ children }: { children: ReactNode }) {
  const source = useQuery({
    queryKey: ['explorer-source'],
    queryFn: () => discoverSource(undefined, preferredSource()),
    staleTime: Infinity,
    retry: 1,
  })

  if (source.isPending) {
    return <BootstrapState title="Opening Explorer" detail="Checking for a local API or a static public catalog." />
  }
  if (source.isError) {
    return <BootstrapState title="Explorer could not open" detail={source.error instanceof Error ? source.error.message : 'The data source is not available.'} retry={() => source.refetch()} />
  }
  return <SourceContext.Provider value={source.data}>{children}</SourceContext.Provider>
}

function preferredSource(): 'auto' | 'static' {
  return document.querySelector<HTMLMetaElement>('meta[name="nicos-catalog-source"]')?.content === 'static'
    ? 'static'
    : 'auto'
}

export function useSource(): ExplorerSource {
  const source = useContext(SourceContext)
  if (!source) throw new Error('Explorer source is not ready.')
  return source
}

function BootstrapState({ title, detail, retry }: { title: string; detail: string; retry?: () => void }) {
  return (
    <main className="bootstrap-state" id="main-content">
      <div className="brand-mark" aria-hidden="true">NC</div>
      <p className="eyebrow">Nicos Catalog Explorer</p>
      <h1>{title}</h1>
      <p>{detail}</p>
      {retry ? <button type="button" onClick={retry}>Try again</button> : <div className="loading-line" aria-hidden="true" />}
    </main>
  )
}
