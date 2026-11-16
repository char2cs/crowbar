import { describe, it, expect, vi, afterEach } from 'vitest'
import { openTransport } from '@/features/hosts/lib/open-transport'

const urls: string[] = []
class FakeNative {
  constructor(url: string) {
    urls.push(url)
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
  urls.length = 0
})

describe('openTransport', () => {
  it('opens a socket for the endpoint', () => {
    vi.stubGlobal('WebSocket', FakeNative)
    openTransport('/v0/ws/x')
    expect(urls[0]).toContain('/v0/ws/x')
  })
})
