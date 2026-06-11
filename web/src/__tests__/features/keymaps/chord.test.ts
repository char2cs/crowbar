import { describe, expect, it, vi } from 'vitest'

// Force a deterministic platform so chord formatting/matching is stable in CI.
vi.mock('@/utils/platform', () => ({ IS_MAC: false }))

import {
  parseChord,
  stringifyChord,
  normalizeChord,
  eventMatchesChord,
  formatChord,
} from '@/features/keymaps/utils/chord'

describe('chord parse/stringify', () => {
  it('parses modifiers in any order and lowercases the key', () => {
    expect(parseChord('Shift+Mod+T')).toEqual({ mod: true, shift: true, alt: false, key: 't' })
  })

  it('treats cmd/ctrl/meta and option/opt as their canonical modifiers', () => {
    expect(parseChord('cmd+\\')).toEqual({ mod: true, shift: false, alt: false, key: '\\' })
    expect(parseChord('option+arrowleft')).toEqual({
      mod: false,
      shift: false,
      alt: true,
      key: 'arrowleft',
    })
  })

  it('returns null for empty input or a bare modifier with no key', () => {
    expect(parseChord('')).toBeNull()
    expect(parseChord('mod+shift')).toBeNull()
  })

  it('normalizes to a canonical modifier order (mod, shift, alt)', () => {
    expect(normalizeChord('alt+shift+mod+k')).toBe('mod+shift+alt+k')
    expect(stringifyChord({ mod: true, shift: false, alt: true, key: 'arrowright' })).toBe(
      'mod+alt+arrowright',
    )
  })
})

describe('eventMatchesChord (non-mac: mod = ctrl)', () => {
  const ev = (init: Partial<KeyboardEvent>) => init as KeyboardEvent

  it('matches when ctrl maps to mod and key + modifiers line up', () => {
    expect(
      eventMatchesChord(ev({ key: '\\', ctrlKey: true, shiftKey: false, altKey: false }), 'mod+\\'),
    ).toBe(true)
  })

  it('does not match when a modifier differs', () => {
    expect(
      eventMatchesChord(ev({ key: '\\', ctrlKey: true, shiftKey: true, altKey: false }), 'mod+\\'),
    ).toBe(false)
    // metaKey must NOT satisfy mod off-mac
    expect(
      eventMatchesChord(ev({ key: '\\', metaKey: true, ctrlKey: false, altKey: false }), 'mod+\\'),
    ).toBe(false)
  })
})

describe('formatChord (non-mac labels)', () => {
  it('renders Ctrl/Shift/Alt with arrow glyphs and a + separator', () => {
    expect(formatChord('mod+shift+t')).toBe('Ctrl+Shift+T')
    expect(formatChord('mod+alt+arrowleft')).toBe('Ctrl+Alt+←')
  })
})
