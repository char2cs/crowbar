import { chatBase } from '@/features/agent/api/agent-chat-reads'
import { API_BASE } from '@/lib/api/fetch'
import type { MarkdownAssetInfo } from '@/features/editor/markdown/plate/markdown-asset'

/** `chats/{chatId}/attachments/{filename}`; the chatId need not be the open chat. */
export interface ParsedChatAttachmentRef {
  chatId: string
  filename: string
}

const REF_PATTERN = /^chats\/([^/]+)\/attachments\/(.+)$/

export function parseChatAttachmentRef(ref: string): ParsedChatAttachmentRef | null {
  const match = REF_PATTERN.exec(ref)
  if (!match) return null
  const [, chatId, filename] = match
  return { chatId, filename }
}

/** Never throws: `chatBase` throws for an unrecorded workspace scope, and that
 *  becomes `null` like every other failure here. */
export function chatAttachmentUrl(wsId: string, ref: string): string | null {
  const parsed = parseChatAttachmentRef(ref)
  if (!parsed) return null
  try {
    return (
      `${API_BASE}${chatBase(wsId)}/${encodeURIComponent(parsed.chatId)}` +
      `/attachments/${encodeURIComponent(parsed.filename)}`
    )
  } catch {
    return null
  }
}

/** An abort stops the read too: an image unmounted mid-read must not keep encoding. */
function blobToDataUrl(blob: Blob, signal?: AbortSignal): Promise<string> {
  return new Promise((resolve, reject) => {
    if (signal?.aborted) {
      reject(signal.reason)
      return
    }
    const reader = new FileReader()
    const onAbort = () => {
      reader.abort()
      reject(signal?.reason)
    }
    signal?.addEventListener('abort', onAbort, { once: true })
    reader.onload = () => {
      signal?.removeEventListener('abort', onAbort)
      resolve(String(reader.result))
    }
    reader.onerror = () => {
      signal?.removeEventListener('abort', onAbort)
      reject(reader.error ?? new Error('failed to read blob'))
    }
    reader.readAsDataURL(blob)
  })
}

/** Never rejects, so it plugs into `MarkdownAssetInfo.resolve` with no adapter. */
export async function fetchChatAttachmentDataUrl(
  wsId: string,
  ref: string,
  signal?: AbortSignal,
): Promise<string | null> {
  const url = chatAttachmentUrl(wsId, ref)
  if (!url) return null
  try {
    const response = await fetch(url, { signal })
    if (!response.ok) return null
    return await blobToDataUrl(await response.blob(), signal)
  } catch {
    return null
  }
}

export interface ChatAttachmentMetadata {
  filename: string
  /** null when unknown; the file card still renders without a size. */
  size: number | null
}

/** A HEAD, so the card never downloads the body. Never rejects. */
export async function fetchChatAttachmentMetadata(
  wsId: string,
  ref: string,
  signal?: AbortSignal,
): Promise<ChatAttachmentMetadata | null> {
  const parsed = parseChatAttachmentRef(ref)
  const url = chatAttachmentUrl(wsId, ref)
  if (!parsed || !url) return null
  try {
    const response = await fetch(url, { method: 'HEAD', signal })
    if (!response.ok) return { filename: parsed.filename, size: null }
    const len = response.headers.get('content-length')
    return { filename: parsed.filename, size: len ? Number(len) : null }
  } catch {
    return { filename: parsed.filename, size: null }
  }
}

export function chatMarkdownAssetInfo(wsId: string): MarkdownAssetInfo {
  return {
    wsId,
    resolve: (src, signal) => fetchChatAttachmentDataUrl(wsId, src, signal),
  }
}
