import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/crowbar-bridge', () => ({
  pairRemoteHost: vi.fn(),
}))

import { pairRemoteHost } from '@/lib/crowbar-bridge'
import { alreadyPairedReason, redeemPairingCode } from '@/features/hosts/lib/pairing'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import type { Host } from '@/features/hosts/types'

describe('redeemPairingCode', () => {
  beforeEach(() => {
    useHostConfigStore.setState({ hosts: [] })
    vi.mocked(pairRemoteHost).mockReset()
  })

  it('on success: saves the host config with the returned clientId', async () => {
    vi.mocked(pairRemoteHost).mockResolvedValueOnce('client-7')
    await redeemPairingCode('h1', '1.2.3.4:3737', 'SHA256:aaaa', '123456', 'my-device')
    expect(pairRemoteHost).toHaveBeenCalledWith(
      'h1',
      '1.2.3.4:3737',
      'SHA256:aaaa',
      '123456',
      'my-device',
    )
    const saved = useHostConfigStore.getState().hosts.find((h) => h.id === 'h1')
    expect(saved).toMatchObject({ clientId: 'client-7', pairedFingerprint: 'SHA256:aaaa' })
  })

  it('on redeem failure: does NOT save the host config', async () => {
    vi.mocked(pairRemoteHost).mockRejectedValueOnce(new Error('bad code'))
    await expect(
      redeemPairingCode('h1', '1.2.3.4:3737', 'SHA256:aaaa', '000000', 'my-device'),
    ).rejects.toThrow()
    expect(useHostConfigStore.getState().hosts).toEqual([])
  })
})

// Two rows for one daemon make every one of its projects ambiguous, so a second pairing of the
// same host is refused, however its address was typed.
describe('alreadyPairedReason', () => {
  const homelab: Host = {
    id: 'h1',
    name: 'homelab',
    endpoint: 'Homelab.lan:3737',
    pairedFingerprint: 'SHA256:aaaa bbbb',
    clientId: 'c1',
  }
  const SAME_ADDRESS =
    'This address is already paired as homelab. Remove it in Settings → Hosts to pair it again.'

  it.each([
    'Homelab.lan:3737',
    'homelab.lan:3737',
    'homelab.lan',
    ' homelab.lan:3737/ ',
    'https://HOMELAB.lan:3737/',
    'tcp://homelab.lan',
  ])('the address %s is the paired one', (typed) => {
    expect(alreadyPairedReason([homelab], typed, 'SHA256:cccc')).toBe(SAME_ADDRESS)
  })

  it('the paired host’s fingerprint at another address is the same host', () => {
    expect(alreadyPairedReason([homelab], '10.0.0.9:3737', 'SHA256:aaaa bbbb')).toBe(
      'This host is already paired as homelab. Remove it in Settings → Hosts to pair it again.',
    )
  })

  it.each([
    ['another port', 'homelab.lan:4000'],
    ['another host', 'office.lan:3737'],
    ['a longer name', 'homelab.lan.example:3737'],
  ])('%s with another fingerprint is a new host', (_, typed) => {
    expect(alreadyPairedReason([homelab], typed, 'SHA256:cccc')).toBeNull()
  })

  it('an IPv6 address without a port takes the default one', () => {
    const v6 = { ...homelab, endpoint: '[fd00::2]:3737' }
    expect(alreadyPairedReason([v6], '[FD00::2]', 'SHA256:cccc')).toBe(SAME_ADDRESS)
    expect(alreadyPairedReason([v6], '[fd00::2]:4000', 'SHA256:cccc')).toBeNull()
  })
})
