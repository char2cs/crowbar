import { nanoid } from 'nanoid'
import { convertFileSrc } from '@/lib/crowbar-bridge'

/** An early-out cap on a file sent to a daemon; the daemon's own cap is the authority. */
export interface UploadLimit {
  bytes: number
  /** How a refusal names the cap: "25 MB attachment". */
  label: string
}

/**
 * The bytes of this computer's file `path`, for a daemon on another host that cannot read
 * this disk. Read through the webview's asset protocol, whose HEAD reports the size without
 * reading the file: a file over `limit` is refused unread.
 */
export async function readDesktopFile(path: string, limit: UploadLimit): Promise<File> {
  const name = path.split(/[\\/]/).pop() || 'file'
  const tooLarge = () => new Error(`${name} is larger than the ${limit.label} limit`)
  const src = convertFileSrc(path)
  const head = await fetch(src, { method: 'HEAD' }).catch(() => null)
  if (head?.ok && Number(head.headers.get('content-length')) > limit.bytes) throw tooLarge()
  const res = await fetch(src).catch(() => null)
  if (!res?.ok) throw new Error(`could not read ${name} to send it to the remote host`)
  const blob = await res.blob()
  if (blob.size > limit.bytes) throw tooLarge()
  return new File([blob], name, { type: blob.type })
}

/** Encodes a multipart body (each of `fields`, then `file` as `fileField`) as plain bytes:
 *  WKWebView hands a Blob-backed fetch body to the crowbar:// scheme as a stream Tauri never
 *  drains, so a FormData holding a File reaches the daemon empty; a Uint8Array always arrives. */
export async function encodeMultipart(
  fields: Record<string, string>,
  fileField: string,
  file: File,
): Promise<{ body: Uint8Array<ArrayBuffer>; contentType: string }> {
  const boundary = `----crowbarUpload${nanoid()}`
  const encoder = new TextEncoder()
  const fieldParts = Object.entries(fields).map(
    ([name, value]) =>
      `--${boundary}\r\nContent-Disposition: form-data; name="${name}"\r\n\r\n${value}\r\n`,
  )
  const head = encoder.encode(
    fieldParts.join('') +
      `--${boundary}\r\n` +
      `Content-Disposition: form-data; name="${fileField}"; filename="${escapeFormDataValue(file.name)}"\r\n` +
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

// Go's mime/multipart escapeQuotes, plus CR/LF stripped: neither has an escape in a quoted
// header value, and left in either would split this Content-Disposition line in two.
function escapeFormDataValue(value: string): string {
  return value
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '\\"')
    .replace(/[\r\n]/g, '')
}
