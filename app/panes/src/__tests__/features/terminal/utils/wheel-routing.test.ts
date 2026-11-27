import { describe, it, expect } from 'vitest'
import { shouldScrollScrollback } from '@/features/terminal/utils/wheel-routing'

describe('shouldScrollScrollback', () => {
  it('does NOT intercept a plain wheel for a tracking app on the primary buffer (the regression)', () => {
    expect(shouldScrollScrollback({ shiftKey: false }, 'any', 'normal')).toBe(false)
  })

  it('intercepts Shift+wheel on the primary buffer to scroll our scrollback', () => {
    expect(shouldScrollScrollback({ shiftKey: true }, 'any', 'normal')).toBe(true)
  })

  it('does NOT intercept when there is no mouse tracking (xterm scrolls natively)', () => {
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
