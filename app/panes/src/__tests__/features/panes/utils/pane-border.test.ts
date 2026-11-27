import { describe, expect, it, vi } from 'vitest'
import { buildInnerViewStyle, buildPaneContentStyle } from '@/features/panes/utils/pane-border'
import type { PanePosition } from '@/features/panes/types/pane'

vi.mock('@/utils/platform', () => ({ IS_MAC: true, IS_MOBILE: false, APPLE_KEYBOARD: true }))

const full: PanePosition = { atLeft: true, atTop: true, atRight: true, atBottom: true }
const notAtEdge: PanePosition = { atLeft: false, atTop: false, atRight: false, atBottom: false }

const INACTIVE = '1px solid var(--pane-border)'
const ACTIVE = 'var(--pane-accent-width) solid var(--pane-accent)'

describe('buildPaneContentStyle — collapsed sidebar', () => {
  it('squares the corner the sidebar was covering', () => {
    const open = buildPaneContentStyle(full, 'right', false, true)
    expect(open.borderTopRightRadius).toBe('var(--radius-lg)')

    const collapsed = buildPaneContentStyle(full, 'right', false, false)
    expect(collapsed.borderTopRightRadius).toBe('0')
    expect(collapsed.borderRight).toBe('none')
  })

  it('mirrors for a collapsed left sidebar', () => {
    const collapsed = buildPaneContentStyle(full, 'left', false, false)
    expect(collapsed.borderTopLeftRadius).toBe('0')
    expect(collapsed.borderLeft).toBe('none')
  })
})

describe('buildPaneContentStyle — left sidebar', () => {
  const sidebar = 'left' as const

  it('single pane: only TL rounded; right+bottom border hidden', () => {
    const s = buildPaneContentStyle(full, sidebar, false)
    expect(s.borderTopLeftRadius).toBe('var(--radius-lg)')
    expect(s.borderTopRightRadius).toBe('0')
    expect(s.borderBottomLeftRadius).toBe('0')
    expect(s.borderBottomRightRadius).toBe('0')
    expect(s.borderTop).toBe(INACTIVE)
    expect(s.borderLeft).toBe(INACTIVE)
    expect(s.borderRight).toBe('none')
    expect(s.borderBottom).toBe('none')
  })

  it('H-split left pane: TL+TR rounded; bottom hidden', () => {
    const pos: PanePosition = { atLeft: true, atTop: true, atRight: false, atBottom: true }
    const s = buildPaneContentStyle(pos, sidebar, false)
    expect(s.borderTopLeftRadius).toBe('var(--radius-lg)')
    expect(s.borderTopRightRadius).toBe('var(--radius-lg)')
    expect(s.borderBottomLeftRadius).toBe('0')
    expect(s.borderBottomRightRadius).toBe('0')
    expect(s.borderRight).toBe(INACTIVE)
    expect(s.borderBottom).toBe('none')
  })

  it('H-split right pane: TL rounded only; right+bottom hidden', () => {
    const pos: PanePosition = { atLeft: false, atTop: true, atRight: true, atBottom: true }
    const s = buildPaneContentStyle(pos, sidebar, false)
    expect(s.borderTopLeftRadius).toBe('var(--radius-lg)')
    expect(s.borderTopRightRadius).toBe('0')
    expect(s.borderBottomLeftRadius).toBe('0')
    expect(s.borderRight).toBe('none')
  })

  it('V-split top pane: TL+BL rounded; right hidden', () => {
    const pos: PanePosition = { atLeft: true, atTop: true, atRight: true, atBottom: false }
    const s = buildPaneContentStyle(pos, sidebar, false)
    expect(s.borderTopLeftRadius).toBe('var(--radius-lg)')
    expect(s.borderTopRightRadius).toBe('0')
    expect(s.borderBottomLeftRadius).toBe('var(--radius-lg)')
    expect(s.borderBottomRightRadius).toBe('0')
    expect(s.borderBottom).toBe(INACTIVE)
  })

  it('V-split bottom pane: TL rounded; right+bottom hidden', () => {
    const pos: PanePosition = { atLeft: true, atTop: false, atRight: true, atBottom: true }
    const s = buildPaneContentStyle(pos, sidebar, false)
    expect(s.borderTopLeftRadius).toBe('var(--radius-lg)')
    expect(s.borderTopRightRadius).toBe('0')
    expect(s.borderBottomLeftRadius).toBe('0')
  })

  it('interior pane (not at any edge): all 4 corners rounded', () => {
    const s = buildPaneContentStyle(notAtEdge, sidebar, false)
    expect(s.borderTopLeftRadius).toBe('var(--radius-lg)')
    expect(s.borderTopRightRadius).toBe('var(--radius-lg)')
    expect(s.borderBottomLeftRadius).toBe('var(--radius-lg)')
    expect(s.borderBottomRightRadius).toBe('var(--radius-lg)')
    expect(s.borderLeft).toBe(INACTIVE)
    expect(s.borderRight).toBe(INACTIVE)
    expect(s.borderBottom).toBe(INACTIVE)
  })

  it('active pane: internal edges get the primary border, window edges stay none', () => {
    const s = buildPaneContentStyle(full, sidebar, true)
    expect(s.borderTop).toBe(ACTIVE)
    expect(s.borderLeft).toBe(ACTIVE)
    expect(s.borderRight).toBe('none')
    expect(s.borderBottom).toBe('none')

    const interior = buildPaneContentStyle(notAtEdge, sidebar, true)
    expect(interior.borderTop).toBe(ACTIVE)
    expect(interior.borderLeft).toBe(ACTIVE)
    expect(interior.borderRight).toBe(ACTIVE)
    expect(interior.borderBottom).toBe(ACTIVE)
  })
})

