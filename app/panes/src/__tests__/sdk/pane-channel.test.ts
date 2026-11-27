import { describe, expect, it, vi } from 'vitest'
import { PaneClient } from '@/sdk/pane-client'
import type { PaneChannel } from '@/sdk/pane-channel'

describe('PaneChannel', () => {
  it('carries a client frame out and a shell frame in', () => {
    const post = vi.fn()
    const channel: PaneChannel = { post, onmessage: null }
    const client = new PaneClient(channel, 'core.chat')
    const titles: string[] = []
    client.onTheme((tokens) => titles.push(Object.keys(tokens).join()))
    client.register()
    channel.onmessage?.(JSON.stringify({ sdk: 1, kind: 'theme', tokens: { 'color-bg': '#fff' } }))
    expect(post).toHaveBeenCalledOnce()
    expect(titles).toEqual(['color-bg'])
  })
})
