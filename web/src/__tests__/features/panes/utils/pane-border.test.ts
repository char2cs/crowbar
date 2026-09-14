import { describe, expect, it } from 'vitest'
import {
  buildInnerViewStyle,
  buildPaneContentStyle,
  isWindowEdge,
} from '@/features/panes/utils/pane-border'
import type { PanePosition } from '@/features/panes/types/pane'

const full: PanePosition = { atLeft: true, atTop: true, atRight: true, atBottom: true }
const notAtEdge: PanePosition = { atLeft: false, atTop: false, atRight: false, atBottom: false }

const INACTIVE = '1px solid var(--border)'
const ACTIVE = '1px solid var(--secondary)'

describe('isWindowEdge', () => {
  it('top is never a window edge', () => {
    expect(isWindowEdge('top', full, 'left')).toBe(false)
    expect(isWindowEdge('top', full, 'right')).toBe(false)
    expect(isWindowEdge('top', notAtEdge, 'left')).toBe(false)
  })

  it('top is never a window edge, regardless of sidebar state', () => {
    expect(isWindowEdge('top', full, 'left', true)).toBe(false)
    expect(isWindowEdge('top', full, 'left', false)).toBe(false)
  })

  it('bottom is always a window edge when atBottom', () => {
    expect(isWindowEdge('bottom', full, 'left')).toBe(true)
    expect(isWindowEdge('bottom', full, 'right')).toBe(true)
    expect(isWindowEdge('bottom', notAtEdge, 'left')).toBe(false)
  })

  it('left is window edge when atLeft and sidebar is NOT on left', () => {
    expect(isWindowEdge('left', { ...full, atLeft: true }, 'right')).toBe(true)
    expect(isWindowEdge('left', { ...full, atLeft: true }, 'left')).toBe(false)
    expect(isWindowEdge('left', { ...full, atLeft: false }, 'right')).toBe(false)
  })

  it('right is window edge when atRight and sidebar is NOT on right', () => {
    expect(isWindowEdge('right', { ...full, atRight: true }, 'left')).toBe(true)
    expect(isWindowEdge('right', { ...full, atRight: true }, 'right')).toBe(false)
    expect(isWindowEdge('right', { ...full, atRight: false }, 'left')).toBe(false)
  })

  // A collapsed sidebar shields nothing: the pane is flush against the frame, so
  // that edge has to square off. Getting this wrong left a rounded corner and a
  // border on the window's own rounded, vibrant edge, and compositing it cost
  // ~98ms per frame in WKWebView — 8ms frames became 106ms for as long as the
  // sidebar stayed hidden.
  it('treats the sidebar side as a window edge once the sidebar is collapsed', () => {
    expect(isWindowEdge('left', { ...full, atLeft: true }, 'left', false)).toBe(true)
    expect(isWindowEdge('right', { ...full, atRight: true }, 'right', false)).toBe(true)
  })

  it('still shields the pane while the sidebar is open', () => {
    expect(isWindowEdge('left', { ...full, atLeft: true }, 'left', true)).toBe(false)
    expect(isWindowEdge('right', { ...full, atRight: true }, 'right', true)).toBe(false)
  })
})

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
    // Internal edges reserve a constant 1px border (neutral --border when
    // inactive) so activating a pane never shifts layout; window edges none.
    expect(s.borderTop).toBe(INACTIVE)
    expect(s.borderLeft).toBe(INACTIVE) // chrome side
    expect(s.borderRight).toBe('none') // window edge
    expect(s.borderBottom).toBe('none') // window edge
  })

  it('H-split left pane: TL+TR rounded; bottom hidden', () => {
    const pos: PanePosition = { atLeft: true, atTop: true, atRight: false, atBottom: true }
    const s = buildPaneContentStyle(pos, sidebar, false)
    expect(s.borderTopLeftRadius).toBe('var(--radius-lg)')
    expect(s.borderTopRightRadius).toBe('var(--radius-lg)')
    expect(s.borderBottomLeftRadius).toBe('0')
    expect(s.borderBottomRightRadius).toBe('0')
    expect(s.borderRight).toBe(INACTIVE) // faces sibling pane
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
    expect(s.borderBottom).toBe(INACTIVE) // faces sibling pane
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
    expect(s.borderLeft).toBe(ACTIVE) // chrome side
    expect(s.borderRight).toBe('none') // window edge
    expect(s.borderBottom).toBe('none') // window edge

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
    expect(s.borderLeft).toBe('none') // window edge
    expect(s.borderRight).toBe(INACTIVE) // chrome side
    expect(s.borderBottom).toBe('none') // window edge
  })

  it('H-split right pane (at sidebar): TL+TR rounded', () => {
    const pos: PanePosition = { atLeft: false, atTop: true, atRight: true, atBottom: true }
    const s = buildPaneContentStyle(pos, sidebar, false)
    expect(s.borderTopLeftRadius).toBe('var(--radius-lg)')
    expect(s.borderTopRightRadius).toBe('var(--radius-lg)')
  })
})