describe('buildPaneContentStyle — right sidebar (mirror)', () => {
  const sidebar = 'right' as const

  it('single pane: only TR rounded; left+bottom border hidden', () => {
    const s = buildPaneContentStyle(full, sidebar, false)
    expect(s.borderTopLeftRadius).toBe('0')
    expect(s.borderTopRightRadius).toBe('var(--radius-lg)')
    expect(s.borderBottomLeftRadius).toBe('0')
    expect(s.borderBottomRightRadius).toBe('0')
    expect(s.borderLeft).toBe('none')
    expect(s.borderRight).toBe(INACTIVE)
    expect(s.borderBottom).toBe('none')
  })

  it('H-split right pane (at sidebar): TL+TR rounded', () => {
    const pos: PanePosition = { atLeft: false, atTop: true, atRight: true, atBottom: true }
    const s = buildPaneContentStyle(pos, sidebar, false)
    expect(s.borderTopLeftRadius).toBe('var(--radius-lg)')
    expect(s.borderTopRightRadius).toBe('var(--radius-lg)')
  })
})

describe('buildPaneContentStyle — gutter (§7.4)', () => {
  const sidebar = 'left' as const

  it('single pane: header-icon inset above (atTop), flush against the (open) sidebar and the window', () => {
    const s = buildPaneContentStyle(full, sidebar, false)
    expect(s.marginLeft).toBe('0')
    expect(s.marginTop).toBe('7px')
    expect(s.marginRight).toBe('0')
    expect(s.marginBottom).toBe('0')
  })

  it('interior pane (not at any edge): 1px on every side — combined with the 6px sash between two neighbours, that lands on the same 8px the window top reserves', () => {
    const s = buildPaneContentStyle(notAtEdge, sidebar, false)
    expect(s.marginLeft).toBe('1px')
    expect(s.marginTop).toBe('1px')
    expect(s.marginRight).toBe('1px')
    expect(s.marginBottom).toBe('1px')
  })

  it('V-split bottom pane (atTop: false): keeps the plain 1px top gutter', () => {
    const pos: PanePosition = { atLeft: true, atTop: false, atRight: true, atBottom: true }
    const s = buildPaneContentStyle(pos, sidebar, false)
    expect(s.marginTop).toBe('1px')
  })

  it('collapsed sidebar: the side it was shielding becomes a window edge and gives up its gutter', () => {
    const s = buildPaneContentStyle(full, sidebar, false, false)
    expect(s.marginLeft).toBe('0')
  })

  it('right sidebar (mirror): flush on both the chrome side and the true left window edge', () => {
    const s = buildPaneContentStyle(full, 'right', false)
    expect(s.marginRight).toBe('0')
    expect(s.marginLeft).toBe('0')
  })
})

describe('buildInnerViewStyle', () => {
  it('facingChatEdge "left": only the left edge is bordered', () => {
    const s = buildInnerViewStyle('left')
    expect(s.borderLeft).toBe('1px solid var(--border)')
    expect(s.borderTop).toBe('none')
    expect(s.borderRight).toBe('none')
    expect(s.borderBottom).toBe('none')
  })

  it('facingChatEdge "right": only the right edge is bordered', () => {
    const s = buildInnerViewStyle('right')
    expect(s.borderRight).toBe('1px solid var(--border)')
    expect(s.borderTop).toBe('none')
    expect(s.borderLeft).toBe('none')
    expect(s.borderBottom).toBe('none')
  })

  it('facingChatEdge "top" (stacked): only the top edge is bordered', () => {
    const s = buildInnerViewStyle('top')
    expect(s.borderTop).toBe('1px solid var(--border)')
    expect(s.borderLeft).toBe('none')
    expect(s.borderRight).toBe('none')
    expect(s.borderBottom).toBe('none')
  })

  it('never rounds any corner, regardless of facingChatEdge', () => {
    for (const facing of ['left', 'right', 'top'] as const) {
      const s = buildInnerViewStyle(facing)
      expect(s.borderTopLeftRadius).toBe('0')
      expect(s.borderTopRightRadius).toBe('0')
      expect(s.borderBottomLeftRadius).toBe('0')
      expect(s.borderBottomRightRadius).toBe('0')
    }
  })
})
