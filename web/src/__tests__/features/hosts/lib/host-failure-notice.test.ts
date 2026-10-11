import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { toastError } = vi.hoisted(() => ({ toastError: vi.fn() }))
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: toastError } }))

import {
  actionErrorMessage,
  toastActionFailure,
  toastHostFailure,
} from '@/features/hosts/lib/host-failure-notice'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  destroyHostRuntime,
  getHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import { ApiError, HostNotConfiguredError, proxyMarkerError } from '@/lib/api/errors'
import { HostNotResolvedError } from '@/lib/host-path'
import { failed } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import {
  loadProjects,
  markHostsRead,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

const unreachable = (hostId = 'h1') => proxyMarkerError('host-unreachable', hostId)!
const changed = (hostId = 'h1') => proxyMarkerError('host-identity-changed', hostId)!
const refused = (hostId = 'h1') =>
  new ApiError('client is not paired', 401, undefined, undefined, hostId)
const statusOf = (hostId: string) => getHostRuntime(hostId)?.getState().status
const toasted = () => toastError.mock.calls.map((call) => call.join(' | ')).join('\n')

beforeEach(() => {
  toastError.mockReset()
  resetHostWorld()
  pairRemoteHost('h1', 'Studio Mac')
  getOrCreateHostRuntime('h1').getState().setStatus('up')
})

afterEach(() => {
  destroyHostRuntime('h1')
  resetHostWorld()
})

describe('an action that failed because of its remote host', () => {
  it('names an unreachable host by its name, and records it down', () => {
    expect(toastHostFailure(unreachable())).toBe(true)

    expect(toastError).toHaveBeenCalledExactlyOnceWith(
      'Couldn’t reach Studio Mac',
      'Check that it is online in Settings → Hosts.',
    )
    expect(statusOf('h1')).toBe('down')
  })

  it('warns that a host’s identity changed and that only pairing again on the host fixes it', () => {
    toastHostFailure(changed())

    const [message, description] = toastError.mock.calls[0] as [string, string]
    expect(message).toBe('Studio Mac’s identity changed — it must be paired again')
    expect(description).toMatch(/refused to connect.*run crowbar pair on that host/)
    expect(statusOf('h1')).toBe('needs-repair')
  })

  // A 401 comes from the host refusing this client, or from the proxy holding no credential.
  it('says a host that refuses this computer (401) must be paired again, and records it so', () => {
    toastHostFailure(refused())

    const [message, description] = toastError.mock.calls[0] as [string, string]
    expect(message).toBe('Studio Mac needs to be paired again')
    expect(description).toMatch(/doesn’t accept this computer’s credential, or none is stored/)
    expect(statusOf('h1')).toBe('needs-repair')
  })

  // The proxy's own plain-text 500/502/504 carry no daemon words: only a status.
  it('says an answer that carries only a status is the host’s error, never a bare number', () => {
    expect(toastHostFailure(new ApiError('502 ', 502, undefined, undefined, 'h1'))).toBe(true)
    expect(
      actionErrorMessage(new ApiError('504 Gateway Timeout', 504, undefined, undefined, 'h1'), 'x'),
    ).toBe('Studio Mac returned an error (504)')

    expect(toastError.mock.calls[0]?.[0]).toBe('Studio Mac returned an error (502)')
    expect(statusOf('h1')).toBe('up')
  })

  it('never shows the host’s id or markup, whatever went wrong', () => {
    for (const err of [unreachable(), changed(), refused()]) toastHostFailure(err)

    expect(toasted()).not.toMatch(/\bh1\b|`/)
  })

  it.each([
    [
      'a host removed meanwhile',
      () => (useHostConfigStore.getState().removeHost('h1'), unreachable()),
    ],
    ['a request for a host this app holds no pairing for', () => new HostNotConfiguredError('h9')],
  ])('says %s is no longer paired, never its id', (_name, failure) => {
    toastHostFailure(failure())

    expect(toasted()).toBe(
      'That host is no longer paired with this computer | Pair it again in Settings → Hosts to use it.',
    )
  })
})

describe('an action whose host could not be told yet', () => {
  const unresolved = (projectId = 'p-remote') =>
    new HostNotResolvedError('project', projectId, 'the project list is not loaded')

  it('says the projects are still loading while the list has not been read', () => {
    toastHostFailure(unresolved())

    expect(toasted()).toBe('Still loading your projects | Try again in a moment.')
  })

  it('says the projects could not be loaded when the read failed', () => {
    useProjectDataStore.setState({ data: failed(new Error('daemon down')) })

    toastHostFailure(unresolved())

    expect(toastError).toHaveBeenCalledExactlyOnceWith(
      'Couldn’t load your projects',
      'Check that Crowbar can reach its daemon, then retry.',
    )
  })

  it('sends a project held on a host that is not configured to Hosts', () => {
    loadProjects(projectOn('p-gone', 'h2'))

    toastHostFailure(unresolved('p-gone'))

    expect(toasted()).toBe(
      'This project’s host is not available | Check the host in Settings → Hosts.',
    )
  })

  it('says a project no host has any more is gone from the list', () => {
    loadProjects(projectOn('p-local', 'local'))
    markHostsRead('h1')

    toastHostFailure(unresolved('p-vanished'))

    expect(toastError).toHaveBeenCalledExactlyOnceWith('This project is no longer in your list')
  })

  it('places a workspace through the project its scope names', () => {
    loadProjects(projectOn('p-gone', 'h2'))
    const err = new HostNotResolvedError(
      'workspace',
      'ws-1',
      'its host is not configured',
      'p-gone',
    )

    toastHostFailure(err)

    expect(toastError.mock.calls[0]?.[0]).toBe('This project’s host is not available')
  })

  it('never leaks the internal reason', () => {
    toastHostFailure(new HostNotResolvedError('workspace', 'ws-1', 'no scope is recorded'))

    expect(toasted()).not.toMatch(/scope|ws-1|no host resolved/)
  })
})

// A never-paired install, and every local failure, reads exactly as before hosts existed.
describe('a failure that is not its host’s', () => {
  it.each([
    ['a local daemon’s 401', new ApiError('nope', 401, undefined, undefined, 'local')],
    ['a local bare status', new ApiError('502 ', 502, undefined, undefined, 'local')],
    [
      'a remote daemon’s own error',
      new ApiError('branch is taken', 409, undefined, undefined, 'h1'),
    ],
    ['a plain error', new Error('boom')],
    ['something that is not an error', 'kaboom'],
  ])('%s is left to the caller', (_name, err) => {
    expect(toastHostFailure(err)).toBe(false)

    expect(toastError).not.toHaveBeenCalled()
    expect(statusOf('h1')).toBe('up')
  })

  it('is toasted with its own words, or the fallback for a non-error', () => {
    toastActionFailure(
      new ApiError('branch is taken', 409, undefined, undefined, 'local'),
      'Failed',
    )
    toastActionFailure('kaboom', 'Failed to rename branch')

    expect(toastError.mock.calls).toEqual([['branch is taken'], ['Failed to rename branch']])
  })

  it('is described in its own words inline', () => {
    expect(actionErrorMessage(new Error('locked'), 'Failed')).toBe('locked')
    expect(actionErrorMessage(42, 'Failed to commit changes')).toBe('Failed to commit changes')
  })
})

describe('inline and toasted host failures read the same', () => {
  it('an inline message names the host and records its status', () => {
    expect(actionErrorMessage(changed(), 'Failed')).toBe(
      'Studio Mac’s identity changed — it must be paired again',
    )
    expect(statusOf('h1')).toBe('needs-repair')
  })

  it('a host failure toasted by an action is the host’s notice, not the action’s text', () => {
    toastActionFailure(unreachable(), 'Failed to rename folder')

    expect(toastError).toHaveBeenCalledExactlyOnceWith(
      'Couldn’t reach Studio Mac',
      'Check that it is online in Settings → Hosts.',
    )
  })
})
