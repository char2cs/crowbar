// Where each pane wants its retained editor portaled, and with what props. PaneContainer writes;
// EditorHostRegistry, outside the pane-layout tree, reads — so rebuilding a pane's subtree
// re-targets the same live editor instead of disposing it.

export interface EditorPortalEntry {
  node: HTMLDivElement
  /** The pane's active buffer if it is an editor, else null. EditorHostSlot keeps the last
   *  non-null id, so a hidden editor still shows its file rather than blanking. */
  activeEditorBufferId: string | null
  isActiveSurface: boolean
}

type Listener = (entry: EditorPortalEntry | undefined) => void

const entries = new Map<string, EditorPortalEntry>()
const listeners = new Map<string, Set<Listener>>()

function notify(paneId: string): void {
  const set = listeners.get(paneId)
  if (!set) return
  const entry = entries.get(paneId)
  for (const cb of set) cb(entry)
}

/** Publish (or update) the target + props for `paneId`. Called by PaneContainer. */
export function setEditorPortalEntry(paneId: string, entry: EditorPortalEntry): void {
  entries.set(paneId, entry)
  notify(paneId)
}

/** Remove `paneId`'s entry — called on PaneContainer unmount. */
export function clearEditorPortalEntry(paneId: string): void {
  if (!entries.has(paneId)) return
  entries.delete(paneId)
  notify(paneId)
}

export function getEditorPortalEntry(paneId: string): EditorPortalEntry | undefined {
  return entries.get(paneId)
}

/** Fires at once with the current entry (or undefined), then on every change, so subscribing to
 *  a different paneId picks up what is already published without waiting for a write. */
export function subscribeEditorPortalEntry(paneId: string, cb: Listener): () => void {
  let set = listeners.get(paneId)
  if (!set) {
    set = new Set()
    listeners.set(paneId, set)
  }
  set.add(cb)
  cb(entries.get(paneId))
  return () => {
    set!.delete(cb)
    if (set!.size === 0) listeners.delete(paneId)
  }
}
