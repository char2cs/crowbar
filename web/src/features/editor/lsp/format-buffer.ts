import type { TextEdit } from 'vscode-languageserver-types'
import type { DocumentRef } from '@/lib/api/documents'
import { useSettingsStore } from '@/features/settings/store'
import { LspClient } from './lsp-client'
import { applyTextEditsToContent } from './workspace-edit'

/**
 * Format `text`, the document's current text, with its language server
 * (textDocument/formatting through the daemon). Resolves null when there is
 * nothing to change or no server formats the document.
 */
export async function formatDocumentWithLsp(
  ref: DocumentRef,
  text: string,
): Promise<string | null> {
  if (ref.authority !== 'workspace') return null
  const client = LspClient.getInstance()
  await client.flushChange(ref.workspaceId, ref.path)
  const { tabSize } = useSettingsStore.getState().settings
  const edits = await client
    .request<TextEdit[]>(ref.workspaceId, 'formatting', {
      path: ref.path,
      options: { tabSize, insertSpaces: true },
    })
    .catch(() => null)
  if (!edits || edits.length === 0) return null
  const formatted = applyTextEditsToContent(text, edits)
  return formatted === text ? null : formatted
}
