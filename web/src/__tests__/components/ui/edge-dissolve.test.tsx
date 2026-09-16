import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { EdgeDissolve } from '@/components/ui/edge-dissolve'

// The progressive-blur "glass" effect stolen from the chat composer's own
// `.dissolve` (composer.css) — content passing behind it should blur and
// fade rather than being clipped by a hard edge. Ported generically (any
// edge, any height) rather than reused in place, since the composer's own
// version is hard-wired to its dock's measured height and scrollbar inset.
describe('EdgeDissolve', () => {
  it('renders 7 layered blur bands, matching the composer’s own recipe', () => {
    render(<EdgeDissolve edge="top" height={100} />)
    const root = screen.getByTestId('edge-dissolve')
    expect(root.children).toHaveLength(7)
  })

  it('is purely decorative — never intercepts pointer events or a11y tree', () => {
    render(<EdgeDissolve edge="top" height={100} />)
    const root = screen.getByTestId('edge-dissolve')
    expect(root).toHaveAttribute('aria-hidden', 'true')
    expect(root).toHaveStyle({ pointerEvents: 'none' })
  })

  it('anchors to the top edge without flipping for edge="bottom" (the composer’s own orientation)', () => {
    render(<EdgeDissolve edge="bottom" height={100} />)
    const root = screen.getByTestId('edge-dissolve')
    expect(root).toHaveStyle({ bottom: '0px' })
    expect(root.style.transform).not.toMatch(/scaleY/)
  })

  it('mirrors the SAME recipe onto the top edge via a vertical flip, rather than re-deriving the math', () => {
    render(<EdgeDissolve edge="top" height={100} />)
    const root = screen.getByTestId('edge-dissolve')
    expect(root).toHaveStyle({ top: '0px' })
    expect(root.style.transform).toMatch(/scaleY\(-1\)/)
  })

  it('sizes itself to the given height', () => {
    render(<EdgeDissolve edge="top" height={120} />)
    expect(screen.getByTestId('edge-dissolve')).toHaveStyle({ height: '120px' })
  })

  it('gives each layer its own blur radius, ramping from subtle to heavy', () => {
    render(<EdgeDissolve edge="top" height={100} />)
    const layers = Array.from(screen.getByTestId('edge-dissolve').children) as HTMLElement[]
    const blurs = layers.map((l) => l.style.backdropFilter)
    expect(blurs).toEqual([
      'blur(1px)',
      'blur(2px)',
      'blur(4px)',
      'blur(8px)',
      'blur(16px)',
      'blur(32px)',
      'blur(64px)',
    ])
  })
})
