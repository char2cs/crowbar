import { chatBase } from '@/features/agent/api/agent-chat-reads'
import { apiUrl } from '@/lib/api/fetch'
import type { MarkdownAssetInfo } from '@/features/editor/markdown/plate/markdown-asset'

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

export function chatAttachmentUrl(wsId: string, ref: string): string | null {
  const parsed = parseChatAttachmentRef(ref)
  if (!parsed) return null
  try {
    return apiUrl(
      `${chatBase(wsId)}/${encodeURIComponent(parsed.chatId)}` +
        `/attachments/${encodeURIComponent(parsed.filename)}`,
    )
  } catch {
    return null
  }
}

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

async function fetchChatAttachmentDataUrl(
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
  size: number | null
}

export async function fetchChatAttachmentMetadata(
  wsId: string,
  ref: string,
): Promise<ChatAttachmentMetadata | null> {
  const parsed = parseChatAttachmentRef(ref)
  const url = chatAttachmentUrl(wsId, ref)
  if (!parsed || !url) return null
  try {
    const response = await fetch(url, { method: 'HEAD' })
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
