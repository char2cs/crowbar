import type { EditorContent } from '@/features/panes/types/pane-content'
import { LspClient } from './lsp-client'

/** Format a buffer's text with its language server; null when nothing to do. */
export async function formatBufferWithLsp(
  buffer: EditorContent & { path: string },
): Promise<string | null> {
  const formatted = await LspClient.getInstance()
    .formatDocument(buffer.path, buffer.content)
    .catch(() => null)
  return formatted === null || formatted === buffer.content ? null : formatted
}
