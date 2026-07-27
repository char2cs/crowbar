/**
 * A bounded recording of everything that becomes terminal INPUT, kept so an
 * intermittent corruption can be diagnosed from evidence instead of theory.
 *
 * The open question it exists to answer: when characters duplicate or vanish
 * while typing, does the extra/missing text originate ABOVE the app (WebKit
 * emitting a character through two paths — e.g. a `keypress` and an `input`, or
 * a composition finalise re-flushing the textarea) or BELOW it (a write that is
 * re-sent, dropped, or reordered on its way to the PTY)? Recording both ends of
 * that seam is the only way to tell them apart after the fact:
 *
 *   dom    — raw events on xterm's helper textarea, BEFORE anything interprets
 *            them (keydown/keypress/beforeinput/input/composition*)
 *   write  — a chunk handed to the write buffer, tagged with which code path
 *            produced it (xterm's onData, the Shift+Enter override, …)
 *   send   — a coalesced chunk actually flushed to the transport
 *
 * Purely observational: it never mutates an event, never preventDefaults, and
 * holds only plain data. It is deliberately NOT dev-gated — the bug shows up in
 * real work, so the tape has to be armed there too. One small object per entry
 * (a few per keystroke) against a fixed-size ring is not measurable next to the
 * DOM work each keystroke already does.
 */

/** Entries kept before the oldest is evicted. ~thousands of keystrokes. */
const MAX_ENTRIES = 20_000

export type InputTapeKind = 'dom' | 'write' | 'send'

export interface InputTapeEntry {
  /** ms since page load (performance.now), rounded — enough to spot bursts. */
  t: number
  kind: InputTapeKind
  /** DOM event type, or the write's origin tag. */
  label: string
  /** Payload: the bytes written, or the event's key/data. */
  data?: string
  /** Extra event context (modifiers, inputType, composition state). */
  meta?: Record<string, string | number | boolean>
}

const entries: InputTapeEntry[] = []
let dropped = 0

/** Printable-safe rendering: escapes control bytes so ESC sequences stay legible. */
function readable(data: string): string {
  // eslint-disable-next-line no-control-regex
  return data.replace(/[\u0000-\u001f\u007f]/g, (ch) => {
    if (ch === '\u001b') return '\\e'
    if (ch === '\r') return '\\r'
    if (ch === '\n') return '\\n'
    if (ch === '\t') return '\\t'
    return `\\x${ch.charCodeAt(0).toString(16).padStart(2, '0')}`
  })
}

export function recordInputTape(
  kind: InputTapeKind,
  label: string,
  data?: string,
  meta?: Record<string, string | number | boolean>,
): void {
  if (entries.length >= MAX_ENTRIES) {
    entries.shift()
    dropped++
  }
  entries.push({
    t: Math.round(typeof performance !== 'undefined' ? performance.now() : 0),
    kind,
    label,
    ...(data === undefined ? {} : { data: readable(data) }),
    ...(meta === undefined ? {} : { meta }),
  })
}

export interface InputTapeDump {
  /** Entries evicted by the ring before this dump. */
  dropped: number
  entries: InputTapeEntry[]
}

export function dumpInputTape(): InputTapeDump {
  return { dropped, entries: entries.slice() }
}

export function clearInputTape(): void {
  entries.length = 0
  dropped = 0
}

/**
 * Attach the observational DOM listeners to xterm's helper textarea. Capture
 * phase so an entry exists even for events something later stops, and every
 * listener is passive-by-construction (it only reads).
 */
export function observeInputEvents(textarea: HTMLTextAreaElement): () => void {
  const onKey = (event: Event) => {
    const e = event as KeyboardEvent
    recordInputTape('dom', e.type, e.key, {
      code: e.code,
      keyCode: e.keyCode,
      shift: e.shiftKey,
      alt: e.altKey,
      ctrl: e.ctrlKey,
      meta: e.metaKey,
      repeat: e.repeat,
      composing: e.isComposing,
      // The value xterm's textarea is carrying at this instant: a non-empty one
      // is how a composition finalise could re-flush already-sent text.
      value: (event.currentTarget as HTMLTextAreaElement).value,
    })
  }
  const onInput = (event: Event) => {
    const e = event as InputEvent
    recordInputTape('dom', e.type, e.data ?? '', {
      inputType: e.inputType,
      composing: e.isComposing,
      value: (event.currentTarget as HTMLTextAreaElement).value,
    })
  }
  const onComposition = (event: Event) => {
    const e = event as CompositionEvent
    recordInputTape('dom', e.type, e.data ?? '', {
      value: (event.currentTarget as HTMLTextAreaElement).value,
    })
  }

  const keyTypes = ['keydown', 'keypress', 'keyup']
  const inputTypes = ['beforeinput', 'input']
  const compositionTypes = ['compositionstart', 'compositionupdate', 'compositionend']

  for (const type of keyTypes) textarea.addEventListener(type, onKey, true)
  for (const type of inputTypes) textarea.addEventListener(type, onInput, true)
  for (const type of compositionTypes) textarea.addEventListener(type, onComposition, true)

  return () => {
    for (const type of keyTypes) textarea.removeEventListener(type, onKey, true)
    for (const type of inputTypes) textarea.removeEventListener(type, onInput, true)
    for (const type of compositionTypes) textarea.removeEventListener(type, onComposition, true)
  }
}

/**
 * Expose the dump on `window` so a recording can be read back from the console
 * (or over the automation bridge) the moment a corruption is noticed, without
 * shipping any UI for it. `{ copy: true }` puts the JSON on the clipboard.
 */
export function installInputTapeGlobal(): void {
  if (typeof window === 'undefined') return
  const host = window as unknown as Record<string, unknown>
  host.__crowbarInputTape = (options?: { copy?: boolean; clear?: boolean }) => {
    const dump = dumpInputTape()
    if (options?.copy) void navigator.clipboard?.writeText(JSON.stringify(dump, null, 2))
    if (options?.clear) clearInputTape()
    return dump
  }
}
