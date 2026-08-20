import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { RouterProvider } from '@tanstack/react-router'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import axe from 'axe-core'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SourceProvider } from './lib/source-context'
import { router } from './router'

const entity = { id: 'service.seed-api', name: 'Seed API', kind: 'service', status: 'active', surface: 'runtime', tags: ['platform'], summary: 'A synthetic public service.' }
const edge = { source: 'repository.seed-api', target: entity.id, kind: 'builds' }
const routeReady = { timeout: 10_000 }

describe('Explorer application', () => {
  beforeEach(async () => {
    localStorage.clear()
    sessionStorage.clear()
    vi.stubGlobal('fetch', vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input)
      if (url === '/api/v1/status') return response({ product_version: 'v0.3.0', api_schema_version: 1, entity_count: 2, edge_count: 1, finding_count: 0 })
      if (url.startsWith('/api/v1/health')) return response({ ok: true, drift: 'clean', findings: [] })
      if (url.startsWith('/api/v1/graph')) return response({ mode: 'aggregate', group_by: 'kind', nodes: [{ id: 'group:service', name: 'service', group: 'service', count: 1, aggregate: true }], edges: [] }, { total: 2 })
      if (url.startsWith('/api/v1/entities?')) return response({ items: [entity] }, { total: 1 })
      if (url === `/api/v1/entities/${entity.id}`) return response({ entity, incoming: [edge], outgoing: [] }, { total: 1 })
      return new Response('missing', { status: 404 })
    }))
    await router.navigate({ to: '/', search: {} })
  })

  it('opens the overview, navigates the catalog, and closes a keyboard-safe page', async () => {
    const user = userEvent.setup()
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    const view = render(<QueryClientProvider client={queryClient}><SourceProvider><RouterProvider router={router} /></SourceProvider></QueryClientProvider>)

    expect(await screen.findByRole('heading', { name: /one clear view/i }, routeReady)).toBeVisible()
    await expectAccessible(view.container)

    await user.click(screen.getByRole('link', { name: /^catalog/i }))
    expect(await screen.findByRole('heading', { name: /find the exact thing/i }, routeReady)).toBeVisible()
    const openPage = await screen.findByRole('button', { name: /open page for seed api/i }, routeReady)
    await user.click(openPage)
    const dialog = await screen.findByRole('dialog', { name: /seed-api/i }, routeReady)
    expect(dialog).toBeVisible()
    expect(screen.getByRole('button', { name: /close page/i })).toHaveFocus()
    await expectAccessible(view.container)

    fireEvent.keyDown(dialog, { key: 'Escape' })
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    await waitFor(() => expect(screen.getByRole('button', { name: /open page for seed api/i })).toHaveFocus())
    expect(sessionStorage.getItem('nicos-catalog:explorer:selected')).toBeNull()
  }, 30_000)
})

async function expectAccessible(root: HTMLElement) {
  const result = await axe.run(root, { rules: { 'color-contrast': { enabled: false } } })
  expect(result.violations.map((violation) => ({ id: violation.id, targets: violation.nodes.map((node) => node.target) }))).toEqual([])
}

function response(data: unknown, meta: Record<string, unknown> = {}) {
  return new Response(JSON.stringify({ schema_version: 1, ok: true, projection_mode: 'local', source_digest: 'sha256:integration', data, meta: { truncated: false, ...meta } }), { headers: { 'Content-Type': 'application/json' } })
}
