import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const invokeMock = vi.fn().mockResolvedValue(undefined)

type TauriWindow = Window & { __TAURI_INTERNALS__?: { invoke: typeof invokeMock } }

describe('remote host bridge commands', () => {
  beforeEach(() => {
    invokeMock.mockClear()
    invokeMock.mockResolvedValue(undefined)
    ;(window as TauriWindow).__TAURI_INTERNALS__ = { invoke: invokeMock }
  })

  afterEach(() => {
    delete (window as TauriWindow).__TAURI_INTERNALS__
    vi.resetModules()
  })

  it('registerRemoteHost invokes register_remote_host with no fingerprint to trust', async () => {
    const { registerRemoteHost } = await import('@/lib/crowbar-bridge')
    await registerRemoteHost('host-1', '1.2.3.4:3737')
    expect(invokeMock).toHaveBeenCalledExactlyOnceWith('register_remote_host', {
      hostId: 'host-1',
      endpoint: '1.2.3.4:3737',
    })
  })

  it('unregisterRemoteHost invokes unregister_remote_host', async () => {
    const { unregisterRemoteHost } = await import('@/lib/crowbar-bridge')
    await unregisterRemoteHost('host-1')
    expect(invokeMock).toHaveBeenCalledExactlyOnceWith('unregister_remote_host', {
      hostId: 'host-1',
    })
  })

  it('forgetHostToken invokes forget_host_token', async () => {
    const { forgetHostToken } = await import('@/lib/crowbar-bridge')
    await forgetHostToken('host-1')
    expect(invokeMock).toHaveBeenCalledExactlyOnceWith('forget_host_token', { hostId: 'host-1' })
  })

  it('probeRemoteFingerprint invokes probe_remote_fingerprint and returns the fingerprint', async () => {
    invokeMock.mockResolvedValueOnce('SHA256:aaaa bbbb cccc dddd eeee ffff')
    const { probeRemoteFingerprint } = await import('@/lib/crowbar-bridge')
    const fp = await probeRemoteFingerprint('1.2.3.4:3737')
    expect(invokeMock).toHaveBeenCalledExactlyOnceWith('probe_remote_fingerprint', {
      endpoint: '1.2.3.4:3737',
    })
    expect(fp).toBe('SHA256:aaaa bbbb cccc dddd eeee ffff')
  })

  it('pairRemoteHost invokes pair_remote_host and returns the client id', async () => {
    invokeMock.mockResolvedValueOnce('client-7')
    const { pairRemoteHost } = await import('@/lib/crowbar-bridge')
    const fingerprint = 'SHA256:aaaa bbbb cccc dddd eeee ffff'
    const clientId = await pairRemoteHost(
      'host-1',
      '1.2.3.4:3737',
      fingerprint,
      '123456',
      'my-device',
    )
    expect(clientId).toBe('client-7')
    expect(invokeMock).toHaveBeenCalledExactlyOnceWith('pair_remote_host', {
      hostId: 'host-1',
      endpoint: '1.2.3.4:3737',
      fingerprint,
      code: '123456',
      name: 'my-device',
    })
  })
})
