import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/crowbar-bridge', () => ({
  pairRemoteHost: vi.fn(),
}))

import { pairRemoteHost } from '@/lib/crowbar-bridge'
import {
  alreadyPairedReason,
  pairingFailureText,
  redeemPairingCode,
} from '@/features/hosts/lib/pairing'
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
    expect(saved).toMatchObject({
      endpoint: '1.2.3.4:3737',
      clientId: 'client-7',
      pairedFingerprint: 'SHA256:aaaa',
    })
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
// same host is refused. Addresses arrive canonical (normalizeEndpoint), as every saved one is.
describe('alreadyPairedReason', () => {
  const homelab: Host = {
    id: 'h1',
    name: 'homelab',
    endpoint: 'homelab.lan:3737',
    pairedFingerprint: 'SHA256:aaaa bbbb',
    clientId: 'c1',
  }

  it('the paired address is the paired host', () => {
    expect(alreadyPairedReason([homelab], 'homelab.lan:3737', 'SHA256:cccc')).toBe(
      'This address is already paired as homelab. Remove it in Settings → Hosts to pair it again.',
    )
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
  ])('%s with another fingerprint is a new host', (_, endpoint) => {
    expect(alreadyPairedReason([homelab], endpoint, 'SHA256:cccc')).toBeNull()
  })
})

// What the desktop shell rejects with, said as what to do next: never transport or OS words.
describe('pairingFailureText', () => {
  const at = '1.2.3.4:3737'
  const WRONG_CODE =
    'That code didn’t work. It may be mistyped or expired — the host doesn’t say which. Check it, or run crowbar pair on the host again for a new one.'
  const TOO_MANY =
    'Too many attempts — wait a minute, or run crowbar pair on the host for a new code.'
  const UNREACHABLE = `Can’t reach ${at}. Check the address and port, and that the host is running.`

  it.each([
    ['the host refused pairing (401 Unauthorized): pairing code is not valid', WRONG_CODE],
    ['the host refused pairing (429 Too Many Requests): no reason given', TOO_MANY],
    [
      'the host refused pairing (429 Too Many Requests): too many requests, try again shortly',
      TOO_MANY,
    ],
    ['cannot reach 1.2.3.4:3737: Connection refused (os error 61)', UNREACHABLE],
    [
      'cannot resolve homelab.example.com: failed to lookup address information: nodename nor servname provided, or not known',
      'Couldn’t find homelab.example.com. Check the spelling, or use its IP address.',
    ],
    ['1.2.3.4:3737 did not answer within 5s', UNREACHABLE],
    ['1.2.3.4:3737 did not finish pairing within 30s', UNREACHABLE],
    ['pairing with 1.2.3.4:3737 failed: connection reset', UNREACHABLE],
    [
      'TLS handshake with 1.2.3.4:3737 failed: received corrupt message of type InvalidContentType',
      `${at} answered, but not as a Crowbar host. Check the address and port.`,
    ],
    [
      '1.2.3.4:3737 presented no certificate',
      `${at} answered, but not as a Crowbar host. Check the address and port.`,
    ],
    [
      '1.2.3.4:3737 does not hold the identity pinned as SHA256:aaaa — this host’s identity changed (invalid peer certificate)',
      `The host at ${at} no longer presents the fingerprint you confirmed, so nothing was sent to it. Go back and check the address.`,
    ],
    [
      'the host refused pairing (400 Bad Request): name is required',
      `${at} refused pairing: name is required.`,
    ],
    [
      'keychain is locked',
      `Couldn’t pair with ${at}. Try again, or run crowbar pair on the host for a new code.`,
    ],
  ])('%s', (raw, text) => {
    expect(pairingFailureText(raw, at)).toBe(text)
    expect(pairingFailureText(new Error(raw), at)).toBe(text)
  })
})
