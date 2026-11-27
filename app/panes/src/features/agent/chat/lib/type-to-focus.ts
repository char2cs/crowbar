export interface TypeBuffer {
  chars: string
  lastAt: number
}

export const EMPTY_TYPE_BUFFER: TypeBuffer = { chars: '', lastAt: 0 }

const REDIRECT_THRESHOLD = 3
const BURST_GAP_MS = 1000

export interface TypeToFocusKeyEvent {
  key: string
  metaKey: boolean
  ctrlKey: boolean
  altKey: boolean
  isComposing: boolean
}

export type TypeToFocusResult =
  | { action: 'ignore'; buffer: TypeBuffer }
  | { action: 'buffer'; buffer: TypeBuffer }
  | { action: 'redirect'; buffer: TypeBuffer; text: string }

export function advanceTypeToFocus(
  buffer: TypeBuffer,
  event: TypeToFocusKeyEvent,
  now: number,
): TypeToFocusResult {
  const qualifies =
    !event.metaKey &&
    !event.ctrlKey &&
    !event.altKey &&
    !event.isComposing &&
    event.key.length === 1
  if (!qualifies) return { action: 'ignore', buffer }

  const carried = now - buffer.lastAt > BURST_GAP_MS ? '' : buffer.chars
  const chars = carried + event.key
  if (chars.length <= REDIRECT_THRESHOLD)
    return { action: 'buffer', buffer: { chars, lastAt: now } }
  return { action: 'redirect', buffer: EMPTY_TYPE_BUFFER, text: chars }
}

export function isFocusInEditable(active: Element | null): boolean {
  if (!active) return false
  const tag = active.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  return active.closest('[contenteditable="true"]') !== null
}
