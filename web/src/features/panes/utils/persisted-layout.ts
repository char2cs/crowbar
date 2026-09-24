import { PANE_CONTENT_TYPES, type PaneContent } from '@/features/panes/types/pane-content'

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

/**
 * Load-time validation of a saved layout's buffers — the only place a
 * persisted buffer is checked.
 *
 * A saved layout outlives the code that wrote it: a buffer whose content type
 * this build no longer has would render blank, so it is dropped (graceful
 * fallback, not migration). Then invariant C2 is established once for what
 * was read: a tab id naming no surviving buffer leaves its pane, and a buffer
 * no pane lists is not restored. From then on the store keeps C2 itself (see
 * `buffer-release.ts`), so nothing is repaired on save.
 */
export function validateLoadedBuffers<P extends PersistablePane, T extends Snapshot<P>>(
  snapshot: T,
): T {
  const known = new Set(
    snapshot.buffers.filter((b) => PANE_CONTENT_TYPES.has(b.type)).map((b) => b.id),
  )
  const panes = {} as Record<string, P>
  const listed = new Set<string>()
  for (const [paneId, pane] of Object.entries(snapshot.panes) as [string, P][]) {
    const editorTabIds = pane.editorTabIds.filter((id) => known.has(id))
    for (const id of editorTabIds) listed.add(id)
    const activeEditorTabId =
      pane.activeEditorTabId && known.has(pane.activeEditorTabId)
        ? pane.activeEditorTabId
        : (editorTabIds[0] ?? null)
    panes[paneId] = { ...pane, editorTabIds, activeEditorTabId }
  }
  const buffers = snapshot.buffers.filter((b) => listed.has(b.id))
  return { ...snapshot, buffers, panes }
}
