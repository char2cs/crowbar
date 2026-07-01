import { describe, it, expect, vi, beforeEach } from 'vitest'
import { terminalListLive } from '@/lib/crowbar-bridge'

// terminalListLive must handle BOTH daemon response shapes: git workspaces return
// TerminalSessionDTO[] (objects with id/status), the home workspace returns a plain
// string[] of session ids. The original `.map(s => s.id)` returned [null] for home,
// silently breaking home-workspace reconnect after a daemon restart.
beforeEach(() => {
  vi.restoreAllMocks()
})

function stubFetch(data: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ success: true, data }), { status: 200 })),
  )
}

describe('terminalListLive', () => {
  it('extracts ids from a TerminalSessionDTO[] (git workspace) and drops ended', async () => {
    stubFetch([
      { id: 'a', status: 'active' },
      { id: 'b', status: 'ended' },
      { id: 'c', status: 'detached' },
    ])
    expect(await terminalListLive('/base')).toEqual(['a', 'c'])
  })

  it('returns the plain strings from a string[] (home workspace)', async () => {
    stubFetch(['x-id', 'y-id', 'z-id'])
    expect(await terminalListLive('/base')).toEqual(['x-id', 'y-id', 'z-id'])
  })

  it('returns [] for an empty list', async () => {
    stubFetch([])
    expect(await terminalListLive('/base')).toEqual([])
  })

  it('skips malformed entries (no id)', async () => {
    stubFetch([{ status: 'active' }, 'good-id', null])
    expect(await terminalListLive('/base')).toEqual(['good-id'])
  })
})
