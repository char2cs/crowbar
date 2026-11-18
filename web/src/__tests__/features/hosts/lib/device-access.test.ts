import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/api/fetch', () => ({ apiFetch: vi.fn() }))

import { apiFetch } from '@/lib/api/fetch'
import {
  getDeviceAccess,
  listPairedDevices,
  mintPairingOffer,
  revokeDevice,
  setDeviceAccess,
} from '@/features/hosts/lib/device-access'

const call = (n = 0) => vi.mocked(apiFetch).mock.calls[n]

describe('device access API', () => {
  beforeEach(() => vi.mocked(apiFetch).mockReset())

  it('reads the connection state of the chosen host', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce({ enabled: false })
    await getDeviceAccess('home-lab')
    expect(call()[0]).toBe('/v0/access')
    expect(call()[3]).toBe('home-lab')
  })

  it('turns connections on with the address and off without one', async () => {
    vi.mocked(apiFetch).mockResolvedValue({})
    await setDeviceAccess('local', { enabled: true, address: '0.0.0.0:3737' })
    await setDeviceAccess('local', { enabled: false })
    expect(JSON.parse(String(call(0)[1]?.body))).toEqual({ enabled: true, address: '0.0.0.0:3737' })
    expect(call(0)[1]?.method).toBe('PUT')
    expect(JSON.parse(String(call(1)[1]?.body))).toEqual({ enabled: false })
  })

  it('mints the code on the chosen host', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce({ code: '123456' })
    await mintPairingOffer('home-lab')
    expect(call()[0]).toBe('/v0/pair/code')
    expect(call()[1]?.method).toBe('POST')
    expect(call()[3]).toBe('home-lab')
  })

  it('revokes one device by id', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce({ revoked: true })
    await revokeDevice('local', 'c1')
    expect(call()[0]).toBe('/v0/clients/c1')
    expect(call()[1]?.method).toBe('DELETE')
  })

  it('lists only devices that are still paired, newest first', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce([
      { id: 'a', name: 'old', pairedAt: '2026-01-01T00:00:00Z', lastSeen: '' },
      {
        id: 'b',
        name: 'gone',
        pairedAt: '2026-02-01T00:00:00Z',
        lastSeen: '',
        revokedAt: '2026-03-01T00:00:00Z',
      },
      { id: 'c', name: 'new', pairedAt: '2026-04-01T00:00:00Z', lastSeen: '' },
    ])
    const devices = await listPairedDevices('local')
    expect(devices.map((d) => d.id)).toEqual(['c', 'a'])
  })
})
