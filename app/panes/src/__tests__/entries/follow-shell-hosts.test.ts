import { afterEach, describe, expect, it } from 'vitest'
import { followShellHosts } from '@/entries/follow-shell-hosts'
import { onHostRemoved, useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { PairedHost } from '@/sdk/paired-host'

const RIGREMOTE: PairedHost = { id: 'h1', name: 'rigremote', endpoint: '127.0.0.1:4747' }
const SECOND: PairedHost = { id: 'h2', name: 'second', endpoint: '127.0.0.1:4757' }

function shellThatSends(...lists: PairedHost[][]) {
  return {
    onHosts: (listener: (hosts: PairedHost[]) => void) => (lists.forEach(listener), () => {}),
  }
}

afterEach(() => useHostConfigStore.setState({ hosts: [], remoteEverPaired: false }))

describe('followShellHosts', () => {
  it('makes the hosts the shell paired the hosts this pane reaches', () => {
    followShellHosts(shellThatSends([RIGREMOTE]))
    const { hosts, remoteEverPaired } = useHostConfigStore.getState()
    expect(hosts.map((h) => [h.id, h.name, h.endpoint])).toEqual([
      ['h1', 'rigremote', '127.0.0.1:4747'],
    ])
    expect(remoteEverPaired).toBe(true)
  })

  it('lets go of a host the shell no longer pairs', () => {
    const removed: string[] = []
    const stop = onHostRemoved((id) => removed.push(id))
    followShellHosts(shellThatSends([RIGREMOTE, SECOND], [SECOND]))
    stop()
    expect(useHostConfigStore.getState().hosts.map((h) => h.id)).toEqual(['h2'])
    expect(removed).toEqual(['h1'])
  })
})
