import { nanoid } from 'nanoid'
import { chatBase } from '@/features/agent/api/agent-chat-reads'
import { LOCAL_HOST } from '@/features/hosts/types'
import { apiFetchRaw } from '@/lib/api/fetch'
import { progressFetch } from '@/lib/api/xhr-transport'
import { encodeMultipart, readDesktopFile } from '@/lib/desktop-file-upload'
import { splitHostPath } from '@/lib/host-path'

export interface UploadedChatAttachment {
  ref: string
  filename: string
  size: number
  contentType: string
}

export type UploadChatAttachmentInput = { file: File } | { path: string }

export interface UploadOptions {
  /** Bytes of the file sent so far, and its size; only a file sent as bytes reports. */
  onProgress?: (loaded: number, total: number) => void
  signal?: AbortSignal
}

interface UploadAttachmentResponseBody {
  data: { ref: string; fileName: string; size: number; contentType: string }
}

/** Multipart bytes for a `File`, or JSON `{path, id}` for a desktop drop the daemon reads itself;
 *  a remote daemon can't read this disk, so a remote chat's path is read here and sent as bytes.
 *  Pass `id` only when the caller must reuse it. */
export async function uploadChatAttachment(
  wsId: string,
  chatId: string,
  input: UploadChatAttachmentInput,
  id: string = nanoid(),
  options: UploadOptions = {},
): Promise<UploadedChatAttachment> {
  const base = chatBase(wsId)
  const path = `${base}/${encodeURIComponent(chatId)}/attachments`
  const file = 'file' in input ? input.file : await fileForRemoteChat(base, input.path)
  const response = file
    ? await postMultipartAttachment(path, file, id, options)
    : await apiFetchRaw(path, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: (input as { path: string }).path, id }),
        signal: options.signal,
      })
  const { data } = (await response.json()) as UploadAttachmentResponseBody
  return { ref: data.ref, filename: data.fileName, size: data.size, contentType: data.contentType }
}

// The daemon's attachments.MaxBytes is the authority on the cap.
const ATTACHMENT_LIMIT = { bytes: 25 * 1024 * 1024, label: '25 MB attachment' }

// The bytes of `path` for a chat whose host (named by its qualified `base`) is remote,
// or null for a local chat, whose daemon reads the path itself.
async function fileForRemoteChat(base: string, path: string): Promise<File | null> {
  if (splitHostPath(base).hostId === LOCAL_HOST.id) return null
  return readDesktopFile(path, ATTACHMENT_LIMIT)
}

async function postMultipartAttachment(
  path: string,
  file: File,
  id: string,
  { onProgress, signal }: UploadOptions,
): Promise<Response> {
  const { body, contentType } = await encodeMultipart({ id }, 'file', file)
  const init = { method: 'POST', headers: { 'Content-Type': contentType }, body, signal }
  if (!onProgress) return apiFetchRaw(path, init)
  // The body carries multipart framing: report the share of it that is the file.
  const send = progressFetch((loaded, total) =>
    onProgress(Math.min(file.size, Math.round((loaded / total) * file.size)), file.size),
  )
  return apiFetchRaw(path, init, undefined, undefined, send)
}
