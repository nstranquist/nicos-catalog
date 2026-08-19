import { Suspense } from 'react'
import { createRootRoute, createRoute, createRouter } from '@tanstack/react-router'
import AppShell from './AppShell'
import { StatePanel } from './components/StatePanel'
import { lazyWithRetry } from './chunk-reload'
import { parseCatalogSearch, parseGraphSearch } from './url-state'

const Overview = lazyWithRetry('overview', () => import('./routes/Overview'))
const Catalog = lazyWithRetry('catalog', () => import('./routes/Catalog'))
const EntityPage = lazyWithRetry('entity', () => import('./routes/Entity'))
const Graph = lazyWithRetry('graph', () => import('./routes/Graph'))
const Health = lazyWithRetry('health', () => import('./routes/Health'))

function RoutePending() {
  return <main className="page" id="main-content"><StatePanel kind="loading" title="Opening view" detail="Explorer is loading this route." /></main>
}

function routeComponent(Component: typeof Overview) {
  return function LazyRoute() { return <Suspense fallback={<RoutePending />}><Component /></Suspense> }
}

const rootRoute = createRootRoute({
  component: AppShell,
  notFoundComponent: () => <main className="page" id="main-content"><StatePanel kind="error" title="Page not found" detail="Use the Explorer navigation to choose a known view." /></main>,
  errorComponent: () => <main className="page" id="main-content"><StatePanel kind="error" title="This route could not open" detail="Reload Explorer or choose another view." /></main>,
})

const overviewRoute = createRoute({ getParentRoute: () => rootRoute, path: '/', component: routeComponent(Overview) })
const catalogRoute = createRoute({ getParentRoute: () => rootRoute, path: '/catalog', validateSearch: parseCatalogSearch, component: routeComponent(Catalog) })
const entityRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/entity/$entityId',
  validateSearch: (raw): { from?: '/' | '/catalog' | '/graph' | '/health' } => {
    const from = typeof raw.from === 'string' && ['/', '/catalog', '/graph', '/health'].includes(raw.from) ? raw.from as '/' | '/catalog' | '/graph' | '/health' : undefined
    return from ? { from } : {}
  },
  component: routeComponent(EntityPage),
})
const graphRoute = createRoute({ getParentRoute: () => rootRoute, path: '/graph', validateSearch: parseGraphSearch, component: routeComponent(Graph) })
const healthRoute = createRoute({
  getParentRoute: () => rootRoute,
  path: '/health',
  validateSearch: (raw): { severity?: 'error' | 'warning' | 'info' } => raw.severity === 'error' || raw.severity === 'warning' || raw.severity === 'info' ? { severity: raw.severity } : {},
  component: routeComponent(Health),
})

const routeTree = rootRoute.addChildren([overviewRoute, catalogRoute, entityRoute, graphRoute, healthRoute])

export const router = createRouter({ routeTree, defaultPreload: 'intent', defaultPreloadStaleTime: 30_000, scrollRestoration: true })

declare module '@tanstack/react-router' {
  interface Register { router: typeof router }
}
