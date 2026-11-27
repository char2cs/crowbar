import type * as Monaco from 'monaco-editor'
import type { DocumentTarget } from '@/lib/api/documents'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { documentRefOf, documentUri } from '@/features/editor/documents/document-ref'
import { openDocument, type OpenDocumentOptions } from '@/features/editor/documents/open-document'
import { activeEditors } from './active-editor-context'

type CodeEditor = Monaco.editor.ICodeEditor

export interface RevealTarget extends Pick<OpenDocumentOptions, 'paneId' | 'onRefused'> {
  doc: DocumentTarget
  position?: { line: number; character: number }
  beforeShow?: (bufferId: string) => void
}

function placeCursor(editor: CodeEditor, target: RevealTarget): void {
  if (target.position) {
    const position = {
      lineNumber: target.position.line + 1,
      column: target.position.character + 1,
    }
    editor.setSelection({
      startLineNumber: position.lineNumber,
      startColumn: position.column,
      endLineNumber: position.lineNumber,
      endColumn: position.column,
    })
    editor.revealPositionInCenterIfOutsideViewport(position)
  }
  editor.focus()
}

function placeWhenShown(
  paneId: string,
  uri: string,
  bufferId: string,
  target: RevealTarget,
): Promise<CodeEditor | null> {
  return new Promise((resolve) => {
    const cleanups: Array<() => void> = []
    let settled = false
    const own = (cleanup: () => void) => (settled ? cleanup() : cleanups.push(cleanup))
    const settle = (editor: CodeEditor | null) => {
      if (settled) return
      settled = true
      for (const cleanup of cleanups) cleanup()
      resolve(editor)
    }
    const abandoned = () => {
      const state = windowPaneStore.getState()
      return (
        !state.buffers.some((b) => b.id === bufferId) ||
        state.panes[paneId]?.activeEditorTabId !== bufferId
      )
    }
    own(
      activeEditors.subscribe(paneId, (ctx) => {
        if (settled || ctx?.uri !== uri || !ctx.editor) return
        placeCursor(ctx.editor as CodeEditor, target)
        settle(ctx.editor as CodeEditor)
      }),
    )
    if (abandoned()) settle(null)
    own(windowPaneStore.subscribe(() => abandoned() && settle(null)))
  })
}

export async function revealInEditor(target: RevealTarget): Promise<CodeEditor | null> {
  const { paneId: into, onRefused, beforeShow } = target
  const bufferId = await openDocument(target.doc, { paneId: into, onRefused, beforeShow })
  if (!bufferId) return null
  const state = windowPaneStore.getState()
  const buffer = state.buffers.find((b) => b.id === bufferId)
  if (!buffer || !isEditorContent(buffer)) return null
  const paneId = state.panes[state.activePaneId]?.editorTabIds.includes(bufferId)
    ? state.activePaneId
    : (state.paneActions.getPaneByEditorTabId(bufferId)?.id ?? state.activePaneId)
  return placeWhenShown(paneId, documentUri(documentRefOf(buffer)), bufferId, target)
}
