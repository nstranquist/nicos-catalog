import { Link } from '@tanstack/react-router'
import type { Edge, Entity } from '../generated/contract'

export function EntityKind({ value }: { value: string }) {
  return <span className="kind-pill">{value}</span>
}

export function EntityStatus({ value }: { value?: string }) {
  return <span className={`status-dot status-${safeClass(value)}`}><span aria-hidden="true" />{value || 'unspecified'}</span>
}

export function EntityLink({ entity, from = '/catalog' }: { entity: Entity; from?: '/' | '/catalog' | '/graph' | '/health' }) {
  return (
    <Link to="/entity/$entityId" params={{ entityId: entity.id }} search={{ from }} className="entity-link">
      <strong>{entity.name}</strong>
      <code>{entity.id}</code>
    </Link>
  )
}

export function RelationshipList({ edges, direction, names }: { edges: Edge[]; direction: 'incoming' | 'outgoing'; names?: Map<string, string> }) {
  if (edges.length === 0) return <p className="quiet">No {direction} relationships.</p>
  return (
    <ul className="relationship-list">
      {edges.map((edge) => {
        const id = direction === 'incoming' ? edge.source : edge.target
        return (
          <li key={`${edge.source}-${edge.kind}-${edge.target}`}>
            <span>{direction === 'incoming' ? 'from' : 'to'}</span>
            <Link to="/entity/$entityId" params={{ entityId: id }} search={{ from: '/catalog' }}>{names?.get(id) ?? id}</Link>
            <code>{edge.kind}</code>
          </li>
        )
      })}
    </ul>
  )
}

function safeClass(value?: string): string {
  return (value || 'unspecified').toLocaleLowerCase().replace(/[^a-z0-9-]+/g, '-')
}
