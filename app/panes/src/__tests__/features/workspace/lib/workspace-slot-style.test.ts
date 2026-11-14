import { describe, it, expect } from 'vitest'
import { workspaceSlotStyling } from '@/features/workspace/lib/workspace-slot-style'

// The slot styling IS the retention hide strategy: the active workspace paints
// (display:contents), retained ones are dropped from the render tree entirely
// with display:none + inert so their Monaco/xterm collapse to zero size and go
// dormant. (content-visibility:hidden was tried here and reverted — it kept
// hidden widgets rendering at full size, pinning the CPU; see the function doc.)
describe('workspaceSlotStyling', () => {
  it('active → display:contents, not inert', () => {
    const { style, inert } = workspaceSlotStyling(true)
    expect(style.display).toBe('contents')
    expect(inert).toBe(false)
  })

  it('inactive → display:none + inert (dropped from the render tree, widgets go dormant)', () => {
    const { style, inert } = workspaceSlotStyling(false)
    expect(style.display).toBe('none')
    expect(inert).toBe(true)
  })

  it('inactive → no content-visibility/positioning overrides (display:none is the whole mechanism)', () => {
    const { style } = workspaceSlotStyling(false)
    expect(style.contentVisibility).toBeUndefined()
    expect(style.position).toBeUndefined()
    expect(style.containIntrinsicSize).toBeUndefined()
  })
})
