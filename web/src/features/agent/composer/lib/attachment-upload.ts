import {
  uploadChatAttachment,
  type UploadChatAttachmentInput,
} from '@/features/agent/api/upload-chat-attachment'
import { markdownForUpload } from '@/features/agent/composer/lib/attachment-markdown'
import {
  isCsvFile,
  resolveCsv,
  rowsToMarkdownTable,
} from '@/features/agent/composer/plate/attachments/resolve-csv'

/**
 * One upload+markdown resolution, shared by every entry point that hands in
 * a `{file}` or a Tauri `{path}` — drop, browse, the Attach File modal, and
 * the paste plugin's oversized-text-to-file fallback.
 *
 * CSV gets first refusal: a small, cleanly-shaped `.csv`/`text/csv` FILE
 * never touches the network at all — `resolveCsv` reads its bytes directly
 * and this returns a markdown table. Everything else — an oversized or
 * malformed CSV, any other kind, or a host PATH (there are no client-side
 * bytes to inspect before the daemon reads a path itself) — uploads, and
 * becomes an image or file link chosen by the response's own `contentType`.
 */
export async function uploadAttachmentMarkdown(
  wsId: string,
  chatId: string,
  input: UploadChatAttachmentInput,
): Promise<string> {
  if ('file' in input && isCsvFile(input.file)) {
    const bytes = new Uint8Array(await input.file.arrayBuffer())
    const resolution = resolveCsv(bytes)
    if (resolution.kind === 'table') return rowsToMarkdownTable(resolution.rows)
  }
  const result = await uploadChatAttachment(wsId, chatId, input)
  return markdownForUpload(result)
}
