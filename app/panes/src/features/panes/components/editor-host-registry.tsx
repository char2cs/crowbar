import { Suspense, useCallback, useMemo, useRef, useSyncExternalStore } from 'react'
import { createPortal } from 'react-dom'
import { useStore } from 'zustand'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import {
  getEditorPortalEntry,
  subscribeEditorPortalEntry,
} from '@/features/panes/lib/editor-portal-registry'
import { EditorPane } from './editor-pane'

const ID_DELIM = '\u0000'
const EMPTY_IDS: string[] = []

function useEditorHostPaneIds(): string[] {
  const key = useStore(windowPaneStore, (s) => {
    const bufferTypeById = new Map(s.buffers.map((b) => [b.id, b.type]))
    const ids: string[] = []
    for (const paneId of Object.keys(s.panes)) {
      const pane = s.panes[paneId]
      if (pane.editorTabIds.some((id) => bufferTypeById.get(id) === 'editor')) {
        ids.push(paneId)
      }
    }
    return ids.sort().join(ID_DELIM)
  })
  return useMemo(() => (key ? key.split(ID_DELIM) : EMPTY_IDS), [key])
}

function EditorHostSlot({ paneId }: { paneId: string }) {
  const entry = useSyncExternalStore(
    useCallback((cb) => subscribeEditorPortalEntry(paneId, cb), [paneId]),
    () => getEditorPortalEntry(paneId),
  )

  const lastNodeRef = useRef<HTMLDivElement | null>(null)
  const lastShownRef = useRef<string | null>(null)
  const lastActiveSurfaceRef = useRef(false)
  if (entry) {
    lastNodeRef.current = entry.node
    lastActiveSurfaceRef.current = entry.isActiveSurface
    if (entry.activeEditorBufferId) lastShownRef.current = entry.activeEditorBufferId
  }
  const node = lastNodeRef.current
  const shown = lastShownRef.current

  if (!node || !shown) return null

  return createPortal(
    <Suspense fallback={null}>
      <EditorPane paneId={paneId} bufferId={shown} isActiveSurface={lastActiveSurfaceRef.current} />
    </Suspense>,
    node,
  )
}

export function EditorHostRegistry() {
  const paneIds = useEditorHostPaneIds()
  return (
    <>
      {paneIds.map((paneId) => (
        <EditorHostSlot key={paneId} paneId={paneId} />
      ))}
    </>
  )
}
