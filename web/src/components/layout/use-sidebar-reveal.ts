import { useEffect, useRef, useState } from 'react'
import type { RefObject } from 'react'
import { useUIState } from '@/features/window/stores/ui-state-store'

/** True for the life of any sidebar row drag (`data-row-dragging`, set by use-sidebar-drag.ts). */
export function useRowDragging(): boolean {
  const [rowDragging, setRowDragging] = useState(false)
  useEffect(() => {
    const read = () => document.documentElement.hasAttribute('data-row-dragging')
    setRowDragging(read())
    const observer = new MutationObserver(() => setRowDragging(read()))
    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-row-dragging'],
    })
    return () => observer.disconnect()
  }, [])
  return rowDragging
}

/**
 * What brings the explorer out: the keyboard in the editing surface (Monaco, a
 * pane's editor view) until focus goes to a chat or anything else, and, when
 * auto-hidden, the pointer. The pointer reveal is sticky: leaving the window
 * from the card's own spot keeps it out, and the next pointer move over
 * anything else sends it back. An open settings dialog takes focus and the
 * pointer away without the user leaving the card, so both stay as they were
 * until it closes.
 */
export function useSidebarReveal({
  autoHide,
  cardRef,
}: {
  autoHide: boolean
  cardRef: RefObject<HTMLDivElement | null>
}) {
  const settingsOpen = useUIState((s) => s.isSettingsOpen)
  const [focusInEditor, setFocusInEditor] = useState(false)
  const [pointerSummoned, setPointerSummoned] = useState(false)
  const peekZoneRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!autoHide || !pointerSummoned || settingsOpen) return
    const onMove = (event: PointerEvent) => {
      const target = event.target
      const own = (el: HTMLElement | null) =>
        el !== null && target instanceof Node && el.contains(target)
      if (own(cardRef.current) || own(peekZoneRef.current)) return
      // A resize drag runs ahead of the card, so the pointer is briefly outside it.
      if (document.documentElement.hasAttribute('data-pane-resizing')) return
      setPointerSummoned(false)
    }
    document.addEventListener('pointermove', onMove, true)
    return () => document.removeEventListener('pointermove', onMove, true)
  }, [autoHide, pointerSummoned, settingsOpen, cardRef])

  useEffect(() => {
    if (settingsOpen) return
    const inEditor = (target: EventTarget | null) =>
      target instanceof Element && target.closest('[data-editor-view]') !== null
    const onFocusIn = (event: FocusEvent) => setFocusInEditor(inEditor(event.target))
    const onFocusOut = (event: FocusEvent) => {
      if (event.relatedTarget === null && document.hasFocus()) setFocusInEditor(false)
    }
    setFocusInEditor(inEditor(document.activeElement))
    document.addEventListener('focusin', onFocusIn)
    document.addEventListener('focusout', onFocusOut)
    return () => {
      document.removeEventListener('focusin', onFocusIn)
      document.removeEventListener('focusout', onFocusOut)
    }
  }, [settingsOpen])

  return { focusInEditor, pointerSummoned, summon: () => setPointerSummoned(true), peekZoneRef }
}
