import { afterEach, describe, expect, it, vi } from 'vitest'

// ws/url.ts imports isTauri from crowbar-bridge (via apiBaseFor's own import
// of it), so vi.hoisted is required — see api.test.ts's identical comment.
const { isTauri } = vi.hoisted(() => ({ isTauri: vi.fn(() => false) }))
vi.mock('@/lib/crowbar-bridge', () => ({ isTauri }))

import { wsUrl } from '@/lib/ws/url'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'

afterEach(() => {
  vi.unstubAllEnvs()
  isTauri.mockReturnValue(false)
  useHostConfigStore.setState({ hosts: [] })
  delete (window as unknown as { __CROWBAR__?: unknown }).__CROWBAR__
})

describe('wsUrl', () => {
  it('resolves a relative path against an http VITE_API_URL as ws://', () => {
    vi.stubEnv('VITE_API_URL', 'http://127.0.0.1:3737')
    expect(wsUrl('/v0/ws/git?wsId=ws-1')).toBe('ws://127.0.0.1:3737/v0/ws/git?wsId=ws-1')
  })

  it('uses wss:// when the base is https', () => {
    vi.stubEnv('VITE_API_URL', 'https://example.test')
    expect(wsUrl('/v0/ws/files?wsId=ws-2')).toBe('wss://example.test/v0/ws/files?wsId=ws-2')
  })

  it('passes through an already-absolute ws url', () => {
    expect(wsUrl('ws://localhost:9999/v0/ws/git')).toBe('ws://localhost:9999/v0/ws/git')
  })

  it('falls back to window.location.origin when no base is set', () => {
    vi.stubEnv('VITE_API_URL', '')
    const result = wsUrl('/v0/ws/workspaces')
    expect(result).toMatch(/^wss?:\/\//)
    expect(result.endsWith('/v0/ws/workspaces')).toBe(true)
  })

  // The default hostId is 'local' — every existing caller omitting it keeps
  // resolving against VITE_API_URL/origin exactly as above, never apiBaseFor.
  it('defaults hostId to "local", ignoring the host store and Tauri entirely', () => {
    vi.stubEnv('VITE_API_URL', 'http://127.0.0.1:3737')
    expect(wsUrl('/v0/ws/git?wsId=ws-1', 'local')).toBe(wsUrl('/v0/ws/git?wsId=ws-1'))
    expect(isTauri).not.toHaveBeenCalled()
  })

  it('throws for a remote hostId outside Tauri', () => {
    expect(() => wsUrl('/v0/ws/git', 'h1')).toThrow(/desktop app/)
  })

  it('resolves a remote hostId to a crowbar://<hostId> base under Tauri', () => {
    isTauri.mockReturnValue(true)
    useHostConfigStore.getState().addHost({
      id: 'h1',
      name: 'Box',
      endpoint: 'box.local:9443',
      pairedFingerprint: 'ff',
      clientId: 'c1',
    })
    expect(wsUrl('/v0/ws/git?wsId=x', 'h1')).toBe('crowbar://h1/v0/ws/git?wsId=x')
  })
})
