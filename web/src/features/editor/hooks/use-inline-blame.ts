/**
 * Current-line blame: after the cursor's line, a muted "author, when · summary"
 * from the daemon's /blame. Loaded once per file (cached in the blame store,
 * invalidated on save); hidden while the buffer has unsaved edits, since
 * blame's line numbers describe the file on disk.
 */
import { useEffect } from 'react'
import type * as Monaco from 'monaco-editor'
import { blameKey, loadBlame, useGitBlameStore } from '@/features/git/stores/git-blame-store'
import type { BlameEntry } from '@/features/git/api/git-blame-api'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { isEditorContent } from '@/features/panes/types/pane-content'
import { useSettingsStore } from '@/features/settings/store'
import { dataOf } from '@/lib/loadable'
import { formatRelativeDate } from '@/utils/date'

const UNCOMMITTED = /^0+$/

function blameLabel(entry: BlameEntry): string {
  if (UNCOMMITTED.test(entry.commitHash)) return 'You · Uncommitted changes'
  const summary = entry.commitMessage.split('\n')[0] ?? ''
  return `${entry.author}, ${formatRelativeDate(entry.date)} · ${summary}`
}

function isDirty(workspaceId: string, path: string): boolean {
  const buffer = windowPaneStore
    .getState()
    .buffers.find((b) => isEditorContent(b) && b.path === path && b.workspaceId === workspaceId)
  return !!buffer && isEditorContent(buffer) && buffer.isDirty
}

export function useInlineBlame(
  editor: Monaco.editor.IStandaloneCodeEditor | null,
  model: Monaco.editor.ITextModel | null,
  workspaceId: string,
  filePath: string,
): void {
  const enabled = useSettingsStore((s) => s.settings.inlineBlame)
  const key = blameKey(workspaceId, filePath)
  const blame = useGitBlameStore((s) => s.blame[key])

  useEffect(() => {
    if (enabled && editor && model && filePath && !blame) void loadBlame(workspaceId, filePath)
  }, [enabled, editor, model, filePath, workspaceId, blame])

  useEffect(() => {
    const entries = dataOf(blame)
    if (!enabled || !editor || !model || !entries || entries.length === 0) return
    const byLine = new Map(entries.map((e) => [e.lineNumber, e]))
    const decorations = editor.createDecorationsCollection()
    const render = () => {
      const position = editor.getPosition()
      const entry = position ? byLine.get(position.lineNumber) : undefined
      if (!position || !entry || isDirty(workspaceId, filePath) || model.isDisposed()) {
        decorations.clear()
        return
      }
      const column = model.getLineMaxColumn(position.lineNumber)
      decorations.set([
        {
          range: {
            startLineNumber: position.lineNumber,
            startColumn: column,
            endLineNumber: position.lineNumber,
            endColumn: column,
          },
          options: {
            after: { content: `    ${blameLabel(entry)}`, inlineClassName: 'crowbar-inline-blame' },
          },
        },
      ])
    }
    render()
    const cursor = editor.onDidChangeCursorPosition(render)
    const edits = model.onDidChangeContent(render)
    return () => {
      cursor.dispose()
      edits.dispose()
      decorations.clear()
    }
  }, [enabled, editor, model, blame, workspaceId, filePath])
}
