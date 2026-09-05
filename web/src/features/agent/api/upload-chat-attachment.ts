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
      ? await fetch(url, { method: 'POST', body: attachmentFormData(input.file, id) })
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

function attachmentFormData(file: File, id: string): FormData {
  const body = new FormData()
  body.append('id', id)
  body.append('file', file, file.name)
  return body
}
