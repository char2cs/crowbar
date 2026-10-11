import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { followStreamFailures, reportHostFailure } from '@/features/hosts/lib/host-failures'
import {
  destroyHostRuntime,
  getHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import { ApiError, proxyMarkerError } from '@/lib/api/errors'
import {
  reportChannelFailure,
  reportChannelState,
  resetConnectionState,
} from '@/lib/ws/connection-store'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

const STREAM = 'crowbar://h1/v0/projects'
const changed = (hostId = 'h1') => proxyMarkerError('host-identity-changed', hostId)!
const unreachable = (hostId = 'h1') => proxyMarkerError('host-unreachable', hostId)!
const statusOf = (hostId: string) => getHostRuntime(hostId)?.getState().status

let stop: (() => void) | undefined

beforeEach(() => {
  resetConnectionState()
  resetHostWorld()
  pairRemoteHost('h1')
  getOrCreateHostRuntime('h1').getState().setStatus('up')
})

afterEach(() => {
  stop?.()
  stop = undefined
  destroyHostRuntime('h1')
  destroyHostRuntime('h2')
  resetHostWorld()
})

describe('followStreamFailures', () => {
  it('marks a host whose stream met a changed identity as needing repair, at once', () => {
    stop = followStreamFailures()

    reportChannelFailure(STREAM, changed())

    expect(statusOf('h1')).toBe('needs-repair')
  })

  it('marks a host whose stream could not reach it as down', () => {
    stop = followStreamFailures()

    reportChannelFailure(STREAM, unreachable())

    expect(statusOf('h1')).toBe('down')
  })

  it('counts a failure recorded before it started', () => {
    reportChannelFailure(STREAM, changed())

    stop = followStreamFailures()

    expect(statusOf('h1')).toBe('needs-repair')
  })

  // The failure stays recorded until the stream opens: it is one piece of evidence, not a
  // standing claim, so a probe that later reads the host up is not overruled by it.
  it('counts each failure once: a later probe’s answer stands', () => {
    stop = followStreamFailures()
    reportChannelFailure(STREAM, unreachable())
    reportChannelFailure('crowbar://h1/v0/projects/p/repos', unreachable())
    expect(statusOf('h1')).toBe('down')
    getHostRuntime('h1')!.getState().setStatus('up', 3)

    reportChannelState(STREAM, true, 'h1')
    reportChannelState('/v0/projects', true, 'local')

    expect(statusOf('h1')).toBe('up')
  })

  it('stops following when stopped', () => {
    followStreamFailures()()

    reportChannelFailure(STREAM, changed())

    expect(statusOf('h1')).toBe('up')
  })
})

describe('reportHostFailure', () => {
  it('leaves a daemon’s own error, and an upgrade refused without a status, to the prober', () => {
    reportHostFailure(new ApiError('boom', 500, undefined, undefined, 'h1'))
    reportHostFailure(new Error('ws upgrade: HTTP error: 401 Unauthorized'))

    expect(statusOf('h1')).toBe('up')
  })

  // Spec §3: a 401 mid-session on an 'up' host is a dead credential, not a blip.
  it('marks a host that answered 401 as needing repair, at once', () => {
    reportHostFailure(new ApiError('client is not paired', 401, undefined, undefined, 'h1'))

    expect(statusOf('h1')).toBe('needs-repair')
  })

  it('gives a host that is no longer configured no status, and the local daemon none', () => {
    reportHostFailure(changed('h2'))
    reportHostFailure(unreachable('local'))

    expect(getHostRuntime('h2')).toBeUndefined()
    expect(getHostRuntime('local')).toBeUndefined()
  })
})
