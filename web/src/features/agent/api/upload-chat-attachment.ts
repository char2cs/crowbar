import { nanoid } from 'nanoid'
import { chatBase } from '@/features/agent/api/agent-api'
import { API_BASE } from '@/lib/api'

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
 * Uploads one chat attachment and returns the durable reference to encode
 * into the message's markdown (`![alt](ref)` / `[name](ref)`).
 *
 * Two request shapes, matching the backend's two ingestion paths: multipart
 * bytes for a `File` (clipboard paste, browser picker — never has a host
 * path), or a JSON `{path, id}` body for a host path from a revived desktop
 * drop (the daemon reads it itself).
 *
 * `id` is optional and minted here by default: most callers (drop, Attach
 * File, paste-to-image) don't need to correlate it with anything else. The
 * one exception (the Excalidraw modal) generates its own single id to serve
 * both the fence-tag id and the filename shortid, and passes it through here.
 */
export async function uploadChatAttachment(
  wsId: string,
  chatId: string,
  input: UploadChatAttachmentInput,
  id: string = nanoid(),
): Promise<UploadedChatAttachment> {
  const url = `${API_BASE}${chatBase(wsId)}/${encodeURIComponent(chatId)}/attachments`
  const response =
    'file' in input
      ? await postMultipartAttachment(url, input.file, id)
      : await fetch(url, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ path: input.path, id }),
        })

  if (!response.ok) {
    throw new Error(`upload chat attachment failed: ${response.status} ${await response.text()}`)
  }
  const { data } = (await response.json()) as UploadAttachmentResponseBody
  return { ref: data.ref, filename: data.fileName, size: data.size, contentType: data.contentType }
}

async function postMultipartAttachment(url: string, file: File, id: string): Promise<Response> {
  const { body, contentType } = await encodeMultipartAttachment(file, id)
  return fetch(url, { method: 'POST', headers: { 'Content-Type': contentType }, body })
}

/**
 * Hand-encodes the multipart body as a plain `Uint8Array` instead of handing
 * `fetch` a `FormData` holding a `File`. This isn't stylistic: in the Tauri
 * desktop build, `fetch` targets the `crowbar://` custom URI scheme (proxied
 * to the daemon's unix socket by `desktop/src-tauri/src/api_proxy.rs`), and
 * WKWebView delivers any Blob-backed fetch body — a `FormData` containing a
 * `File`, or a bare `Blob` — to that scheme handler as a streamed
 * `httpBodyStream` rather than materialized `httpBody` data. Tauri's
 * asynchronous scheme-handler bridge never drains that stream, so the proxy
 * receives a genuinely EMPTY body and the daemon sees neither `id` nor `file`
 * (`400 {"error":"id required"}`) — reproduced directly against the running
 * dev app: identical FormData minus the File part, or a JSON string body,
 * both forward correctly; only a Blob/File-bearing body is lost in transit.
 * A `Uint8Array` is never Blob-backed, so it always survives the proxy, and
 * it produces the exact wire bytes a native `FormData` would — the backend's
 * `multipart.Reader` parses it identically either way.
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

// Mirrors Go's mime/multipart.Writer escapeQuotes — backslash and double-quote
// are the only two characters that would otherwise break the quoted filename.
function escapeFormDataValue(value: string): string {
  return value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
}
