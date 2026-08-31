import { describe, expect, it } from 'vitest'
import { buildPaneContentStyle, isWindowEdge } from '@/features/panes/utils/pane-border'
import type { PanePosition } from '@/features/panes/types/pane'

const full: PanePosition = { atLeft: true, atTop: true, atRight: true, atBottom: true }
const notAtEdge: PanePosition = { atLeft: false, atTop: false, atRight: false, atBottom: false }

const INACTIVE = '2px solid transparent'
const ACTIVE = '2px solid var(--secondary)'

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
    // Internal edges reserve a constant 1px border (transparent when
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

// Spec §7.4: "Percent of the content box, inset by a constant 4px. Left and
// top always take it, so the gutter above the first pane is the same as the
// one beside it and two neighbours sit 8px apart on either axis. Right and
// bottom give it up at the window, where the pane is meant to run into the
// frame." Task 1 measured this live as 0px everywhere — no gutter mechanism
// existed at all. Gated by the exact same we(edge)/isWindowEdge test the
// border/radius already use, so a window edge always reads 0 on every axis.
describe('buildPaneContentStyle — gutter (§7.4)', () => {
  const sidebar = 'left' as const

  it('single pane: 4px beside the (open) sidebar and above, 0 at the window', () => {
    const s = buildPaneContentStyle(full, sidebar, false)
    expect(s.marginLeft).toBe('4px') // chrome side — not the window frame
    expect(s.marginTop).toBe('4px') // top is never a window edge
    expect(s.marginRight).toBe('0') // window edge — gives it up
    expect(s.marginBottom).toBe('0') // window edge — gives it up
  })

  it('interior pane (not at any edge): 4px on every side, so two neighbours sit 8px apart', () => {
    const s = buildPaneContentStyle(notAtEdge, sidebar, false)
    expect(s.marginLeft).toBe('4px')
    expect(s.marginTop).toBe('4px')
    expect(s.marginRight).toBe('4px')
    expect(s.marginBottom).toBe('4px')
  })

  it('collapsed sidebar: the side it was shielding becomes a window edge and gives up its gutter', () => {
    const s = buildPaneContentStyle(full, sidebar, false, false)
    expect(s.marginLeft).toBe('0')
  })

  it('right sidebar (mirror): 4px beside it, 0 on the true left window edge', () => {
    const s = buildPaneContentStyle(full, 'right', false)
    expect(s.marginRight).toBe('4px') // chrome side
    expect(s.marginLeft).toBe('0') // window edge
  })
})
