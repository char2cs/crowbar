import { PANE_CONTENT_TYPES, type PaneContent } from '@/features/panes/types/pane-content'

interface PersistablePane {
  editorTabIds: string[]
  activeEditorTabId: string | null
}

// Generic over the pane type so a real `Record<string, PaneGroup>` (the save
// site's actual shape) round-trips through this function unchanged in every
// field the caller doesn't ask us to touch — not just narrowed down to the
// two fields this function cares about.
export interface Snapshot<P extends PersistablePane = PersistablePane> {
  buffers: PaneContent[]
  panes: Record<string, P>
}

/**
 * Drops buffers whose content type this build no longer has, and heals the
 * pane membership that referred to them.
 *
 * A saved layout outlives the code that wrote it, so a tab opened before a
 * content type was retired comes back pointing at a renderer that is gone:
 * the pane's switch matches nothing and the tab renders blank,
 * indistinguishable from a bug. Dropping it is the graceful fallback —
 * deliberately not migration code translating old shapes into new ones,
 * which would mean carrying a definition of every type this app has ever
 * had. (A "New Tab" placeholder buffer used to need its own explicit rule
 * here; Task 1 removed the type from the union entirely, so it is now just
 * one more type this build no longer knows and falls out of the generic
 * check below.)
 *
 * Pane membership is persisted alongside the buffers, so dropping a buffer
 * WITHOUT stripping its id from `editorTabIds` leaves a stranded id — the
 * pane then activates a tab that does not exist and renders blank (see the
 * comment in pane-slice's `removeEditorTabFromPane`). Both halves move
 * together, here.
 *
 * Membership is checked against the buffers that actually survive the
 * type-check, not just the dropped ids themselves — any id a pane holds
 * that isn't backed by a surviving buffer gets healed, whether that id was
 * just doomed above or was already an orphan from some unrelated bug.
 * Consequently the early return below only fires when there is genuinely
 * nothing to do: no dropped buffers AND no pane already holding a stranded
 * id.
 */
export function stripNewTabs<P extends PersistablePane, T extends Snapshot<P>>(snapshot: T): T {
  // One pass builds all three: the doomed ids, the surviving buffers, and the
  // surviving id set the pane rebuild below heals against.
  const droppedIds = new Set<string>()
  const buffers: typeof snapshot.buffers = []
  const aliveIds = new Set<string>()
  for (const b of snapshot.buffers) {
    if (!PANE_CONTENT_TYPES.has(b.type)) {
      droppedIds.add(b.id)
      continue
    }
    buffers.push(b)
    aliveIds.add(b.id)
  }

  // Single pass over panes doubles as both the "is there anything to strip"
  // guard check and the rebuild — the alive set is computed once above and
  // reused here, so the common "nothing to strip" case doesn't cost a second
  // walk over every pane's editorTabIds.
  let changed = droppedIds.size > 0
  const panes = {} as Record<string, P>
  for (const [paneId, pane] of Object.entries(snapshot.panes) as [string, P][]) {
    const editorTabIds = pane.editorTabIds.filter((id) => aliveIds.has(id))
    const activeEditorTabId =
      pane.activeEditorTabId && !aliveIds.has(pane.activeEditorTabId)
        ? (editorTabIds[0] ?? null)
        : pane.activeEditorTabId

    if (
      editorTabIds.length !== pane.editorTabIds.length ||
      activeEditorTabId !== pane.activeEditorTabId
    ) {
      changed = true
    }
    panes[paneId] = { ...pane, editorTabIds, activeEditorTabId }
  }

  return changed ? { ...snapshot, buffers, panes } : snapshot
}
