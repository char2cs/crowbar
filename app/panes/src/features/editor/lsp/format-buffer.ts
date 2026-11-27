import type { TextEdit } from 'vscode-languageserver-types'
import type { DocumentRef } from '@/lib/api/documents'
import { useSettingsStore } from '@/features/settings/store'
import { LspClient } from './lsp-client'
import { applyTextEditsToContent } from './workspace-edit'
import { report } from '@/lib/errors/report'

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
    .catch((err: unknown) => {
      report(err, { action: 'Format the document' })
      return null
    })
  if (!edits || edits.length === 0) return null
  const formatted = applyTextEditsToContent(text, edits)
  return formatted === text ? null : formatted
}
