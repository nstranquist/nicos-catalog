import type { ReactNode } from 'react'

type StateKind = 'loading' | 'empty' | 'error' | 'stale' | 'notice'

export function StatePanel({ kind, title, detail, action }: { kind: StateKind; title: string; detail: string; action?: ReactNode }) {
  return (
    <section className={`state-panel state-${kind}`} role={kind === 'error' ? 'alert' : 'status'} aria-live="polite">
      <span className="state-symbol" aria-hidden="true">{symbol(kind)}</span>
      <div>
        <h2>{title}</h2>
        <p>{detail}</p>
        {action && <div className="state-action">{action}</div>}
      </div>
    </section>
  )
}

function symbol(kind: StateKind): string {
  switch (kind) {
    case 'loading': return '···'
    case 'empty': return '○'
    case 'error': return '!'
    case 'stale': return '↻'
    case 'notice': return 'i'
  }
}

export function QueryState<T>({ query, empty, children }: {
  query: { data?: T; isPending: boolean; isError: boolean; error: unknown; isFetching: boolean; refetch(): unknown }
  empty?: (data: T) => boolean
  children(data: T, stale: boolean): ReactNode
}) {
  if (query.isPending) return <StatePanel kind="loading" title="Loading catalog data" detail="Explorer is reading a bounded data view." />
  if (query.isError && query.data === undefined) {
    return <StatePanel kind="error" title="The data could not load" detail={message(query.error)} action={<button type="button" onClick={() => query.refetch()}>Try again</button>} />
  }
  if (query.data === undefined) return <StatePanel kind="error" title="No response" detail="Explorer received no usable data." />
  if (empty?.(query.data)) return <StatePanel kind="empty" title="No matching items" detail="Change the filters or use a broader search." />
  return <>{query.isError && <StatePanel kind="stale" title="Showing the last good result" detail={message(query.error)} action={<button type="button" onClick={() => query.refetch()}>Refresh</button>} />}{children(query.data, query.isFetching)}</>
}

function message(error: unknown): string {
  return error instanceof Error ? error.message : 'Explorer could not complete this read.'
}
