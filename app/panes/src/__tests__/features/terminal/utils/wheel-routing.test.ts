import { describe, it, expect } from 'vitest'
import { shouldScrollScrollback } from '@/features/terminal/utils/wheel-routing'

describe('shouldScrollScrollback', () => {
  // THE REGRESSION: a mouse-tracking app on the PRIMARY buffer. The daemon
  // serializes DECSET 1003 (mouse) without 1049 (alt) whenever an app tracks
  // the mouse on the primary buffer — Claude Code's TUI lands exactly here:
  // (mode 'any', buffer 'normal'). A plain wheel must NOT be intercepted; it
  // must be forwarded to the app. (The old buffer-type logic returned "intercept"
  // here, eating the wheel — "works for a moment, then stuck".)
  it('does NOT intercept a plain wheel for a tracking app on the primary buffer (the regression)', () => {
    expect(shouldScrollScrollback({ shiftKey: false }, 'any', 'normal')).toBe(false)
  })

  it('intercepts Shift+wheel on the primary buffer to scroll our scrollback', () => {
    expect(shouldScrollScrollback({ shiftKey: true }, 'any', 'normal')).toBe(true)
  })

  it('does NOT intercept when there is no mouse tracking (xterm scrolls natively)', () => {
    // false here means "return early / let xterm handle it" — correct for native
    // scrollback too; our intercept path exists only for Shift over a tracking app.
    expect(shouldScrollScrollback({ shiftKey: false }, 'none', 'normal')).toBe(false)
    expect(shouldScrollScrollback({ shiftKey: false }, undefined, 'normal')).toBe(false)
  })

  it('does NOT intercept Shift+wheel on the alternate buffer (no scrollback there)', () => {
    expect(shouldScrollScrollback({ shiftKey: true }, 'any', 'alternate')).toBe(false)
  })

  it('does NOT intercept a plain wheel on the alternate buffer (forward to app)', () => {
    expect(shouldScrollScrollback({ shiftKey: false }, 'any', 'alternate')).toBe(false)
  })
})
