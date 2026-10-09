import { describe, expect, it, vi } from 'vitest'

// The guards live in api.ts, which imports isTauri from the bridge.
vi.mock('@/lib/crowbar-bridge', () => ({ isTauri: () => false }))

import { ApiError, isHostIdentityChangedError, isHostUnreachableError } from '@/lib/api'
import { streamFailureOf } from '@/lib/api/errors'

// What ws_open rejects with for a remote host (desktop/src-tauri/src/ws_bridge/remote.rs):
// `<x-crowbar-proxy value>: <reason>`. Tauri hands a string error to JS as a string.
describe('streamFailureOf', () => {
  it('names an identity change the way an HTTP request failing that way is named', () => {
    const err = streamFailureOf(
      'host-identity-changed: the certificate presented by box:3737 no longer matches',
      'h1',
    )

    expect(err).toBeInstanceOf(ApiError)
    expect(isHostIdentityChangedError(err)).toBe(true)
    expect(err).toMatchObject({ hostId: 'h1', status: 502 })
  })

  it('names an unreachable host, and tells it apart from an identity change', () => {
    const err = streamFailureOf('host-unreachable: cannot reach box:3737: connection refused', 'h1')

    expect(isHostUnreachableError(err)).toBe(true)
    expect(isHostIdentityChangedError(err)).toBe(false)
    expect(err).toMatchObject({ hostId: 'h1', status: 503 })
  })

  it('reads the marker off an Error as well as a bare string', () => {
    const err = streamFailureOf(new Error('host-unreachable: dial timed out'), 'h2')

    expect(isHostUnreachableError(err)).toBe(true)
    expect(err?.hostId).toBe('h2')
  })

  it.each([
    'ws upgrade: HTTP error: 401 Unauthorized',
    'unknown remote host h1',
    'no stored credential for this host — pair it again',
    'connect daemon socket: No such file or directory',
    'daemon-unavailable: the local daemon is starting',
    ' host-unreachable: leading space',
    'host-unreachable',
    'host-unreachable-ish: x',
    '',
  ])('does not make a failure out of %j', (message) => {
    expect(streamFailureOf(message, 'h1')).toBeNull()
  })

  it('ignores errors that are not text', () => {
    expect(streamFailureOf(undefined, 'h1')).toBeNull()
    expect(streamFailureOf({ type: 'error' }, 'h1')).toBeNull()
    expect(streamFailureOf(new Event('error'), 'h1')).toBeNull()
  })
})
