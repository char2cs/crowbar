/**
 * Typing on an unfocused chat should land in the composer, the way it already
 * does in every native chat surface people compare this one to — but only once
 * it's clearly a sentence starting, not a single bare-key shortcut elsewhere in
 * the app being pressed. Buffering a few keys before redirecting is what tells
 * the two apart; see `advanceTypeToFocus`'s own doc for the exact rule.
 */

export interface TypeBuffer {
  chars: string
  lastAt: number
}

export const EMPTY_TYPE_BUFFER: TypeBuffer = { chars: '', lastAt: 0 }

/** Redirect once buffered length exceeds this — i.e. on the 4th key. */
const REDIRECT_THRESHOLD = 3
/** A gap this long since the last qualifying key starts a fresh burst, so two
 *  disconnected keystrokes minutes apart never combine into one redirect. */
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

/**
 * One keydown's worth of the "type anywhere, land in the composer" state
 * machine.
 *
 * A key only ever QUALIFIES if it is a single printable character typed with
 * no modifier and no IME composition in flight — that excludes every
 * navigation/shortcut key (arrows, Enter, Tab, Escape, function keys, a bare
 * Shift/Control/Alt/Meta press) by construction, since none of those report a
 * one-character `key`.
 *
 * A disqualifying key does NOT reset an in-progress buffer: capitalizing a
 * letter fires a bare "Shift" keydown before the letter's own, and treating
 * that as burst-ending would corrupt "aB" typed with a shift-held capital
 * into just "B".
 */
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

/** Whether focus already lives somewhere text can be typed — the composer
 *  itself, or (just as importantly) any OTHER field elsewhere in the app that
 *  this feature must never steal a keystroke from. */
export function isFocusInEditable(active: Element | null): boolean {
  if (!active) return false
  const tag = active.tagName
  if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return true
  return active.closest('[contenteditable="true"]') !== null
}
