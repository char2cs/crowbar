import { describe, it, expect } from 'vitest'
import { workspaceSlotStyling } from '@/features/workspace/lib/workspace-slot-style'

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
