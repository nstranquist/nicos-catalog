import { Link, Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { FormEvent, useEffect, useRef, useState } from 'react'
import { useSource } from './lib/source-context'

const navigation = [
  { to: '/', label: 'Overview', key: 'o' },
  { to: '/catalog', label: 'Catalog', key: 'c' },
  { to: '/graph', label: 'Graph', key: 'g' },
  { to: '/health', label: 'Health', key: 'h' },
] as const

type Theme = 'auto' | 'light' | 'dark'

export default function AppShell() {
  const source = useSource()
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const searchInput = useRef<HTMLInputElement>(null)
  const [search, setSearch] = useState('')
  const [menu, setMenu] = useState(false)
  const [theme, setTheme] = useState<Theme>(() => readTheme())
  const status = useQuery({ queryKey: ['status', source.sourceDigest], queryFn: () => source.status(), staleTime: Infinity })

  useEffect(() => {
    document.documentElement.dataset.theme = theme
    localStorage.setItem('nicos-catalog:theme', theme)
  }, [theme])

  useEffect(() => setMenu(false), [pathname])

  useEffect(() => {
    let pendingG = false
    let timer = 0
    const keydown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null
      const typing = target?.matches('input, textarea, select, [contenteditable="true"]')
      if (event.key === '/' && !typing && !event.metaKey && !event.ctrlKey && !event.altKey) {
        event.preventDefault()
        searchInput.current?.focus()
        return
      }
      if (typing || event.metaKey || event.ctrlKey || event.altKey) return
      if (pendingG) {
        const destination = navigation.find((item) => item.key === event.key.toLocaleLowerCase())
        pendingG = false
        clearTimeout(timer)
        if (destination) {
          event.preventDefault()
          void navigate({ to: destination.to, search: {} })
        }
        return
      }
      if (event.key.toLocaleLowerCase() === 'g') {
        pendingG = true
        timer = window.setTimeout(() => { pendingG = false }, 900)
      }
    }
    document.addEventListener('keydown', keydown)
    return () => { document.removeEventListener('keydown', keydown); clearTimeout(timer) }
  }, [navigate])

  function submit(event: FormEvent) {
    event.preventDefault()
    const q = search.trim()
    void navigate({ to: '/catalog', search: q ? { q } : {} })
  }

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">Skip to catalog content</a>
      <aside id="primary-navigation" className={`rail ${menu ? 'rail-open' : ''}`} aria-label="Primary">
        <div className="brand-lockup">
          <Link to="/" search={{}} className="brand-mark" aria-label="Nicos Catalog Explorer home">NC</Link>
          <div><strong>Nicos Catalog</strong><span>Explorer</span></div>
        </div>
        <nav aria-label="Primary">
          {navigation.map((item, index) => (
            <Link key={item.to} to={item.to} search={{}} activeOptions={{ exact: item.to === '/' }} activeProps={{ 'aria-current': 'page' }}>
              <span className="nav-index" aria-hidden="true">0{index + 1}</span>
              <span>{item.label}</span>
              <kbd>g {item.key}</kbd>
            </Link>
          ))}
        </nav>
        <div className="rail-meta">
          <span className="mode-badge">{source.kind}</span>
          <p>{source.projection} projection</p>
          <p>{status.data?.product_version ?? 'version unknown'}</p>
        </div>
      </aside>
      <div className="shell-main">
        <header className="topbar">
          <button type="button" className="menu-button" aria-expanded={menu} aria-controls="primary-navigation" onClick={() => setMenu((value) => !value)}>Menu</button>
          <form className="global-search" role="search" aria-label="Global catalog search" onSubmit={submit}>
            <label htmlFor="global-search">Search the catalog</label>
            <input id="global-search" ref={searchInput} value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Name, ID, tag, or surface" maxLength={256} />
            <kbd aria-hidden="true">/</kbd>
          </form>
          <button className="theme-button" type="button" onClick={() => setTheme(nextTheme(theme))} aria-label={`Color theme: ${theme}. Change theme.`}>{theme}</button>
        </header>
        <p className="route-announcement sr-only" aria-live="polite">{routeName(pathname)} view</p>
        <Outlet />
        <footer className="product-footer">
          <span>Nicos Catalog Explorer</span>
          <span>{status.data ? `${status.data.entity_count} entities · ${status.data.edge_count} relationships` : 'Reading catalog status'}</span>
          <span>No network telemetry</span>
        </footer>
      </div>
    </div>
  )
}

function readTheme(): Theme {
  const value = localStorage.getItem('nicos-catalog:theme')
  return value === 'light' || value === 'dark' ? value : 'auto'
}

function nextTheme(theme: Theme): Theme {
  if (theme === 'auto') return 'light'
  if (theme === 'light') return 'dark'
  return 'auto'
}

function routeName(path: string): string {
  if (path.startsWith('/catalog') || path.startsWith('/entity')) return 'Catalog'
  if (path.startsWith('/graph')) return 'Graph'
  if (path.startsWith('/health')) return 'Health'
  return 'Overview'
}
