import type { Dossier, Meta } from '../generated/contract'
import { EntityKind, EntityStatus, RelationshipList } from './EntityBits'

export function DossierContent({ dossier, meta }: { dossier: Dossier; meta: Meta }) {
  const { entity, incoming, outgoing } = dossier
  return (
    <article className="dossier-content">
      <header className="dossier-header">
        <p className="eyebrow">Entity dossier</p>
        <h2>{entity.name}</h2>
        <code>{entity.id}</code>
        <div className="dossier-badges"><EntityKind value={entity.kind} /><EntityStatus value={entity.status} /></div>
      </header>
      <section aria-labelledby="facts-heading">
        <h3 id="facts-heading">Facts</h3>
        {entity.summary ? <p className="summary-copy">{entity.summary}</p> : <p className="quiet">No public summary is present.</p>}
        <dl className="facts-grid">
          <div><dt>Surface</dt><dd>{entity.surface || 'Unspecified'}</dd></div>
          <div><dt>Owner</dt><dd>{entity.owner_label || 'Unspecified'}</dd></div>
          <div><dt>Entry point</dt><dd>{entity.entrypoint_label || 'Unspecified'}</dd></div>
          <div><dt>Tags</dt><dd>{entity.tags?.length ? entity.tags.join(', ') : 'None'}</dd></div>
        </dl>
        {entity.url && <p><a href={entity.url} rel="noreferrer">Open public resource <span aria-hidden="true">↗</span></a></p>}
      </section>
      <section aria-labelledby="relationships-heading">
        <div className="section-heading-row">
          <h3 id="relationships-heading">Relationships</h3>
          <span>{meta.total ?? incoming.length + outgoing.length} total</span>
        </div>
        {meta.truncated && <p className="notice-copy">This bounded dossier omits some relationships.</p>}
        <h4>Outgoing</h4>
        <RelationshipList edges={outgoing} direction="outgoing" />
        <h4>Incoming</h4>
        <RelationshipList edges={incoming} direction="incoming" />
      </section>
    </article>
  )
}
