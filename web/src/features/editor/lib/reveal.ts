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
import { readWorkspaceFile } from '@/features/file-system/controllers/platform'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import {
  getActiveWorkspaceStore,
  getWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import type { ActiveEditorRegistry } from './active-editor-context'

type CodeEditor = Monaco.editor.ICodeEditor

export interface RevealTarget {
  workspaceId: string
  /** Workspace-relative path. */
  path: string
  /** 0-based LSP-style position. */
  position?: { line: number; character: number }
  /** Restore an exact scroll offset (jump list) instead of centering. */
  scroll?: { top: number; left: number }
  /** Called with the buffer that will be shown, just before it is activated. */
  beforeShow?: (bufferId: string) => void
}

function findEditorBufferId(workspaceId: string, path: string): string | null {
  const buffer = windowPaneStore
    .getState()
    .buffers.find((b) => isEditorContent(b) && b.path === path && b.workspaceId === workspaceId)
  return buffer?.id ?? null
}

/**
 * Make `bufferId` the active tab of the pane that holds it, or attach it to
 * the active pane when none does (a pane only renders tabs in its own
 * editorTabIds — activating one it does not hold leaves it blank).
 */
export function showBufferInPane(bufferId: string): string {
  const state = windowPaneStore.getState()
  const holding = state.paneActions.getPaneByEditorTabId(bufferId)
  if (holding) {
    state.paneActions.setActivePane(holding.id)
    state.paneActions.activateEditorTabInPane(holding.id, bufferId)
    return holding.id
  }
  const buffer = state.buffers.find((b) => b.id === bufferId)
  if (buffer) state.paneActions.addEditorTabToPane(state.activePaneId, buffer)
  return state.activePaneId
}

function registriesFor(workspaceId: string): ActiveEditorRegistry[] {
  const own = getWorkspaceStore(workspaceId)?.activeEditorRegistry
  const active = getActiveWorkspaceStore()?.activeEditorRegistry
  return [own, active].filter((r, i, all): r is ActiveEditorRegistry => !!r && all.indexOf(r) === i)
}

/**
 * Resolves with the pane's editor once it shows `path`, or null if the pane
 * moves to another tab (or the buffer closes) first. No timeout: the pane
 * either shows the buffer or the user navigated away.
 */
export function editorReadyFor(
  paneId: string,
  workspaceId: string,
  path: string,
  bufferId: string,
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
    for (const registry of registriesFor(workspaceId)) {
      own(
        registry.subscribe(paneId, (ctx) => {
          if (ctx?.filePath === path && ctx.editor) settle(ctx.editor as CodeEditor)
        }),
      )
    }
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
    if (!target.scroll) editor.revealPositionInCenterIfOutsideViewport(position)
  }
  if (target.scroll) {
    editor.setScrollPosition({ scrollTop: target.scroll.top, scrollLeft: target.scroll.left })
  }
  editor.focus()
}

/**
 * Open (or switch to) `target.path` in `target.workspaceId` and place the
 * cursor. Resolves with the editor showing it, or null when the file could
 * not be opened or the user navigated away before it showed. Read failures
 * propagate so callers can tell the user.
 */
export async function revealInEditor(target: RevealTarget): Promise<CodeEditor | null> {
  let bufferId = findEditorBufferId(target.workspaceId, target.path)
  if (!bufferId) {
    // Read through the target's own workspace: sibling worktrees share
    // relative paths with different content.
    const content = await readWorkspaceFile(target.workspaceId, target.path)
    bufferId =
      findEditorBufferId(target.workspaceId, target.path) ??
      windowPaneStore.getState().bufferActions.openContent({
        type: 'editor',
        path: target.path,
        name: target.path.split('/').pop() || target.path,
        content,
        workspaceId: target.workspaceId,
      })
  }
  target.beforeShow?.(bufferId)
  const paneId = showBufferInPane(bufferId)
  const editor = await editorReadyFor(paneId, target.workspaceId, target.path, bufferId)
  if (editor) placeCursor(editor, target)
  return editor
}