// Spec §7.4: "Percent of the content box, inset by a constant gutter. Left
// and top always take it, so the gutter above the first pane is the same as
// the one beside it. Right and bottom give it up at the window, where the
// pane is meant to run into the frame."
//
// Left/right depart from that "same test as border/radius" rule: the gutter
// drops whenever `position.atLeft`/`atRight` is true — a real window edge OR
// a side the sidebar is shielding — so the pane sits flush against the
// sidebar with no gap, even though its rounded corner and border stay put
// there (border/radius are still gated on the stricter we(edge)/isWindowEdge
// test). Only an interior split neighbour, touching neither boundary, keeps
// the gutter.
//
// The interior gutter is 1px, not 4px: two split neighbours are ALSO
// separated by their own resize sash (`PaneSash`, 6px wide) sitting between
// them as its own flex sibling — reported live as "huge" next to the crisp
// 8px inset the real window top gets, because 4px margin + 6px sash + 4px
// margin came to 14px, nearly double. 1px margin + 6px sash + 1px margin
// lands on the SAME 8px the true window-top edge already reserves (see
// below) — one gutter value, used consistently everywhere a pane isn't
// flush against the literal window frame.
//
// The pane actually touching the window's top (atTop) reserves the header
// row's own icon inset instead — 8px on macOS (44px row, 28px icon-sm
// buttons, centered) — so the rounded tile's top edge lines up with where
// those icons start. That value is untouched; it's the interior gutter that
// was brought in line with it, not the other way around.
describe('buildPaneContentStyle — gutter (§7.4)', () => {
  const sidebar = 'left' as const

  it('single pane: header-icon inset above (atTop), flush against the (open) sidebar and the window', () => {
    const s = buildPaneContentStyle(full, sidebar, false)
    expect(s.marginLeft).toBe('0') // chrome side — sits flush against the sidebar, no gap
    expect(s.marginTop).toBe('8px') // atTop — matches the header row's icon inset (macOS: (44-28)/2)
    expect(s.marginRight).toBe('0') // window edge — gives it up
    expect(s.marginBottom).toBe('0') // window edge — gives it up
  })

  it('interior pane (not at any edge): 1px on every side — combined with the 6px sash between two neighbours, that lands on the same 8px the window top reserves', () => {
    const s = buildPaneContentStyle(notAtEdge, sidebar, false)
    expect(s.marginLeft).toBe('1px')
    expect(s.marginTop).toBe('1px') // not atTop — a split neighbour, not the window's top
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
    expect(s.marginRight).toBe('0') // chrome side — sits flush against the sidebar
    expect(s.marginLeft).toBe('0') // window edge
  })
})

// The IDE shell's own border/rounding no longer depends on the shared box
// at all: only the facing edge (the one genuine internal seam) ever draws a
// border, and no corner is ever rounded — reported live: "the IDE shell
// shouldn't have rounded corners anymore."
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
