import { describe, expect, it, vi } from 'vitest'
import type { PaneChannel } from '@/sdk/pane-channel'
import { PaneClient } from '@/sdk/pane-client'
import { ShellViewedOwner } from '@/features/git/lib/shell-viewed-owner'

function pane() {
  const posted: Record<string, unknown>[] = []
  const channel: PaneChannel = {
    post: (text) => posted.push(JSON.parse(text) as Record<string, unknown>),
    onmessage: null,
  }
  const deliver = (frame: object) => channel.onmessage?.(JSON.stringify({ sdk: 1, ...frame }))
  return { client: new PaneClient(channel, 'core.review'), posted, deliver }
}

describe('ShellViewedOwner', () => {
  it('hands a follower the map the shell sent before it followed, then every later one', () => {
    const { client, deliver } = pane()
    const owner = new ShellViewedOwner(client)
    deliver({ kind: 'review.viewed', workspace: 'w1', viewed: { 'a.ts': 's1' } })
    const heard = vi.fn()
    owner.follow('w1', heard)
    deliver({ kind: 'review.viewed', workspace: 'w2', viewed: { 'z.ts': 'z' } })
    deliver({ kind: 'review.viewed', workspace: 'w1', viewed: {} })
    expect(heard.mock.calls).toEqual([[{ 'a.ts': 's1' }], [{}]])
  })

  it('asks the shell to mark files and keeps no copy of its own', async () => {
    const { client, posted } = pane()
    const owner = new ShellViewedOwner(client)
    const heard = vi.fn()
    owner.follow('w1', heard)
    await owner.set('w1', { files: [{ path: 'a.ts', signature: 's1' }], viewed: true })
    expect(posted).toEqual([
      {
        sdk: 1,
        kind: 'review.setViewed',
        files: [{ path: 'a.ts', signature: 's1' }],
        viewed: true,
      },
    ])
    expect(heard).not.toHaveBeenCalled()
  })
})
