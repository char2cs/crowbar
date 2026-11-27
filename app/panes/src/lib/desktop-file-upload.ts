import { nanoid } from 'nanoid'
import { convertFileSrc } from '@/lib/crowbar-bridge'
import { UserFacingError } from '@/lib/errors/user-facing-error'

export interface UploadLimit {
  bytes: number
  label: string
}

export async function readDesktopFile(path: string, limit: UploadLimit): Promise<File> {
  const name = path.split(/[\\/]/).pop() || 'file'
  const tooLarge = () => new UserFacingError(`${name} is larger than the ${limit.label} limit`)
  const unreadable = (cause?: unknown) =>
    new UserFacingError(`could not read ${name} to send it to the remote host`, { cause })
  const src = convertFileSrc(path)
  const size = await sizeOf(src)
  if (size !== null && size > limit.bytes) throw tooLarge()
  const res = await fetch(src).catch((err: unknown) => {
    throw unreadable(err)
  })
  if (!res.ok) throw unreadable()
  const blob = await res.blob()
  if (blob.size > limit.bytes) throw tooLarge()
  return new File([blob], name, { type: blob.type })
}

async function sizeOf(src: string): Promise<number | null> {
  try {
    const head = await fetch(src, { method: 'HEAD' })
    return head.ok ? Number(head.headers.get('content-length')) : null
  } catch {
    return null
  }
}

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

function escapeFormDataValue(value: string): string {
  return value
    .replace(/\\/g, '\\\\')
    .replace(/"/g, '\\"')
    .replace(/[\r\n]/g, '')
}
