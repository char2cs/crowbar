import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { awaitHost, hostUnavailability, useHostUnavailable } from '@/lib/host-unavailability'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import { failed, idle, loading, success } from '@/lib/loadable'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import type { Host, HostStatus } from '@/features/hosts/types'
import type { Project } from '@/lib/types'

const host = (id: string): Host => ({
  id,
  name: id,
  endpoint: `${id}.example:3737`,
  pairedFingerprint: 'SHA256:aaaa',
  clientId: `client-${id}`,
})

const project = (id: string, hostId: string): Project => ({
  id,
  name: id,
  path: `/work/${id}`,
  lastActivity: new Date(0),
  hostId,
})

function configureHosts(...ids: string[]): void {
  useHostConfigStore.setState({ hosts: ids.map(host) })
}

function loadProjects(...projects: Project[]): void {
  useProjectDataStore.setState({ data: success(projects) })
}

function recordScope(wsId: string, projectId: string): void {
  recordWorkspaceScope({ projectId, repoId: `repo-of-${projectId}`, wsId })
}

/** The paired hosts whose own answer the list holds since they came up. */
function markRead(...hostIds: string[]): void {
  useProjectDataStore.setState({ readHosts: new Set(hostIds) })
}

function setStatus(hostId: string, status: HostStatus): void {
  getOrCreateHostRuntime(hostId).getState().setStatus(status)
}

beforeEach(() => {
  useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  useProjectDataStore.setState({ data: idle(), readHosts: new Set() })
  __resetWorkspaceScopesForTest()
})

afterEach(() => {
  destroyHostRuntime('host-1')
  destroyHostRuntime('host-2')
})

// A waiter cannot wait on a host forever: once the project list has loaded and still does
// not place it, and every paired host has said what it has, nothing more is coming.
describe('a host that will not resolve', () => {
  it('is unavailable only once a loaded list still does not place it and every host was read', () => {
    configureHosts('host-1')
    const remote = renderHook(() => useHostUnavailable({ projectId: 'p-remote' }))
    const other = renderHook(() => useHostUnavailable({ projectId: 'p-other' }))
    expect(remote.result.current).toBeNull()
    act(() => useProjectDataStore.setState({ data: loading() }))
    expect(remote.result.current).toBeNull()
    act(() => useProjectDataStore.setState({ data: failed(new Error('boom')) }))
    expect(remote.result.current).toBeNull()
    act(() => loadProjects(project('p-other', 'host-1')))
    expect(remote.result.current).toBeNull()
    act(() => markRead('host-1'))
    expect(remote.result.current).toBe('project-missing')
    expect(other.result.current).toBeNull()
  })

  it('tells a project gone from the list apart from one whose host is not configured', () => {
    configureHosts('host-1')
    loadProjects(project('p-remote', 'host-1'))
    const { result } = renderHook(() => useHostUnavailable({ projectId: 'p-remote' }))
    expect(result.current).toBeNull()
    act(() => configureHosts())
    expect(result.current).toBe('host-unavailable')
  })

  it('is never unavailable for an install that cannot hold a remote project', () => {
    loadProjects(project('p-local', 'local'))
    const { result } = renderHook(() => useHostUnavailable({ projectId: 'p-unknown' }))
    expect(result.current).toBeNull()
  })

  it('has nothing to wait on without a key, however final the list', () => {
    loadProjects(project('p-local', 'local'))
    const { result } = renderHook(() => useHostUnavailable(null))
    expect(result.current).toBeNull()
  })

  it('keeps waiting on a workspace whose scope is not recorded yet, however loaded the list', () => {
    configureHosts('host-1')
    loadProjects(project('p-other', 'host-1'))
    markRead('host-1')
    const { result } = renderHook(() => useHostUnavailable({ wsId: 'ws-1' }))
    expect(result.current).toBeNull()
    act(() => recordScope('ws-1', 'p-remote'))
    expect(result.current).toBe('project-missing')
    act(() => loadProjects(project('p-other', 'host-1'), project('p-remote', 'host-1')))
    expect(result.current).toBeNull()
  })

  // Boot: the first local answer lands before the prober has read a paired host, and a
  // project created there since the cache was written is in no row yet.
  it('keeps waiting on a project no row holds while a paired host was not read since it came up', () => {
    configureHosts('host-1', 'host-2')
    loadProjects(project('p-local', 'local'))
    const { result } = renderHook(() => useHostUnavailable({ projectId: 'p-remote' }))
    for (const status of ['unknown', 'probing', 'up'] as const) {
      act(() => setStatus('host-1', status))
      expect(result.current).toBeNull()
    }
    act(() => markRead('host-1'))
    expect(result.current).toBeNull()
    act(() => markRead('host-1', 'host-2'))
    expect(result.current).toBe('project-missing')
  })

  it('says the host is not available while every host still to answer is down or needs repair, and still waits', async () => {
    configureHosts('host-1', 'host-2')
    loadProjects(project('p-local', 'local'))
    markRead('host-2')
    const { result } = renderHook(() => useHostUnavailable({ projectId: 'p-remote' }))
    const placed = awaitHost({ projectId: 'p-remote' })
    expect(result.current).toBeNull()
    act(() => setStatus('host-1', 'down'))
    expect(result.current).toBe('host-unavailable')
    act(() => setStatus('host-1', 'needs-repair'))
    expect(result.current).toBe('host-unavailable')
    expect(hostUnavailability({ projectId: 'p-remote' })).toBeNull()

    act(() => {
      setStatus('host-1', 'up')
      loadProjects(project('p-local', 'local'), project('p-remote', 'host-1'))
    })
    expect(result.current).toBeNull()
    await expect(placed).resolves.toBe(true)
  })

  it('awaitHost answers true when the project lands, false once every host was read without it', async () => {
    configureHosts('host-1')
    const lands = awaitHost({ projectId: 'p-remote' })
    const absent = awaitHost({ projectId: 'p-gone' })
    const gaveUp = vi.fn()
    void absent.then(gaveUp)
    loadProjects(project('p-remote', 'host-1'))
    await expect(lands).resolves.toBe(true)
    expect(gaveUp).not.toHaveBeenCalled()
    markRead('host-1')
    await expect(absent).resolves.toBe(false)
  })

  it('awaitHost answers at once for a host that already resolves', async () => {
    configureHosts('host-1')
    loadProjects(project('p-remote', 'host-1'))
    await expect(awaitHost({ projectId: 'p-remote' })).resolves.toBe(true)
  })
})
