import type { DocumentRef } from '@/lib/api/documents'
import { documentUri } from '@/features/editor/documents/document-ref'
import type {
  ActiveEditorContext,
  ActiveEditorRegistry,
} from '@/features/editor/lib/active-editor-context'

export interface PaneSwitchManager {
  showBuffer(paneId: string, uri: string): boolean
  getRawEditor(paneId: string): unknown
}

export interface ActiveBufferInfo {
  doc: DocumentRef
}

interface EditorWithModel {
  getModel(): unknown
}

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
