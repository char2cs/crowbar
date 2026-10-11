import { nanoid } from 'nanoid'
import { chatBase } from '@/features/agent/api/agent-api'
import { LOCAL_HOST } from '@/features/hosts/types'
import { apiUrl } from '@/lib/api'
import { ApiError, PROXY_HEADER, proxyMarkerError } from '@/lib/api/errors'
import { encodeMultipart, readDesktopFile } from '@/lib/desktop-file-upload'
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

// A remote host's failure is named the way apiFetch names it, so its trouble reads as that
// host's; the local daemon's reads exactly as it always has.
async function uploadFailure(response: Response, base: string): Promise<Error> {
  const { hostId } = splitHostPath(base)
  const message = `upload chat attachment failed: ${response.status} ${await response.text()}`
  if (hostId === LOCAL_HOST.id) return new Error(message)
  const marked = proxyMarkerError(response.headers.get(PROXY_HEADER), hostId, response.status)
  return marked ?? new ApiError(message, response.status, undefined, undefined, hostId)
}

// The daemon's attachments.MaxBytes is the authority on the cap.
const ATTACHMENT_LIMIT = { bytes: 25 * 1024 * 1024, label: '25 MB attachment' }

// The bytes of `path` for a chat whose host (named by its qualified `base`) is remote,
// or null for a local chat, whose daemon reads the path itself.
async function fileForRemoteChat(base: string, path: string): Promise<File | null> {
  if (splitHostPath(base).hostId === LOCAL_HOST.id) return null
  return readDesktopFile(path, ATTACHMENT_LIMIT)
}

async function postMultipartAttachment(url: string, file: File, id: string): Promise<Response> {
  const { body, contentType } = await encodeMultipart({ id }, 'file', file)
  return fetch(url, { method: 'POST', headers: { 'Content-Type': contentType }, body })
}
