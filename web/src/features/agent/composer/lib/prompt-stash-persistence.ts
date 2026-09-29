import { IS_MAC } from '@/utils/platform'
import { nanoid } from 'nanoid'

/**
 * Durable, client-owned prompt stashes. The payload is the composer's own
 * markdown serialization, so rich document structure and settled attachment
 * references follow the same round trip as an ordinary send.
 */
export interface PromptStash {
  id: string
  markdown: string
  createdAt: string
}

interface StoredPromptStashesV1 {
  version: 1
  items: PromptStash[]
}

const MAX_PROMPT_STASHES = 20
const MAX_PROMPT_STASH_BYTES = 64 * 1024
const MAX_PROMPT_STASH_VALUE_BYTES = 512 * 1024
const MAX_PROMPT_STASHES_TOTAL_BYTES = 2 * 1024 * 1024
const MAX_STASH_ID_LENGTH = 128
const KEY_PREFIX = 'crowbar:agent-prompt-stashes:v1:'

function promptStashStorageKey(wsId: string, chatId: string): string {
  return `${KEY_PREFIX}${encodeURIComponent(wsId)}:${encodeURIComponent(chatId)}`
}

function byteLength(value: string): number {
  return new TextEncoder().encode(value).byteLength
}

function storageBytes(value: string): number {
  return value.length * 2
}

function isPromptStash(value: unknown): value is PromptStash {
  if (!value || typeof value !== 'object') return false
  const item = value as Partial<PromptStash>
  return (
    typeof item.id === 'string' &&
    item.id.length > 0 &&
    item.id.length <= MAX_STASH_ID_LENGTH &&
    typeof item.markdown === 'string' &&
    item.markdown.trim().length > 0 &&
    byteLength(item.markdown) <= MAX_PROMPT_STASH_BYTES &&
    typeof item.createdAt === 'string' &&
    Number.isFinite(Date.parse(item.createdAt))
  )
}

function serialized(items: PromptStash[]): string {
  return JSON.stringify({ version: 1, items } satisfies StoredPromptStashesV1)
}

function isValidCollection(items: PromptStash[]): boolean {
  return items.length <= MAX_PROMPT_STASHES && items.every(isPromptStash)
}

function fitsStorageBudget(key: string, value: string): boolean {
  if (storageBytes(key) + storageBytes(value) > MAX_PROMPT_STASH_VALUE_BYTES) return false
  let total = storageBytes(key) + storageBytes(value)
  for (let index = 0; index < localStorage.length; index++) {
    const storedKey = localStorage.key(index)
    if (!storedKey?.startsWith(KEY_PREFIX) || storedKey === key) continue
    const storedValue = localStorage.getItem(storedKey)
    if (storedValue === null) continue
    total += storageBytes(storedKey) + storageBytes(storedValue)
    if (total > MAX_PROMPT_STASHES_TOTAL_BYTES) return false
  }
  return total <= MAX_PROMPT_STASHES_TOTAL_BYTES
}

/**
 * A local blob URL is only an optimistic upload preview. It cannot survive a
 * reload and must never be made to look like a completed chat attachment.
 * Preserve its place in the document as an explicit note instead.
 *
 * Likewise, an attachment-looking `chats/...` reference that does not match
 * Crowbar's durable reference grammar is retained as an explicit invalid
 * reference, not upgraded into a working attachment.
 */
function makePromptStashMarkdown(markdown: string): string {
  return markdown.replace(
    /(!?)\[([^\]]*)\]\(([^)]+)\)/g,
    (whole, imageMarker: string, label: string, ref: string) => {
      const normalizedRef = ref.trim().replace(/^<|>$/g, '')
      if (normalizedRef.startsWith('blob:')) {
        const kind = imageMarker ? 'image' : 'file'
        return `> ${kind === 'image' ? 'Image' : 'File'} “${label || 'unnamed'}” was still uploading when this draft was stashed. Attach it again before sending.`
      }
      if (
        normalizedRef.startsWith('chats/') &&
        !/^chats\/[^/]+\/attachments\/.+/.test(normalizedRef)
      ) {
        return `> Attachment “${label || 'unnamed'}” had an invalid Crowbar reference when this draft was stashed. Attach it again before sending.`
      }
      return whole
    },
  )
}

export function createPromptStash(
  markdown: string,
  options: { id?: string; now?: Date } = {},
): PromptStash | null {
  const prepared = makePromptStashMarkdown(markdown).trim()
  if (!prepared || byteLength(prepared) > MAX_PROMPT_STASH_BYTES) return null
  const id = options.id ?? nanoid()
  const item = {
    id,
    markdown: prepared,
    createdAt: (options.now ?? new Date()).toISOString(),
  }
  return isPromptStash(item) ? item : null
}

/** Newest first, bounded. */
export function addPromptStash(items: PromptStash[], item: PromptStash): PromptStash[] {
  return [item, ...items.filter((candidate) => candidate.id !== item.id)].slice(
    0,
    MAX_PROMPT_STASHES,
  )
}

export function loadPromptStashes(wsId: string, chatId: string): PromptStash[] {
  const key = promptStashStorageKey(wsId, chatId)
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return []
    if (storageBytes(key) + storageBytes(raw) > MAX_PROMPT_STASH_VALUE_BYTES) {
      localStorage.removeItem(key)
      return []
    }
    const parsed = JSON.parse(raw) as Partial<StoredPromptStashesV1>
    if (parsed.version !== 1 || !Array.isArray(parsed.items) || !isValidCollection(parsed.items)) {
      localStorage.removeItem(key)
      return []
    }
    return parsed.items
  } catch {
    try {
      localStorage.removeItem(key)
    } catch {
      // Storage itself is unavailable.
    }
    return []
  }
}

/** False means the caller must keep the current draft untouched. */
export function savePromptStashes(wsId: string, chatId: string, items: PromptStash[]): boolean {
  if (!isValidCollection(items)) return false
  const key = promptStashStorageKey(wsId, chatId)
  try {
    if (items.length === 0) {
      localStorage.removeItem(key)
      return true
    }
    const value = serialized(items)
    if (!fitsStorageBudget(key, value)) return false
    localStorage.setItem(key, value)
    return true
  } catch {
    return false
  }
}

interface StashShortcutEvent {
  key: string
  metaKey: boolean
  ctrlKey: boolean
  shiftKey: boolean
  altKey: boolean
  repeat: boolean
}

export function matchesPromptStashShortcut(event: StashShortcutEvent, macOS = IS_MAC): boolean {
  const modifier = macOS ? event.metaKey && !event.ctrlKey : event.ctrlKey && !event.metaKey
  return (
    modifier && !event.shiftKey && !event.altKey && !event.repeat && event.key.toLowerCase() === 's'
  )
}
