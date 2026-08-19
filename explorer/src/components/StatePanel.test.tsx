import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { QueryState, StatePanel } from './StatePanel'

describe('StatePanel', () => {
  it.each([
    ['loading', 'Loading'],
    ['empty', 'Empty'],
    ['error', 'Error'],
    ['stale', 'Stale'],
    ['notice', 'Notice'],
  ] as const)('renders the %s state', (kind, title) => {
    render(<StatePanel kind={kind} title={title} detail="Bounded detail." />)
    expect(screen.getByRole(kind === 'error' ? 'alert' : 'status')).toHaveTextContent(`${title}Bounded detail.`)
  })
})

describe('QueryState', () => {
  it('renders loading, empty, and missing-response states', () => {
    const { rerender } = render(<QueryState query={query({ isPending: true })}>{() => 'ready'}</QueryState>)
    expect(screen.getByText('Loading catalog data')).toBeVisible()
    rerender(<QueryState query={query({ data: [] })} empty={(items) => items.length === 0}>{() => 'ready'}</QueryState>)
    expect(screen.getByText('No matching items')).toBeVisible()
    rerender(<QueryState query={query()}>{() => 'ready'}</QueryState>)
    expect(screen.getByText('No response')).toBeVisible()
  })

  it('offers a retry after a hard error', () => {
    const refetch = vi.fn()
    render(<QueryState query={query({ isError: true, error: new Error('Safe failure.'), refetch })}>{() => 'ready'}</QueryState>)
    expect(screen.getByRole('alert')).toHaveTextContent('Safe failure.')
    fireEvent.click(screen.getByRole('button', { name: 'Try again' }))
    expect(refetch).toHaveBeenCalledOnce()
  })

  it('keeps the last good value visible during a failed refresh', () => {
    const refetch = vi.fn()
    render(<QueryState query={query({ data: 'last result', isError: true, isFetching: true, error: 'failure', refetch })}>{(data, stale) => `${data}:${stale}`}</QueryState>)
    expect(screen.getByText('Showing the last good result')).toBeVisible()
    expect(screen.getByText('last result:true')).toBeVisible()
    fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    expect(refetch).toHaveBeenCalledOnce()
  })
})

function query<T>(overrides: Partial<{ data: T; isPending: boolean; isError: boolean; error: unknown; isFetching: boolean; refetch(): unknown }> = {}) {
  return { data: undefined, isPending: false, isError: false, error: undefined, isFetching: false, refetch: () => undefined, ...overrides }
}
