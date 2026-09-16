/**
 * Per-pane registry of where a pane currently wants its retained editor
 * widget portaled to, plus the props that widget should render with.
 *
 * `PaneContainer` publishes into this registry — it already computes exactly
 * which buffer is active, whether the editor view is visible, and whether the
 * pane itself has focus (see its own `activeBuffer`/`isActivePane`). The
 * always-mounted {@link EditorHostRegistry} is the sole reader: it lives
 * OUTSIDE the recursive pane-layout tree (SplitViewRoot/PaneNodeRenderer)
 * specifically so a pane's own React subtree can be torn down and rebuilt —
 * by a tab switch, a split, entering/exiting fullscreen — without ever
 * touching the portaled editor. `createPortal` re-targets the SAME live DOM
 * (and the SAME React component instance, with its retained Monaco widget)
 * into whatever node is registered here, instead of disposing and recreating
 * it. See pane-container.tsx's own doc for why those three cases all used to
 * destroy an editor's retained widget.
 */

export interface EditorPortalEntry {
  node: HTMLDivElement
  /**
   * The pane's currently ACTIVE buffer id, IFF it is an editor-type buffer —
   * null while a non-editor tab (branch review, terminal, ...) is active.
   * `EditorHostSlot` remembers the last non-null value itself, so the
   * retained widget keeps showing its last file while hidden instead of
   * going blank or unmounting.
   */
  activeEditorBufferId: string | null
  isPreview: boolean
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

/**
 * Subscribe to `paneId`'s entry. Fires immediately with the current value
 * (or `undefined`) on subscribe, then on every subsequent change — same
 * immediate-callback contract as `ActiveEditorRegistry.subscribe`, so a
 * consumer that (re)subscribes to a different paneId picks up whatever is
 * already published there without waiting for the next write.
 */
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
