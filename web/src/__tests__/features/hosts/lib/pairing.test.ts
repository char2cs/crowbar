import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/crowbar-bridge', () => ({
  pairRemoteHost: vi.fn(),
}))

import { pairRemoteHost } from '@/lib/crowbar-bridge'
import { redeemPairingCode } from '@/features/hosts/lib/pairing'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

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
