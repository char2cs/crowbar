import { IS_MAC } from '@/utils/platform'
import { nanoid } from 'nanoid'
import { readLocal, removeLocal, writeLocal } from '@/lib/local-storage'

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
  let total = storageBytes(key) + storageBytes(value)
  if (total > MAX_PROMPT_STASH_VALUE_BYTES) return false
  for (const [storedKey, storedValue] of otherStashes(key)) {
    total += storageBytes(storedKey) + storageBytes(storedValue)
    if (total > MAX_PROMPT_STASHES_TOTAL_BYTES) return false
  }
  return true
}

/** Every other chat's stash; none when storage is unavailable. */
function otherStashes(key: string): [string, string][] {
  const found: [string, string][] = []
  try {
    for (let index = 0; index < localStorage.length; index++) {
      const stored = localStorage.key(index)
      if (!stored?.startsWith(KEY_PREFIX) || stored === key) continue
      const stash = localStorage.getItem(stored)
      if (stash !== null) found.push([stored, stash])
    }
  } catch {
    return []
  }
  return found
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
  const raw = readLocal(key)
  if (!raw) return []
  const items =
    storageBytes(key) + storageBytes(raw) > MAX_PROMPT_STASH_VALUE_BYTES ? null : parsedItems(raw)
  if (items) return items
  removeLocal(key)
  return []
}

function parsedItems(raw: string): PromptStash[] | null {
  try {
    const parsed = JSON.parse(raw) as Partial<StoredPromptStashesV1>
    if (parsed.version !== 1 || !Array.isArray(parsed.items)) return null
    return isValidCollection(parsed.items) ? parsed.items : null
  } catch {
    return null
  }
}

/** False means the caller must keep the current draft untouched. */
export function savePromptStashes(wsId: string, chatId: string, items: PromptStash[]): boolean {
  if (!isValidCollection(items)) return false
  const key = promptStashStorageKey(wsId, chatId)
  if (items.length === 0) {
    removeLocal(key)
    return true
  }
  const value = serialized(items)
  return fitsStorageBudget(key, value) && writeLocal(key, value)
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
