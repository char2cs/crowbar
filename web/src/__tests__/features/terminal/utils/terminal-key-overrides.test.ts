import { describe, it, expect } from 'vitest'
import { resolveKeyOverride } from '@/features/terminal/utils/terminal-key-overrides'

const kd = (init: Partial<KeyboardEvent>): KeyboardEvent =>
  ({
    type: 'keydown',
    key: 'a',
    shiftKey: false,
    altKey: false,
    ctrlKey: false,
    metaKey: false,
    ...init,
  }) as KeyboardEvent

describe('resolveKeyOverride', () => {
  it('disambiguates Shift+Enter and Alt+Enter (xterm.js cannot)', () => {
    expect(resolveKeyOverride(kd({ key: 'Enter', shiftKey: true }))).toBe('\x1b[13;2u')
    expect(resolveKeyOverride(kd({ key: 'Enter', altKey: true }))).toBe('\x1b[13;3u')
  })

  it('does NOT override plain Enter (xterm sends CR correctly)', () => {
    expect(resolveKeyOverride(kd({ key: 'Enter' }))).toBeNull()
  })

  it('does NOT override keys xterm already handles (no double-fire)', () => {
    // these must fall through to xterm: Ctrl+U, Shift+Tab, Option+arrows, Backspace
    expect(resolveKeyOverride(kd({ key: 'u', ctrlKey: true }))).toBeNull()
    expect(resolveKeyOverride(kd({ key: 'Tab', shiftKey: true }))).toBeNull()
    expect(resolveKeyOverride(kd({ key: 'ArrowLeft', altKey: true }))).toBeNull()
    expect(resolveKeyOverride(kd({ key: 'Backspace', altKey: true }))).toBeNull()
  })

  it('ignores Cmd/Ctrl + Enter (app/OS shortcuts, not text input)', () => {
    expect(resolveKeyOverride(kd({ key: 'Enter', metaKey: true, shiftKey: true }))).toBeNull()
    expect(resolveKeyOverride(kd({ key: 'Enter', ctrlKey: true, shiftKey: true }))).toBeNull()
  })

  it('ignores non-keydown events (avoids double on keypress/keyup)', () => {
    expect(resolveKeyOverride(kd({ type: 'keyup', key: 'Enter', shiftKey: true }))).toBeNull()
    expect(resolveKeyOverride(kd({ type: 'keypress', key: 'Enter', shiftKey: true }))).toBeNull()
  })
})
