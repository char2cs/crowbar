import { workspaceRef, type DocumentRef } from '@/lib/api/documents'
import { documentName } from '@/features/editor/documents/document-ref'
import { openDocument, openUntitledDocument } from '@/features/editor/documents/open-document'
import { restoreDocuments } from '@/features/editor/documents/restore-documents'
import { revealInEditor } from '@/features/editor/lib/reveal'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { editorTarget } from './editor-target'
import type { OpenBuffer } from './open-buffer'

const HOST = 'host'

function placeThenRestore(ref: DocumentRef): Promise<void> {
  windowPaneStore
    .getState()
    .bufferActions.openContent(
      { type: 'editor', doc: ref, name: documentName(ref) },
      { paneId: ROOT_PANE_ID },
    )
  return restoreDocuments()
}

/** Opens the file a file pane was saved with: one the user just opened is read first and refused when it cannot be, one restored with the layout keeps its tab whatever its read finds. */
export const openEditorBuffer: OpenBuffer = async (wsId, saved, refuse) => {
  const target = editorTarget(saved)
  if (!target) throw new Error('the file pane has no file to open')
  if (target.kind === 'untitled') return openUntitledDocument(wsId, ROOT_PANE_ID)
  const ref: DocumentRef =
    target.kind === HOST ? { authority: HOST, path: target.path } : workspaceRef(wsId, target.path)
  if (!target.fresh) return placeThenRestore(ref)
  const spec = target.kind === HOST ? { absolutePath: target.path } : ref
  const options = { paneId: ROOT_PANE_ID, onRefused: refuse }
  if (target.line === null) return openDocument(spec, options)
  return revealInEditor({
    doc: spec,
    position: { line: target.line - 1, character: 0 },
    ...options,
  })
}
