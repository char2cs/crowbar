import { afterEach, describe, expect, it, vi } from 'vitest'
import { wsUrl } from '@/lib/ws/url'

afterEach(() => {
  vi.unstubAllEnvs()
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
})
