import { Suspense, useCallback, useMemo, useRef, useSyncExternalStore } from 'react'
import { createPortal } from 'react-dom'
import { useStore } from 'zustand'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useBufferActions } from '@/features/workspace/stores/hooks/use-buffer-store'
import {
  getEditorPortalEntry,
  subscribeEditorPortalEntry,
} from '@/features/panes/lib/editor-portal-registry'
import { EditorPane } from './editor-pane'

const ID_DELIM = '\u0000'
const EMPTY_IDS: string[] = []

/**
 * Every pane id that currently holds at least one 'editor'-type buffer,
 * membership-keyed so the returned array is referentially stable across
 * renders that don't add or remove one — same trick as workspace-host.tsx's
 * `existingIdsKey`, for the same reason (avoid re-rendering this list on
 * every unrelated pane/buffer mutation).
 */
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

/**
 * Portals this pane's retained `EditorPane` into whatever node
 * `PaneContainer` currently has registered for it. Mounted once per pane id
 * named by {@link useEditorHostPaneIds} — it does NOT unmount on a tab
 * switch, a split, or fullscreen, because none of those touch THIS
 * component's own position in the tree (it lives here, not inside
 * SplitViewRoot's recursive layout — see editor-portal-registry.ts's doc).
 */
function EditorHostSlot({ paneId }: { paneId: string }) {
  const entry = useSyncExternalStore(
    useCallback((cb) => subscribeEditorPortalEntry(paneId, cb), [paneId]),
    () => getEditorPortalEntry(paneId),
  )

  // Remember the last KNOWN GOOD target node, buffer (id + isPreview), and
  // active-surface flag — NOT just the active buffer id. `entry` itself goes
  // briefly `undefined` on every dependency change of PaneContainer's own
  // registration effect (a tab switch, a focus change — anything in that
  // effect's array): React runs its cleanup (`clearEditorPortalEntry`) and
  // its new body (`setEditorPortalEntry`) back to back, but this component's
  // reactive read of the registry can observe the gap in between, depending
  // on scheduling. Returning null for THAT gap — as an earlier version of
  // this component did — unmounts the portaled EditorPane exactly like the
  // bug this whole registry exists to prevent, just moved one level down and
  // made scheduling-dependent instead of structural (hence intermittent:
  // live-reported as the SAME blank-pane symptom reappearing with no
  // repro steps). The portal must stay mounted on the LAST good node through
  // any such gap; only losing this paneId from useEditorHostPaneIds's list
  // (the pane's last editor tab actually closed) should tear it down — that
  // unmounts EditorHostSlot itself, which is the real, structural signal.
  const lastNodeRef = useRef<HTMLDivElement | null>(null)
  const lastShownRef = useRef<{ id: string; isPreview: boolean } | null>(null)
  const lastActiveSurfaceRef = useRef(false)
  if (entry) {
    lastNodeRef.current = entry.node
    lastActiveSurfaceRef.current = entry.isActiveSurface
    if (entry.activeEditorBufferId) {
      lastShownRef.current = { id: entry.activeEditorBufferId, isPreview: entry.isPreview }
    }
  }
  const node = lastNodeRef.current
  const shown = lastShownRef.current

  const { promotePreview } = useBufferActions()
  const onPromote = useCallback(() => {
    if (shown) promotePreview(shown.id)
  }, [promotePreview, shown])

  if (!node || !shown) return null

  return createPortal(
    <Suspense fallback={null}>
      <EditorPane
        paneId={paneId}
        bufferId={shown.id}
        isActiveSurface={lastActiveSurfaceRef.current}
        isPreview={shown.isPreview}
        onPromote={onPromote}
      />
    </Suspense>,
    node,
  )
}

/**
 * THE WINDOW'S RETAINED-EDITOR HOST — one of it, for the whole window,
 * rendered as a sibling of `SplitViewRoot` (see `WindowPaneSurface`'s own
 * doc for the identical principle already applied one level up, to the
 * whole pane tree, and `SplitViewRoot`'s own doc for the same principle
 * applied to parked views).
 *
 * Holds the actual `EditorPane` — and with it, the retained Monaco widget —
 * for every pane with an open editor tab, portaled into whatever DOM node
 * `PaneContainer` currently publishes for that pane. `PaneContainer`'s own
 * subtree is free to be torn down and rebuilt by a tab switch, a pane split,
 * or entering/exiting fullscreen; nothing here moves when that happens, so
 * nothing here disposes and recreates a Monaco widget because of it.
 */
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
