import type { TextEdit } from 'vscode-languageserver-types'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { useSettingsStore } from '@/features/settings/store'
import { LspClient } from './lsp-client'
import { applyTextEditsToContent } from './workspace-edit'

/**
 * Format a buffer's text with its language server (textDocument/formatting
 * through the daemon). Resolves null when there is nothing to change or no
 * server formats the file's language.
 */
export async function formatBufferWithLsp(
  buffer: EditorContent & { path: string },
): Promise<string | null> {
  const client = LspClient.getInstance()
  await client.flushChange(buffer.path)
  const { tabSize } = useSettingsStore.getState().settings
  const edits = await client
    .request<TextEdit[]>(buffer.workspaceId, 'formatting', {
      path: buffer.path,
      options: { tabSize, insertSpaces: true },
    })
    .catch(() => null)
  if (!edits || edits.length === 0) return null
  const formatted = applyTextEditsToContent(buffer.content, edits)
  return formatted === buffer.content ? null : formatted
}
