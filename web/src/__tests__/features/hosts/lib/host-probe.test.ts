import { beforeEach, describe, expect, it, vi } from 'vitest'
import { probeHost } from '@/features/hosts/lib/host-probe'
import {
  getOrCreateHostRuntime,
  destroyHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'

vi.mock('@/lib/api', () => ({ apiFetch: vi.fn(), isHostIdentityChangedError: vi.fn(() => false) }))
import { apiFetch, isHostIdentityChangedError } from '@/lib/api'

describe('probeHost', () => {
  beforeEach(() => {
    destroyHostRuntime('h1')
    vi.mocked(apiFetch).mockReset()
    vi.mocked(isHostIdentityChangedError).mockReset().mockReturnValue(false)
  })

  it('sets status "up" when the health call succeeds', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce(undefined)
    getOrCreateHostRuntime('h1')
    await probeHost('h1')
    expect(getOrCreateHostRuntime('h1').getState().status).toBe('up')
  })

  it('sets status "down" when the health call rejects with an ordinary error', async () => {
    vi.mocked(apiFetch).mockRejectedValueOnce(new Error('unreachable'))
    getOrCreateHostRuntime('h1')
    await probeHost('h1')
    expect(getOrCreateHostRuntime('h1').getState().status).toBe('down')
  })

  it('sets status "needs-repair" when the health call rejects with a host-identity-changed error', async () => {
    const err = new Error('identity changed')
    vi.mocked(apiFetch).mockRejectedValueOnce(err)
    vi.mocked(isHostIdentityChangedError).mockReturnValue(true)
    getOrCreateHostRuntime('h1')
    await probeHost('h1')
    expect(getOrCreateHostRuntime('h1').getState().status).toBe('needs-repair')
  })

  it('sets status "probing" while the call is in flight', async () => {
    let resolve!: () => void
    vi.mocked(apiFetch).mockReturnValueOnce(
      new Promise((r) => {
        resolve = () => r(undefined)
      }),
    )
    getOrCreateHostRuntime('h1')
    const promise = probeHost('h1')
    expect(getOrCreateHostRuntime('h1').getState().status).toBe('probing')
    resolve()
    await promise
  })
})
