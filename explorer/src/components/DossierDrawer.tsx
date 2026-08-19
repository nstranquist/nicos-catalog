import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, type KeyboardEvent } from 'react'
import { useSource } from '../lib/source-context'
import { writeSelection } from '../selection-store'
import { DossierContent } from './DossierContent'
import { QueryState } from './StatePanel'

export function DossierDrawer({ entityID, onClose }: { entityID: string; onClose(): void }) {
  const source = useSource()
  const panel = useRef<HTMLDivElement>(null)
  const close = useRef<HTMLButtonElement>(null)
  const query = useQuery({ queryKey: ['dossier', source.sourceDigest, entityID], queryFn: () => source.dossier(entityID) })

  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
    writeSelection(entityID)
    close.current?.focus()
    return () => previous?.focus()
  }, [entityID])

  function keydown(event: KeyboardEvent<HTMLDivElement>) {
    if (event.key === 'Escape') {
      event.preventDefault()
      onClose()
      return
    }
    if (event.key !== 'Tab' || !panel.current) return
    const focusable = [...panel.current.querySelectorAll<HTMLElement>('a[href], button:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])')]
    if (focusable.length === 0) return
    const first = focusable[0]
    const last = focusable[focusable.length - 1]
    if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus() }
    else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus() }
  }

  return (
    <div className="drawer-layer" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
      <div className="dossier-drawer" data-test="dossier" role="dialog" aria-modal="true" aria-label={`Entity dossier: ${entityID}`} ref={panel} onKeyDown={keydown}>
        <button className="drawer-close" type="button" ref={close} onClick={onClose} aria-label="Close dossier">Close <span aria-hidden="true">×</span></button>
        <QueryState query={query}>{(result) => <DossierContent dossier={result.data} meta={result.meta} />}</QueryState>
      </div>
    </div>
  )
}
