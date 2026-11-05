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

// Every pane id holding at least one editor buffer, keyed by membership so the array stays
// referentially stable across unrelated pane/buffer mutations.
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

// Portals this pane's retained EditorPane into the node PaneContainer registered for it. It lives
// outside SplitViewRoot's layout, so a tab switch, split or fullscreen never unmounts it.
function EditorHostSlot({ paneId }: { paneId: string }) {
  const entry = useSyncExternalStore(
    useCallback((cb) => subscribeEditorPortalEntry(paneId, cb), [paneId]),
    () => getEditorPortalEntry(paneId),
  )

  // `entry` can read undefined between PaneContainer's effect cleanup and re-run; returning null
  // then would unmount the editor. Keep the last good node, buffer and flag; only the pane
  // leaving useEditorHostPaneIds (unmounting this slot) tears the portal down.
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

// The window's one retained-editor host, a sibling of SplitViewRoot: it owns every pane's
// EditorPane (and Monaco widget), portaled into PaneContainer's node, so rebuilding a
// PaneContainer subtree never disposes and recreates a Monaco widget.
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
