import {
  PANE_CONTENT_TYPES,
  hasUnsavedEdits,
  type PaneContent,
} from '@/features/panes/types/pane-content'

interface PersistablePane {
  editorTabIds: string[]
  activeEditorTabId: string | null
}

// Generic over the pane type so a real `Record<string, PaneGroup>` round-trips
// unchanged in every field this function does not touch.
export interface Snapshot<P extends PersistablePane = PersistablePane> {
  buffers: PaneContent[]
  panes: Record<string, P>
}

interface Adoption {
  /** The pane that takes a kept buffer no pane lists. */
  adoptInto: string
  /** Keep every unlisted buffer, not only unsaved ones (a record with no views). */
  keepUnlisted?: boolean
}

/**
 * Load-time validation of a saved layout's buffers — the only place a
 * persisted buffer is checked.
 *
 * A saved layout outlives the code that wrote it: a buffer whose content type
 * this build no longer has would render blank, so it is dropped (graceful
 * fallback, not migration). Then invariant C2 is established once for what
 * was read: a tab id naming no surviving buffer leaves its pane, and a buffer
 * no pane lists is not restored — unless it holds unsaved edits (the only copy
 * of them), which `adoptInto` takes as a tab instead. From then on the store
 * keeps C2 itself (see `buffer-release.ts`), so nothing is repaired on save.
 */
export function validateLoadedBuffers<P extends PersistablePane, T extends Snapshot<P>>(
  snapshot: T,
  { adoptInto, keepUnlisted = false }: Adoption,
): T {
  const known = snapshot.buffers.filter((b) => PANE_CONTENT_TYPES.has(b.type))
  const knownIds = new Set(known.map((b) => b.id))
  const panes = {} as Record<string, P>
  const listed = new Set<string>()
  for (const [paneId, pane] of Object.entries(snapshot.panes) as [string, P][]) {
    const editorTabIds = pane.editorTabIds.filter((id) => knownIds.has(id))
    for (const id of editorTabIds) listed.add(id)
    panes[paneId] = { ...pane, editorTabIds }
  }
  const adopted = known.filter(
    (b) => !listed.has(b.id) && (keepUnlisted || hasUnsavedEdits(b)) && panes[adoptInto],
  )
  if (adopted.length > 0) {
    const pane = panes[adoptInto]
    panes[adoptInto] = {
      ...pane,
      editorTabIds: [...pane.editorTabIds, ...adopted.map((b) => b.id)],
    }
    for (const b of adopted) listed.add(b.id)
  }
  for (const [paneId, pane] of Object.entries(panes)) {
    const activeEditorTabId =
      pane.activeEditorTabId && pane.editorTabIds.includes(pane.activeEditorTabId)
        ? pane.activeEditorTabId
        : (pane.editorTabIds[0] ?? null)
    panes[paneId] = { ...pane, activeEditorTabId }
  }
  const buffers = known.filter((b) => listed.has(b.id))
  return { ...snapshot, buffers, panes }
}
