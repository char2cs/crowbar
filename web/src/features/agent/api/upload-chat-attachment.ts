import { nanoid } from 'nanoid'
import { chatBase } from '@/features/agent/api/agent-chat-reads'
import { apiFetchRaw } from '@/lib/api/fetch'

export interface UploadedChatAttachment {
  ref: string
  filename: string
  size: number
  contentType: string
}

export type UploadChatAttachmentInput = { file: File } | { path: string }

interface UploadAttachmentResponseBody {
  data: { ref: string; fileName: string; size: number; contentType: string }
}

/**
 * Multipart bytes for a `File` (no host path), or JSON `{path, id}` for a desktop
 * drop the daemon reads itself. `id` is passed in only when the caller must reuse it.
 */
export async function uploadChatAttachment(
  wsId: string,
  chatId: string,
  input: UploadChatAttachmentInput,
  id: string = nanoid(),
): Promise<UploadedChatAttachment> {
  const path = `${chatBase(wsId)}/${encodeURIComponent(chatId)}/attachments`
  const response =
    'file' in input
      ? await postMultipartAttachment(path, input.file, id)
      : await apiFetchRaw(path, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ path: input.path, id }),
        })
  const { data } = (await response.json()) as UploadAttachmentResponseBody
  return { ref: data.ref, filename: data.fileName, size: data.size, contentType: data.contentType }
}

async function postMultipartAttachment(path: string, file: File, id: string): Promise<Response> {
  const { body, contentType } = await encodeMultipartAttachment(file, id)
  return apiFetchRaw(path, { method: 'POST', headers: { 'Content-Type': contentType }, body })
}

/**
 * Not `FormData`: WKWebView streams Blob-backed bodies to the `crowbar://` scheme
 * handler, which Tauri never drains, so the daemon gets an empty body. A plain
 * `Uint8Array` survives the proxy with the same wire bytes.
 */
async function encodeMultipartAttachment(
  file: File,
  id: string,
): Promise<{ body: Uint8Array<ArrayBuffer>; contentType: string }> {
  const boundary = `----crowbarAttachment${nanoid()}`
  const encoder = new TextEncoder()
  const head = encoder.encode(
    `--${boundary}\r\n` +
      `Content-Disposition: form-data; name="id"\r\n\r\n${id}\r\n` +
      `--${boundary}\r\n` +
      `Content-Disposition: form-data; name="file"; filename="${escapeFormDataValue(file.name)}"\r\n` +
      `Content-Type: ${file.type || 'application/octet-stream'}\r\n\r\n`,
  )
  const tail = encoder.encode(`\r\n--${boundary}--\r\n`)
  const fileBytes = new Uint8Array(await file.arrayBuffer())
  const body = new Uint8Array(head.length + fileBytes.length + tail.length)
  body.set(head, 0)
  body.set(fileBytes, head.length)
  body.set(tail, head.length + fileBytes.length)
  return { body, contentType: `multipart/form-data; boundary=${boundary}` }
}

// Go's multipart escapeQuotes, plus stripping CR/LF: a quoted header value has
// no escape for them, and one would inject a header line into the body.
function escapeFormDataValue(value: string): string {
  return value
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '\\"')
    .replace(/[\r\n]/g, '')
}
