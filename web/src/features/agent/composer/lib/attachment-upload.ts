import {
  uploadChatAttachment,
  type UploadChatAttachmentInput,
  type UploadOptions,
} from '@/features/agent/api/upload-chat-attachment'
import { markdownForUpload } from '@/features/agent/composer/lib/attachment-markdown'
import {
  isCsvFile,
  resolveCsv,
  rowsToMarkdownTable,
} from '@/features/agent/composer/plate/attachments/resolve-csv'

/** A small, clean CSV file inlines as a table without touching the network.
 *  Everything else uploads — including a host path, which has no client-side bytes. */
export async function uploadAttachmentMarkdown(
  wsId: string,
  chatId: string,
  input: UploadChatAttachmentInput,
  options?: UploadOptions,
): Promise<string> {
  if ('file' in input && isCsvFile(input.file)) {
    const bytes = new Uint8Array(await input.file.arrayBuffer())
    const resolution = resolveCsv(bytes)
    if (resolution.kind === 'table') return rowsToMarkdownTable(resolution.rows)
  }
  const result = options
    ? await uploadChatAttachment(wsId, chatId, input, undefined, options)
    : await uploadChatAttachment(wsId, chatId, input)
  return markdownForUpload(result)
}
