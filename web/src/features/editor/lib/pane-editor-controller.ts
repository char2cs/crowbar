/**
 * Pure, React-free core of the per-pane editor switch controller.
 *
 * `applyActiveBuffer` is the imperative switch logic that runs whenever a pane's
 * `activeBufferId` changes. It is intentionally decoupled from React and from
 * Monaco so it can be unit-tested with fakes: given a manager + an active-editor
 * registry + the buffer that just became active, it swaps the pane's retained
 * widget onto that buffer's model and publishes the new ActiveEditorContext.
 *
 * A tab switch must NOT re-render React, so this is driven by `store.subscribe`
 * (vanilla zustand) in {@link usePaneEditorController}, not by a render-path hook.
 */

import { fileUri } from '@/features/editor/lib/editor-uri'
import type {
  ActiveEditorContext,
  ActiveEditorRegistry,
} from '@/features/editor/lib/active-editor-context'

/** The minimal manager surface the switch logic needs (subset of EditorManager). */
export interface PaneSwitchManager {
  showBuffer(paneId: string, uri: string): void
  /** Underlying monaco standalone editor for the pane (opaque to this module). */
  getRawEditor(paneId: string): unknown
}

/** The minimal shape of the buffer that just became active in the pane. */
export interface ActiveBufferInfo {
  /** Buffer id — the content-write target (so a flush attributes to the right
   *  buffer even after the active buffer has changed on a fast switch). */
  bufferId: string
  /** Filesystem path — the stable model key (`fileUri(filePath)`). */
  filePath: string
}

/** Opaque editor handle that exposes only `getModel()` (cast at the seam). */
interface EditorWithModel {
  getModel(): unknown
}

/**
 * Swap the pane's retained widget onto `buffer`'s model and publish the new
 * ActiveEditorContext to the registry.
 *
 * - No-op when `buffer` is null (nothing active).
 * - `manager.showBuffer` is itself uri-deduped (same uri → no model swap), and
 *   `registry.set` is uri-deduped (same uri → subscribers not re-notified), so
 *   switching to the buffer that is already active is a no-op end-to-end.
 *
 * Returns the published ActiveEditorContext (or undefined if it could not be
 * published, e.g. no buffer / no raw editor) so the caller can rebind listeners
 * to the new model.
 */
/**
 * I2 reconcile gate. On a swap/rebind the satellite external-sync effect would
 * otherwise apply the STORE snapshot over the freshly-bound model. That is only
 * safe when the store is the authoritative value — i.e. the buffer is NOT dirty.
 *
 * Genuine external changes (disk reload, format-on-save, undo applied to the
 * store) only ever update the store content for a CLEAN buffer; a dirty buffer
 * is flagged, never overwritten (see external-buffer-sync). So when the buffer
 * is dirty the held model carries pending local edits the store snapshot does
 * not, and applying it would CLOBBER those edits (lost keystrokes). Skip it.
 *
 * Returns true when the store snapshot may be reconciled onto the model.
 */
export function shouldReconcileModelFromStore(buffer: { isDirty: boolean } | null): boolean {
  return !buffer?.isDirty
}

export function applyActiveBuffer(
  deps: { manager: PaneSwitchManager; registry: ActiveEditorRegistry },
  paneId: string,
  buffer: ActiveBufferInfo | null,
): ActiveEditorContext | undefined {
  if (!buffer) return undefined
  const { manager, registry } = deps
  const uri = fileUri(buffer.filePath)

  manager.showBuffer(paneId, uri)

  const editor = manager.getRawEditor(paneId) as EditorWithModel | null
  const model = editor?.getModel() ?? null
  if (!editor || !model) return undefined

  const ctx: ActiveEditorContext = {
    paneId,
    uri,
    filePath: buffer.filePath,
    model,
    editor,
  }
  registry.set(paneId, ctx)
  return ctx
}
