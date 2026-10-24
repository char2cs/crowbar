/**
 * Reveal a file position in the editor — the one navigation path for
 * go-to-definition, references, jump-list Back/Forward and "open at line".
 *
 * The cursor is placed once the pane's retained Monaco widget actually shows
 * the target model ({@link editorReadyFor}), not after a guessed delay: the old
 * 100–150 ms timers raced the lazy editor chunk and the model swap, and placed
 * the cursor by a character offset computed from whichever buffer happened to
 * be active — the wrong file on a slow switch, the wrong line on a large one.
 */
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

/**
 * Resolves with the pane's editor once it shows `uri`, or null if the pane
 * moves to another tab (or the buffer closes) first. No timeout: the pane
 * either shows the buffer or the user navigated away.
 */
function editorReadyFor(paneId: string, uri: string, bufferId: string): Promise<CodeEditor | null> {
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
        if (ctx?.uri === uri && ctx.editor) settle(ctx.editor as CodeEditor)
      }),
    )
    if (abandoned()) settle(null)
    own(windowPaneStore.subscribe(() => abandoned() && settle(null)))
  })
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

/**
 * Open (or switch to) `target.doc` and place the cursor. Resolves with the
 * editor showing it, or null when the document could not be opened (the open
 * reports why) or the user navigated away before it showed.
 */
export async function revealInEditor(target: RevealTarget): Promise<CodeEditor | null> {
  const bufferId = await openDocument(target.doc, { beforeShow: target.beforeShow })
  if (!bufferId) return null
  const state = windowPaneStore.getState()
  const buffer = state.buffers.find((b) => b.id === bufferId)
  if (!buffer || !isEditorContent(buffer)) return null
  const paneId = state.panes[state.activePaneId]?.editorTabIds.includes(bufferId)
    ? state.activePaneId
    : (state.paneActions.getPaneByEditorTabId(bufferId)?.id ?? state.activePaneId)
  const editor = await editorReadyFor(paneId, documentUri(documentRefOf(buffer)), bufferId)
  if (editor) placeCursor(editor, target)
  return editor
}
