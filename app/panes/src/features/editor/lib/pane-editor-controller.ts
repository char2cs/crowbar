// React- and Monaco-free core of the per-pane editor switch, so it is testable with fakes. Driven
// by `store.subscribe` in usePaneEditorController, not a render-path hook, because a tab switch
// must not re-render React.

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

/** Swap the pane's retained widget onto `buffer`'s model and publish its ActiveEditorContext.
 *  Both steps dedupe by uri, so re-applying the active buffer is a no-op. Returns the context, or
 *  undefined when nothing was published (no buffer, document not open yet, no raw editor). */
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
