// Current-line blame from the daemon's /blame, loaded once per file (cached, invalidated on save).
// Hidden while the buffer is dirty, since blame's line numbers describe the file on disk; host
// documents (no repository) and untitled buffers (no file) have none.
import { useEffect } from 'react'
import type * as Monaco from 'monaco-editor'
import { blameKey, loadBlame, useGitBlameStore } from '@/features/git/stores/git-blame-store'
import type { BlameEntry } from '@/features/git/api/git-blame-api'
import { workspaceRef } from '@/lib/api/documents'
import { findDocumentBuffer } from '../documents/open-document'
import { isUntitledRef } from '../documents/document-ref'
import { useSettingsStore } from '@/features/settings/store'
import { useHostResolved } from '@/lib/host-resolve'
import { dataOf } from '@/lib/loadable'
import { formatRelativeDate } from '@/utils/date'

const UNCOMMITTED = /^0+$/

function blameLabel(entry: BlameEntry): string {
  if (UNCOMMITTED.test(entry.commitHash)) return 'You · Uncommitted changes'
  const summary = entry.commitMessage.split('\n')[0] ?? ''
  return `${entry.author}, ${formatRelativeDate(entry.date)} · ${summary}`
}

function isDirty(workspaceId: string, path: string): boolean {
  const buffer = findDocumentBuffer(workspaceRef(workspaceId, path))
  return !!buffer?.isDirty
}

export function useInlineBlame(
  editor: Monaco.editor.IStandaloneCodeEditor | null,
  model: Monaco.editor.ITextModel | null,
  workspaceId: string | null,
  filePath: string,
): void {
  const enabled =
    useSettingsStore((s) => s.settings.inlineBlame) && !isUntitledRef({ path: filePath })
  const key = workspaceId === null ? '' : blameKey(workspaceId, filePath)
  const blame = useGitBlameStore((s) => s.blame[key])
  // A load before the workspace's host is known is recorded as a failure and not retried.
  const hostResolved = useHostResolved({ wsId: workspaceId ?? '' })

  useEffect(() => {
    if (!enabled || workspaceId === null || !hostResolved) return
    if (!editor || !model || !filePath || blame) return
    void loadBlame(workspaceId, filePath)
  }, [enabled, editor, model, filePath, workspaceId, blame, hostResolved])

  useEffect(() => {
    const entries = dataOf(blame)
    if (!enabled || workspaceId === null || !editor || !model || !entries?.length) return
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
