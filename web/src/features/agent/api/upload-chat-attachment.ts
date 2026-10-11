import { nanoid } from 'nanoid'
import { chatBase } from '@/features/agent/api/agent-api'
import { LOCAL_HOST } from '@/features/hosts/types'
import { apiUrl } from '@/lib/api'
import { ApiError, PROXY_HEADER, proxyMarkerError } from '@/lib/api/errors'
import { convertFileSrc } from '@/lib/crowbar-bridge'
import { splitHostPath } from '@/lib/host-path'

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
 * drop (the daemon reads it itself). A remote daemon cannot read the desktop's
 * disk, so a path for a remote chat is read here and sent as multipart bytes.
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
  const base = chatBase(wsId)
  const url = apiUrl(`${base}/${encodeURIComponent(chatId)}/attachments`)
  const file = 'file' in input ? input.file : await fileForRemoteChat(base, input.path)
  const response = file
    ? await postMultipartAttachment(url, file, id)
    : await fetch(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: (input as { path: string }).path, id }),
      })

  if (!response.ok) throw await uploadFailure(response, base)
  const { data } = (await response.json()) as UploadAttachmentResponseBody
  return { ref: data.ref, filename: data.fileName, size: data.size, contentType: data.contentType }
}

// Named the way apiFetch names a failure, so a host's own trouble reads as that host's.
async function uploadFailure(response: Response, base: string): Promise<ApiError> {
  const { hostId } = splitHostPath(base)
  const marked = proxyMarkerError(response.headers.get(PROXY_HEADER), hostId, response.status)
  if (marked) return marked
  const message = `upload chat attachment failed: ${response.status} ${await response.text()}`
  return new ApiError(message, response.status, undefined, undefined, hostId)
}

// An early-out only: the daemon's attachments.MaxBytes is the authority on the cap.
const MAX_ATTACHMENT_BYTES = 25 * 1024 * 1024

// The bytes of `path` for a chat whose host (named by its qualified `base`) is remote,
// or null for a local chat, whose daemon reads the path itself. Read through the
// webview's asset protocol, whose HEAD reports the size without reading the file.
async function fileForRemoteChat(base: string, path: string): Promise<File | null> {
  if (splitHostPath(base).hostId === LOCAL_HOST.id) return null
  const name = path.split(/[\\/]/).pop() || 'attachment'
  const src = convertFileSrc(path)
  const head = await fetch(src, { method: 'HEAD' }).catch(() => null)
  if (head?.ok && Number(head.headers.get('content-length')) > MAX_ATTACHMENT_BYTES) {
    throw tooLarge(name)
  }
  const res = await fetch(src).catch(() => null)
  if (!res?.ok) throw new Error(`could not read ${name} to send it to the remote host`)
  const blob = await res.blob()
  if (blob.size > MAX_ATTACHMENT_BYTES) throw tooLarge(name)
  return new File([blob], name, { type: blob.type })
}

function tooLarge(name: string): Error {
  return new Error(`${name} is larger than the 25 MB attachment limit`)
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

// Mirrors Go's mime/multipart.Writer escapeQuotes (backslash and double-quote
// escaped) plus stripping CR/LF, which neither escapeQuotes nor a quoted-string
// header value has any valid escape for — left in, a filename carrying one
// splits this hand-built Content-Disposition line in two, injecting an extra
// header-looking line into the multipart body this client sends.
function escapeFormDataValue(value: string): string {
  return value
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '\\"')
    .replace(/[\r\n]/g, '')
}
