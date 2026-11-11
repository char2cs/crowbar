// The one navigation path for definition, references, jump-list Back/Forward and "open at line".
// The cursor is placed once the pane's retained widget shows the target model, never after a
// guessed delay, which races the lazy editor chunk and the model swap.
import type * as Monaco from 'monaco-editor'
import type { DocumentTarget } from '@/lib/api/documents'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { documentRefOf, documentUri } from '@/features/editor/documents/document-ref'
import { openDocument } from '@/features/editor/documents/open-document'
import { activeEditors } from './active-editor-context'

type CodeEditor = Monaco.editor.ICodeEditor

export interface RevealTarget {
  doc: DocumentTarget
  /** 0-based LSP-style position. */
  position?: { line: number; character: number }
  /** Called with the buffer that will be shown, just before it is activated. */
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

/** Place the cursor the moment the pane's editor shows `uri` and resolve with that editor; null
 *  if the pane moves to another tab or the buffer closes first. No timeout: the pane either shows
 *  the buffer or the user navigated away. */
function placeWhenShown(
  paneId: string,
  uri: string,
  bufferId: string,
  target: RevealTarget,
): Promise<CodeEditor | null> {
  return new Promise((resolve) => {
    const cleanups: Array<() => void> = []
    let settled = false
    // A subscription can settle synchronously while it is being made (the
    // registry replays the current context), so cleanups registered after
    // settling run immediately.
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
    // By uri, not path: one window registry sees every document.
    own(
      activeEditors.subscribe(paneId, (ctx) => {
        if (settled || ctx?.uri !== uri || !ctx.editor) return
        // Placed inside the notification, before dev (StrictMode) can remount the
        // widget: the manager hands its view state, cursor included, to the survivor.
        placeCursor(ctx.editor as CodeEditor, target)
        settle(ctx.editor as CodeEditor)
      }),
    )
    if (abandoned()) settle(null)
    own(windowPaneStore.subscribe(() => abandoned() && settle(null)))
  })
}

/** Open (or switch to) `target.doc` and place the cursor; resolves with that editor, or null if
 *  the open failed (it reports why) or the user navigated away. A remount may replace the editor;
 *  the survivor restores the cursor from view state and usePaneEditorSatellites focuses it. */
export async function revealInEditor(target: RevealTarget): Promise<CodeEditor | null> {
  const bufferId = await openDocument(target.doc, { beforeShow: target.beforeShow })
  if (!bufferId) return null
  const state = windowPaneStore.getState()
  const buffer = state.buffers.find((b) => b.id === bufferId)
  if (!buffer || !isEditorContent(buffer)) return null
  const paneId = state.panes[state.activePaneId]?.editorTabIds.includes(bufferId)
    ? state.activePaneId
    : (state.paneActions.getPaneByEditorTabId(bufferId)?.id ?? state.activePaneId)
  return placeWhenShown(paneId, documentUri(documentRefOf(buffer)), bufferId, target)
}
