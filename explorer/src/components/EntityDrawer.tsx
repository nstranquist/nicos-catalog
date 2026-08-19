import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, type KeyboardEvent } from 'react'
import { useSource } from '../lib/source-context'
import { writeSelection } from '../selection-store'
import { EntityContent } from './EntityContent'
import { QueryState } from './StatePanel'

export function EntityDrawer({ entityID, onClose }: { entityID: string; onClose(): void }) {
  const source = useSource()
  const panel = useRef<HTMLDivElement>(null)
  const close = useRef<HTMLButtonElement>(null)
  const query = useQuery({ queryKey: ['entity', source.sourceDigest, entityID], queryFn: () => source.entityDetail(entityID) })

  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : undefined
    writeSelection(entityID)
    close.current?.focus()
    return () => {
      window.setTimeout(() => {
        if (document.querySelector('[data-test="entity-page"]')) return
        const opener = [...document.querySelectorAll<HTMLElement>('[data-test="open-entity"]')]
          .find((element) => element.dataset.entityId === entityID)
        const current = opener ?? (previous !== document.body && previous?.isConnected ? previous : undefined)
        current?.focus()
      }, 0)
    }
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
      <div className="entity-drawer" data-test="entity-page" role="dialog" aria-modal="true" aria-label={`Entity: ${entityID}`} ref={panel} onKeyDown={keydown}>
        <button className="drawer-close" type="button" ref={close} onClick={onClose} aria-label="Close page">Close <span aria-hidden="true">×</span></button>
        <QueryState query={query}>{(result) => <EntityContent detail={result.data} meta={result.meta} />}</QueryState>
      </div>
    </div>
  )
}
