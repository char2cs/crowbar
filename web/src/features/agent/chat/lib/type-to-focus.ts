// Buffering a few keys before redirecting tells a sentence starting apart from
// a bare-key shortcut elsewhere in the app.

export interface TypeBuffer {
  chars: string
  lastAt: number
}

export const EMPTY_TYPE_BUFFER: TypeBuffer = { chars: '', lastAt: 0 }

/** Redirect once buffered length exceeds this — i.e. on the 4th key. */
const REDIRECT_THRESHOLD = 3
/** A gap this long since the last qualifying key starts a fresh burst. */
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

/** Only an unmodified single printable character qualifies. A disqualifying key
 *  does not reset the buffer: a capital's bare "Shift" keydown would drop "a" from "aB". */
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

/** Any field, not just the composer: this must never steal a keystroke. */
export function isFocusInEditable(active: Element | null): boolean {
  if (!active) return false
  const tag = active.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  return active.closest('[contenteditable="true"]') !== null
}
