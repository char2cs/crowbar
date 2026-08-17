/**
 * Prompt drafts are client-owned, but they must survive a desktop reload. Keep
 * the persistence boundary deliberately tiny and versioned so replacing it with
 * a server queue later does not leak storage concerns into the chat UI.
 */

export type PromptQueueState =
  | 'queued'
  | 'submitting'
  | 'awaiting_turn'
  | 'failed'
  | 'outcome_uncertain'

export interface PromptQueueItem {
  clientRequestId: string
  text: string
  state: PromptQueueState
  createdAt: string
  /** Set once, on the first network attempt. Retries keep the same evidence window. */
  submittedAt?: string
  baselineSequence: number
  error?: string
  /** A busy response cannot be retried until server state has crossed idle. */
  waitForIdleEpoch?: number
}

interface StoredQueueV1 {
  version: 1
  items: PromptQueueItem[]
}

export const MAX_PROMPT_QUEUE_ITEMS = 100
export const MAX_PROMPT_TEXT_BYTES = 64 * 1024
// Web Storage quota is shared by the whole origin. Keep one queue comfortably
// below the common multi-megabyte ceiling and all Crowbar prompt queues below a
// separate aggregate budget, accounting in UTF-16 bytes (the conservative unit
// browsers commonly charge). Actual storage denial remains detectable.
export const MAX_PROMPT_QUEUE_VALUE_BYTES = 256 * 1024
export const MAX_PROMPT_QUEUES_TOTAL_BYTES = 1024 * 1024
const MAX_REQUEST_ID_LENGTH = 128
const MAX_ERROR_LENGTH = 2_048
const KEY_PREFIX = 'crowbar:agent-prompt-queue:v1:'
const STATES = new Set<PromptQueueState>([
  'queued',
  'submitting',
  'awaiting_turn',
  'failed',
  'outcome_uncertain',
])

export function promptQueueStorageKey(wsId: string, chatId: string): string {
  return `${KEY_PREFIX}${encodeURIComponent(wsId)}:${encodeURIComponent(chatId)}`
}

function byteLength(value: string): number {
  return new TextEncoder().encode(value).byteLength
}

function storageBytes(value: string): number {
  return value.length * 2
}

function serializedQueue(items: PromptQueueItem[]): string {
  return JSON.stringify({ version: 1, items } satisfies StoredQueueV1)
}

function fitsStorageBudget(key: string, serialized: string): boolean {
  if (storageBytes(key) + storageBytes(serialized) > MAX_PROMPT_QUEUE_VALUE_BYTES) return false
  let total = storageBytes(key) + storageBytes(serialized)
  for (let index = 0; index < localStorage.length; index++) {
    const storedKey = localStorage.key(index)
    if (!storedKey?.startsWith(KEY_PREFIX) || storedKey === key) continue
    const storedValue = localStorage.getItem(storedKey)
    if (storedValue === null) continue
    total += storageBytes(storedKey) + storageBytes(storedValue)
    if (total > MAX_PROMPT_QUEUES_TOTAL_BYTES) return false
  }
  return total <= MAX_PROMPT_QUEUES_TOTAL_BYTES
}

export function isPromptTextWithinLimit(value: string): boolean {
  return value.trim().length > 0 && byteLength(value) <= MAX_PROMPT_TEXT_BYTES
}

/** Whether this complete queue fits Crowbar's reserved Web Storage budget.
 * This is a preflight only; savePromptQueue still reports browser quota or
 * privacy-mode failures from the actual write. */
export function canPersistPromptQueue(
  wsId: string,
  chatId: string,
  items: PromptQueueItem[],
): boolean {
  if (items.length > MAX_PROMPT_QUEUE_ITEMS || !items.every(isQueueItem)) return false
  try {
    return fitsStorageBudget(promptQueueStorageKey(wsId, chatId), serializedQueue(items))
  } catch {
    return false
  }
}

function isQueueItem(value: unknown): value is PromptQueueItem {
  if (!value || typeof value !== 'object') return false
  const item = value as Partial<PromptQueueItem>
  return (
    typeof item.clientRequestId === 'string' &&
    item.clientRequestId.length > 0 &&
    item.clientRequestId.length <= MAX_REQUEST_ID_LENGTH &&
    typeof item.text === 'string' &&
    isPromptTextWithinLimit(item.text) &&
    typeof item.state === 'string' &&
    STATES.has(item.state as PromptQueueState) &&
    typeof item.createdAt === 'string' &&
    Number.isFinite(Date.parse(item.createdAt)) &&
    (item.submittedAt === undefined ||
      (typeof item.submittedAt === 'string' && Number.isFinite(Date.parse(item.submittedAt)))) &&
    typeof item.baselineSequence === 'number' &&
    Number.isSafeInteger(item.baselineSequence) &&
    item.baselineSequence >= 0 &&
    (item.error === undefined ||
      (typeof item.error === 'string' && item.error.length <= MAX_ERROR_LENGTH)) &&
    (item.waitForIdleEpoch === undefined ||
      (typeof item.waitForIdleEpoch === 'number' &&
        Number.isSafeInteger(item.waitForIdleEpoch) &&
        item.waitForIdleEpoch >= 0))
  )
}

/** Rehydrate only a fully valid bounded document. A browser/app exit while the
 * request was in flight has an unknowable outcome, so `submitting` is promoted
 * to a user-driven recovery state and is never replayed automatically. */
export function loadPromptQueue(wsId: string, chatId: string): PromptQueueItem[] {
  const key = promptQueueStorageKey(wsId, chatId)
  try {
    const raw = localStorage.getItem(key)
    if (!raw) return []
    if (storageBytes(key) + storageBytes(raw) > MAX_PROMPT_QUEUE_VALUE_BYTES) {
      localStorage.removeItem(key)
      return []
    }
    const parsed = JSON.parse(raw) as Partial<StoredQueueV1>
    if (
      parsed.version !== 1 ||
      !Array.isArray(parsed.items) ||
      parsed.items.length > MAX_PROMPT_QUEUE_ITEMS ||
      !parsed.items.every(isQueueItem)
    ) {
      localStorage.removeItem(key)
      return []
    }
    return parsed.items.map((item) =>
      item.state === 'submitting'
        ? {
            ...item,
            state: 'outcome_uncertain' as const,
            error:
              'Crowbar closed while this prompt was being submitted. Check the chat before retrying.',
          }
        : item,
    )
  } catch {
    // Corrupt JSON and denied storage are both non-fatal. Best-effort removal
    // prevents repeatedly parsing the same bad document on every mount.
    try {
      localStorage.removeItem(key)
    } catch {
      // Storage itself is unavailable.
    }
    return []
  }
}

/** Returns false when persistence is unavailable/quota-limited. The in-memory
 * queue remains usable and the caller can surface that durability was lost. */
export function savePromptQueue(wsId: string, chatId: string, items: PromptQueueItem[]): boolean {
  if (items.length > MAX_PROMPT_QUEUE_ITEMS || !items.every(isQueueItem)) return false
  const key = promptQueueStorageKey(wsId, chatId)
  try {
    if (items.length === 0) localStorage.removeItem(key)
    else {
      const serialized = serializedQueue(items)
      if (!fitsStorageBudget(key, serialized)) return false
      localStorage.setItem(key, serialized)
    }
    return true
  } catch {
    return false
  }
}

export function clearPersistedPromptQueue(wsId: string, chatId: string): void {
  try {
    localStorage.removeItem(promptQueueStorageKey(wsId, chatId))
  } catch {
    // Queue cleanup must never turn a successful server deletion into a UI error.
  }
}
