import { APPLE_KEYBOARD } from '@/utils/platform'

interface ParsedChord {
  mod: boolean
  shift: boolean
  alt: boolean
  key: string
}

function parseChord(chord: string): ParsedChord | null {
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

function stringifyChord(parsed: ParsedChord): string {
  const out: string[] = []
  if (parsed.mod) out.push('mod')
  if (parsed.shift) out.push('shift')
  if (parsed.alt) out.push('alt')
  out.push(parsed.key.toLowerCase())
  return out.join('+')
}

export function normalizeChord(chord: string): string {
  const parsed = parseChord(chord)
  return parsed ? stringifyChord(parsed) : ''
}

export function chordFromEvent(e: KeyboardEvent): string | null {
  const key = e.key
  if (key === 'Control' || key === 'Shift' || key === 'Alt' || key === 'Meta') {
    return null
  }
  const mod = APPLE_KEYBOARD ? e.metaKey : e.ctrlKey
  const parsed: ParsedChord = {
    mod,
    shift: e.shiftKey,
    alt: e.altKey,
    key: key.toLowerCase(),
  }
  return stringifyChord(parsed)
}

export function eventMatchesChord(e: KeyboardEvent, chord: string): boolean {
  const parsed = parseChord(chord)
  if (!parsed) return false
  const mod = APPLE_KEYBOARD ? e.metaKey : e.ctrlKey
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

export function formatChord(chord: string): string {
  const parsed = parseChord(chord)
  if (!parsed) return ''
  const parts: string[] = []
  if (parsed.mod) parts.push(APPLE_KEYBOARD ? '⌘' : 'Ctrl')
  if (parsed.shift) parts.push(APPLE_KEYBOARD ? '⇧' : 'Shift')
  if (parsed.alt) parts.push(APPLE_KEYBOARD ? '⌥' : 'Alt')
  const keyLabel = KEY_LABELS[parsed.key] ?? parsed.key.toUpperCase()
  parts.push(keyLabel)
  return APPLE_KEYBOARD ? parts.join('') : parts.join('+')
}
