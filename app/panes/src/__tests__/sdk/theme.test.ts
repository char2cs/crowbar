import { describe, expect, it } from 'vitest'
import type { PaneChannel } from '@/sdk/pane-channel'
import { PaneClient } from '@/sdk/pane-client'
import { followTheme } from '@/sdk/theme'

function deliverTheme(channel: PaneChannel, tokens: Record<string, string>) {
  channel.onmessage?.(JSON.stringify({ sdk: 1, kind: 'theme', tokens }))
}

describe('followTheme', () => {
  it('publishes the tokens, replaces them on change and drops ones no longer sent', () => {
    const channel: PaneChannel = { post: () => {}, onmessage: null }
    const root = document.createElement('div')
    const stop = followTheme(new PaneClient(channel, 'core.chat'), root)
    deliverTheme(channel, { 'color-bg': '#111', 'radius.sm': '6px' })
    expect(root.style.getPropertyValue('--color-bg')).toBe('#111')
    deliverTheme(channel, { 'color-bg': '#222' })
    expect(root.style.getPropertyValue('--color-bg')).toBe('#222')
    expect(root.style.getPropertyValue('--radius.sm')).toBe('')
    stop()
    expect(root.style.getPropertyValue('--color-bg')).toBe('')
  })
})
