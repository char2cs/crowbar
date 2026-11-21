import { describe, expect, it } from 'vitest'
import type { PaneChannel } from '@/sdk/pane-channel'
import { PaneClient } from '@/sdk/pane-client'
import { followBreakpoints } from '@/sdk/window-breakpoints'

function deliver(channel: PaneChannel, cleared: string[]) {
  channel.onmessage?.(JSON.stringify({ sdk: 1, kind: 'breakpoints', cleared }))
}

describe('followBreakpoints', () => {
  it('names the breakpoints the window clears on the root, and keeps the attribute when none are', () => {
    const channel: PaneChannel = { post: () => {}, onmessage: null }
    const root = document.createElement('div')
    const stop = followBreakpoints(new PaneClient(channel, 'core.editor'), root)
    expect(root.hasAttribute('data-window')).toBe(false)
    deliver(channel, ['sm', 'md'])
    expect(root.getAttribute('data-window')).toBe('sm md')
    deliver(channel, [])
    expect(root.getAttribute('data-window')).toBe('')
    stop()
    expect(root.hasAttribute('data-window')).toBe(false)
  })
})
