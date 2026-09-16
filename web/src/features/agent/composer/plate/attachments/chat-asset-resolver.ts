import { chatBase } from '@/features/agent/api/agent-api'
import { API_BASE } from '@/lib/api'
import type { MarkdownAssetInfo } from '@/features/editor/markdown/plate/markdown-asset'

/** A chat-attachment logical reference, as stored in a message's markdown
 *  text: `chats/{chatId}/attachments/{filename}`. The chatId a ref names
 *  isn't necessarily the chat currently open — parsed straight off the
 *  string, which is always enough to build the serving URL. */
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

/** Built through `chatBase` — the same hierarchical, project/repo-scoped
 *  builder every other chat endpoint in `agent-api.ts` uses — not a flat
 *  `/workspaces/{wsId}/...` path. `chatBase` throws when `wsId`'s
 *  project/repo scope was never recorded (see `workspaceBase`); that is
 *  swallowed here rather than propagated, so this stays a total function
 *  matching its `string | null` signature — same "never throws" contract as
 *  the fetch helpers below. */
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

function blobToDataUrl(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result))
    reader.onerror = () => reject(reader.error ?? new Error('failed to read blob'))
    reader.readAsDataURL(blob)
  })
}

/** Same return contract as `loadLocalImage` (a data: URL, or null) — plugs
 *  straight into `MarkdownAssetInfo.resolve` with no adapter. Never rejects:
 *  a bad ref, an unresolvable workspace scope, a network failure and a
 *  non-ok response all fall back to `null`. */
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
    return await blobToDataUrl(await response.blob())
  } catch {
    return null
  }
}

export interface ChatAttachmentMetadata {
  filename: string
  /** null when the server didn't report Content-Length, or the request
   *  failed — the file card still renders without a size. */
  size: number | null
}

/** Metadata for a file card without downloading the body — a HEAD against
 *  the same URL `fetchChatAttachmentDataUrl` GETs. Never rejects: an
 *  unparseable ref or an unresolvable workspace scope resolves to `null`,
 *  and a network failure or non-ok response still resolves with the
 *  filename and a `null` size rather than throwing. */
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

/** The `MarkdownAssetContext` value for chat: same shape the standalone
 *  markdown file editor provides, but `resolve` routes through the
 *  attachment-serving endpoint instead of a workspace file read. `fileDir`
 *  is unused here — every chat-attachment ref already carries its own
 *  chatId+filename. */
export function chatMarkdownAssetInfo(wsId: string): MarkdownAssetInfo {
  return { wsId, fileDir: '', resolve: (src) => fetchChatAttachmentDataUrl(wsId, src) }
}
