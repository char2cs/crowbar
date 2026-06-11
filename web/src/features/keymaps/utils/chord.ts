/**
 * Pure chord parsing, formatting and matching.
 *
 * Normalized chord string grammar (lowercase, `+`-separated, modifiers first in
 * canonical order): `[mod+][shift+][alt+]<key>`
 *
 *  - `mod`   → Cmd on macOS, Ctrl elsewhere
 *  - `shift` → Shift
 *  - `alt`   → Alt / Option
 *  - `<key>` → a single key, lowercased (e.g. `t`, `\`, `arrowleft`)
 *
 * Examples: `mod+\`, `mod+shift+t`, `mod+alt+arrowleft`.
 */
import { IS_MAC } from '@/utils/platform'

export interface ParsedChord {
  mod: boolean
  shift: boolean
  alt: boolean
  key: string
}

const MOD_ORDER = ['mod', 'shift', 'alt'] as const

/** Parse a normalized chord string into structured form. Returns null if empty. */
export function parseChord(chord: string): ParsedChord | null {
  const parts = chord
    .trim()
    .toLowerCase()
    .split('+')
    .map((p) => p.trim())
    .filter(Boolean)
  if (parts.length === 0) return null

  const result: ParsedChord = { mod: false, shift: false, alt: false, key: '' }
  for (const part of parts) {
    if (
      part === 'mod' ||
      part === 'cmd' ||
      part === 'ctrl' ||
      part === 'control' ||
      part === 'meta'
    ) {
      result.mod = true
    } else if (part === 'shift') {
      result.shift = true
    } else if (part === 'alt' || part === 'option' || part === 'opt') {
      result.alt = true
    } else {
      result.key = part
    }
  }
  if (!result.key) return null
  return result
}

/** Produce the canonical normalized chord string from structured form. */
export function stringifyChord(parsed: ParsedChord): string {
  const out: string[] = []
  if (parsed.mod) out.push('mod')
  if (parsed.shift) out.push('shift')
  if (parsed.alt) out.push('alt')
  out.push(parsed.key.toLowerCase())
  return out.join('+')
}

/** Normalize an arbitrary chord string to canonical form (stable modifier order). */
export function normalizeChord(chord: string): string {
  const parsed = parseChord(chord)
  return parsed ? stringifyChord(parsed) : ''
}

/**
 * Build a normalized chord from a KeyboardEvent. Returns null when the event is
 * only a bare modifier press (no real key yet).
 */
export function chordFromEvent(e: KeyboardEvent): string | null {
  const key = e.key
  if (key === 'Control' || key === 'Shift' || key === 'Alt' || key === 'Meta') {
    return null
  }
  const mod = IS_MAC ? e.metaKey : e.ctrlKey
  const parsed: ParsedChord = {
    mod,
    shift: e.shiftKey,
    alt: e.altKey,
    key: key.toLowerCase(),
  }
  return stringifyChord(parsed)
}

/** True when a KeyboardEvent matches the given normalized chord. */
export function eventMatchesChord(e: KeyboardEvent, chord: string): boolean {
  const parsed = parseChord(chord)
  if (!parsed) return false
  const mod = IS_MAC ? e.metaKey : e.ctrlKey
  if (parsed.mod !== mod) return false
  if (parsed.shift !== e.shiftKey) return false
  if (parsed.alt !== e.altKey) return false
  return e.key.toLowerCase() === parsed.key
}

const KEY_LABELS: Record<string, string> = {
  arrowleft: '←',
  arrowright: '→',
  arrowup: '↑',
  arrowdown: '↓',
  '\\': '\\',
}

/** Human-readable label for a chord, platform-aware (⌘/Ctrl, ⇧, ⌥). */
export function formatChord(chord: string): string {
  const parsed = parseChord(chord)
  if (!parsed) return ''
  const parts: string[] = []
  if (parsed.mod) parts.push(IS_MAC ? '⌘' : 'Ctrl')
  if (parsed.shift) parts.push(IS_MAC ? '⇧' : 'Shift')
  if (parsed.alt) parts.push(IS_MAC ? '⌥' : 'Alt')
  const keyLabel = KEY_LABELS[parsed.key] ?? parsed.key.toUpperCase()
  parts.push(keyLabel)
  return IS_MAC ? parts.join('') : parts.join('+')
}

export { MOD_ORDER }
