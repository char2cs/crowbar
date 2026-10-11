import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiFetch: vi.fn(),
}))

import { ApiError, apiFetch, isHostUnreachableError } from '@/lib/api'
import { proxyMarkerError } from '@/lib/api/errors'
import { browseDirectory } from '@/features/hosts/lib/browse'

beforeEach(() => {
  vi.mocked(apiFetch).mockReset()
})

describe('browseDirectory', () => {
  it("lists the path on the named host's own daemon: the host is apiFetch's 4th argument", async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce([])

    await browseDirectory('h1', '/home/mateo')

    expect(apiFetch).toHaveBeenCalledWith(
      '/v0/system/browse?path=%2Fhome%2Fmateo',
      undefined,
      undefined,
      'h1',
    )
  })

  it('encodes a path the query string would otherwise split', async () => {
    vi.mocked(apiFetch).mockResolvedValueOnce([])

    await browseDirectory('h1', '/srv/a b&c=d#e')

    expect(vi.mocked(apiFetch).mock.calls[0][0]).toBe(
      '/v0/system/browse?path=%2Fsrv%2Fa%20b%26c%3Dd%23e',
    )
  })

  it('answers the entries the daemon listed', async () => {
    const entries = [
      { name: 'projects', path: '/home/mateo/projects', isDir: true },
      { name: '.bashrc', path: '/home/mateo/.bashrc', isDir: false },
    ]
    vi.mocked(apiFetch).mockResolvedValueOnce(entries)

    await expect(browseDirectory('h1', '/home/mateo')).resolves.toEqual(entries)
  })

  it("rejects with the daemon's own refusal, so a caller can say why", async () => {
    const refusal = new ApiError(
      'cannot read directory: open /root: permission denied',
      400,
      undefined,
      undefined,
      'h1',
    )
    vi.mocked(apiFetch).mockRejectedValueOnce(refusal)

    await expect(browseDirectory('h1', '/root')).rejects.toBe(refusal)
  })

  it("rejects with the proxy's host failure unchanged", async () => {
    vi.mocked(apiFetch).mockRejectedValueOnce(proxyMarkerError('host-unreachable', 'h1'))

    const failure = await browseDirectory('h1', '/').catch((err: unknown) => err)

    expect(isHostUnreachableError(failure)).toBe(true)
  })
})
