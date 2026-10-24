/**
 * Pure, React-free core of the per-pane editor switch controller.
 *
 * `applyActiveBuffer` is the imperative switch logic that runs whenever a pane's
 * active document changes or opens. It is intentionally decoupled from React and from
 * Monaco so it can be unit-tested with fakes: given a manager + an active-editor
 * registry + the buffer that just became active, it swaps the pane's retained
 * widget onto that buffer's model and publishes the new ActiveEditorContext.
 *
 * A tab switch must NOT re-render React, so this is driven by `store.subscribe`
 * (vanilla zustand) in {@link usePaneEditorController}, not by a render-path hook.
 */

import type { DocumentRef } from '@/lib/api/documents'
import { documentUri } from '@/features/editor/documents/document-ref'
import type {
  ActiveEditorContext,
  ActiveEditorRegistry,
} from '@/features/editor/lib/active-editor-context'

/** The minimal manager surface the switch logic needs (subset of EditorManager). */
export interface PaneSwitchManager {
  /** False, leaving the pane empty, while the document is not open. */
  showBuffer(paneId: string, uri: string): boolean
  /** Underlying monaco standalone editor for the pane (opaque to this module). */
  getRawEditor(paneId: string): unknown
}

/** The minimal shape of the buffer that just became active in the pane. */
export interface ActiveBufferInfo {
  /** The buffer's document — the model key, so two workspaces' files at the
   *  same path never share a model. */
  doc: DocumentRef
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
 * Returns the published ActiveEditorContext, or undefined when nothing was
 * published (no buffer, its document not open yet, or no raw editor).
 */
export function applyActiveBuffer(
  deps: { manager: PaneSwitchManager; registry: ActiveEditorRegistry },
  paneId: string,
  buffer: ActiveBufferInfo | null,
): ActiveEditorContext | undefined {
  if (!buffer) return undefined
  const { manager, registry } = deps
  const uri = documentUri(buffer.doc)

  if (!manager.showBuffer(paneId, uri)) return undefined

  const editor = manager.getRawEditor(paneId) as EditorWithModel | null
  const model = editor?.getModel() ?? null
  if (!editor || !model) return undefined

  const ctx: ActiveEditorContext = {
    paneId,
    uri,
    filePath: buffer.doc.path,
    model,
    editor,
  }
  registry.set(paneId, ctx)
  return ctx
}
