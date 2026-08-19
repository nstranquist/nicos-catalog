import { Component, type ErrorInfo, type ReactNode } from 'react'
import { StatePanel } from './components/StatePanel'

export class ErrorBoundary extends Component<{ children: ReactNode }, { error?: Error }> {
  state: { error?: Error } = {}

  static getDerivedStateFromError(error: Error) { return { error } }

  componentDidCatch(error: Error, info: ErrorInfo) {
    if (import.meta.env.DEV) console.error(error, info.componentStack)
  }

  render() {
    if (!this.state.error) return this.props.children
    return (
      <main className="page" id="main-content">
        <StatePanel kind="error" title="Explorer stopped safely" detail="A display error prevented this view from opening." action={<button type="button" onClick={() => location.reload()}>Reload Explorer</button>} />
      </main>
    )
  }
}
