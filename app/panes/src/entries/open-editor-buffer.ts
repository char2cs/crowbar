import { workspaceRef } from '@/lib/api/documents'
import { openDocument, openUntitledDocument } from '@/features/editor/documents/open-document'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { editorTarget } from './editor-target'
import type { OpenBuffer } from './open-buffer'

/** Opens the file a file pane was saved with. */
export const openEditorBuffer: OpenBuffer = async (wsId, saved) => {
  const target = editorTarget(saved)
  if (!target) throw new Error('the file pane has no file to open')
  if (target.kind === 'untitled') return openUntitledDocument(wsId, ROOT_PANE_ID)
  const ref =
    target.kind === 'host' ? { absolutePath: target.path } : workspaceRef(wsId, target.path)
  return openDocument(ref, { paneId: ROOT_PANE_ID })
}
