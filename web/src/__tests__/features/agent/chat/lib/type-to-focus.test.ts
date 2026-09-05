import { describe, expect, it } from 'vitest'
import {
  advanceTypeToFocus,
  EMPTY_TYPE_BUFFER,
  isFocusInEditable,
} from '@/features/agent/chat/lib/type-to-focus'

const key = (k: string, overrides: Partial<Parameters<typeof advanceTypeToFocus>[1]> = {}) => ({
  key: k,
  metaKey: false,
  ctrlKey: false,
  altKey: false,
  isComposing: false,
  ...overrides,
})

describe('advanceTypeToFocus', () => {
  it('buffers the first three qualifying keys without redirecting', () => {
    let buffer = EMPTY_TYPE_BUFFER
    for (const k of ['h', 'e', 'l']) {
      const result = advanceTypeToFocus(buffer, key(k), 1000)
      expect(result.action).toBe('buffer')
      buffer = result.buffer
    }
    expect(buffer.chars).toBe('hel')
  })

  it('redirects on the fourth qualifying key, carrying all buffered characters', () => {
    let buffer = EMPTY_TYPE_BUFFER
    for (const k of ['h', 'e', 'l']) buffer = advanceTypeToFocus(buffer, key(k), 1000).buffer
    const result = advanceTypeToFocus(buffer, key('l'), 1000)
    expect(result).toEqual({ action: 'redirect', buffer: EMPTY_TYPE_BUFFER, text: 'hell' })
  })

  it('ignores a modifier-held key and does not touch the buffer', () => {
    const buffer = { chars: 'he', lastAt: 1000 }
    const result = advanceTypeToFocus(buffer, key('k', { metaKey: true }), 1010)
    expect(result).toEqual({ action: 'ignore', buffer })
  })

  it('ignores non-printable keys (arrows, Enter, Tab, Escape, Backspace)', () => {
    for (const k of ['ArrowUp', 'Enter', 'Tab', 'Escape', 'Backspace', 'F1', 'Shift']) {
      const result = advanceTypeToFocus(EMPTY_TYPE_BUFFER, key(k), 1000)
      expect(result.action).toBe('ignore')
    }
  })

  it('ignores a key while IME composing', () => {
    const result = advanceTypeToFocus(EMPTY_TYPE_BUFFER, key('a', { isComposing: true }), 1000)
    expect(result.action).toBe('ignore')
  })

  it('does not reset an in-progress buffer on a disqualifying key between qualifying ones', () => {
    // Capitalizing a letter fires a bare "Shift" keydown before the letter's own
    // — that must not wipe out characters already typed earlier in the burst.
    let buffer = advanceTypeToFocus(EMPTY_TYPE_BUFFER, key('a'), 1000).buffer
    buffer = advanceTypeToFocus(buffer, key('Shift'), 1005).buffer
    const result = advanceTypeToFocus(buffer, key('B', { key: 'B' }), 1010)
    expect(result.action).toBe('buffer')
    expect(result.buffer.chars).toBe('aB')
  })

  it('starts a fresh burst when the gap since the last qualifying key exceeds 1s', () => {
    const buffer = { chars: 'he', lastAt: 1000 }
    const result = advanceTypeToFocus(buffer, key('l'), 1000 + 1001)
    expect(result.action).toBe('buffer')
    expect(result.buffer.chars).toBe('l')
  })

  it('does not start a fresh burst exactly at the 1s boundary', () => {
    const buffer = { chars: 'he', lastAt: 1000 }
    const result = advanceTypeToFocus(buffer, key('l'), 1000 + 1000)
    expect(result.buffer.chars).toBe('hel')
  })
})

describe('isFocusInEditable', () => {
  it('is false for null (nothing focused)', () => {
    expect(isFocusInEditable(null)).toBe(false)
  })

  it('is false for document.body', () => {
    expect(isFocusInEditable(document.body)).toBe(false)
  })

  it('is true for an input, textarea or select', () => {
    for (const tag of ['input', 'textarea', 'select']) {
      const el = document.createElement(tag)
      expect(isFocusInEditable(el)).toBe(true)
    }
  })

  it('is true for a contenteditable element', () => {
    const el = document.createElement('div')
    el.setAttribute('contenteditable', 'true')
    expect(isFocusInEditable(el)).toBe(true)
  })

  it('is true for a node nested inside a contenteditable region', () => {
    const host = document.createElement('div')
    host.setAttribute('contenteditable', 'true')
    const span = document.createElement('span')
    host.appendChild(span)
    expect(isFocusInEditable(span)).toBe(true)
  })

  it('is false for a plain div', () => {
    expect(isFocusInEditable(document.createElement('div'))).toBe(false)
  })
})
