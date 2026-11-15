import { describe, expect, it, vi } from 'vitest'

// Force a deterministic platform so chord formatting/matching is stable in CI.
vi.mock('@/utils/platform', () => ({ IS_MAC: false, IS_MOBILE: false, APPLE_KEYBOARD: false }))

import {
  normalizeChord,
  eventMatchesChord,
  formatChord,
  chordFromEvent,
} from '@/features/keymaps/utils/chord'

describe('normalizeChord', () => {
  it('accepts modifiers in any order and lowercases the key', () => {
    expect(normalizeChord('Shift+Mod+T')).toBe('mod+shift+t')
  })

  it('treats cmd/ctrl/meta and option/opt as their canonical modifiers', () => {
    expect(normalizeChord('cmd+\\')).toBe('mod+\\')
    expect(normalizeChord('option+arrowleft')).toBe('alt+arrowleft')
  })

  it('returns an empty string for empty input or a bare modifier with no key', () => {
    expect(normalizeChord('')).toBe('')
    expect(normalizeChord('mod+shift')).toBe('')
  })

  it('normalizes to a canonical modifier order (mod, shift, alt)', () => {
    expect(normalizeChord('alt+shift+mod+k')).toBe('mod+shift+alt+k')
    expect(normalizeChord('alt+mod+arrowright')).toBe('mod+alt+arrowright')
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

describe('backtick chord', () => {
  it('round-trips through the normalizer, the recorder, the matcher and the label', () => {
    expect(normalizeChord('mod+`')).toBe('mod+`')
    const event = new KeyboardEvent('keydown', { key: '`', ctrlKey: true })
    expect(chordFromEvent(event)).toBe('mod+`')
    expect(eventMatchesChord(event, 'mod+`')).toBe(true)
    expect(formatChord('mod+`')).toBe('Ctrl+`')
  })
})
