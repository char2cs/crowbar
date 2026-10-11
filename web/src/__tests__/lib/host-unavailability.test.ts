import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import { awaitHost, useHostUnavailable } from '@/lib/host-unavailability'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { failed, idle, loading, success } from '@/lib/loadable'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import type { Host } from '@/features/hosts/types'
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

beforeEach(() => {
  useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  useProjectDataStore.setState({ data: idle() })
  __resetWorkspaceScopesForTest()
})

// A waiter cannot wait on a host forever: once the project list has loaded and
// still does not place it, nothing more is coming.
describe('a host that will not resolve', () => {
  it('is unavailable only once a loaded list still does not place it', () => {
    configureHosts('host-1')
    const remote = renderHook(() => useHostUnavailable({ projectId: 'p-remote' }))
    const other = renderHook(() => useHostUnavailable({ projectId: 'p-other' }))
    expect(remote.result.current).toBeNull()
    act(() => useProjectDataStore.setState({ data: loading() }))
    expect(remote.result.current).toBeNull()
    act(() => useProjectDataStore.setState({ data: failed(new Error('boom')) }))
    expect(remote.result.current).toBeNull()
    act(() => loadProjects(project('p-other', 'host-1')))
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

  it('keeps waiting on a workspace whose scope is not recorded yet, however loaded the list', () => {
    configureHosts('host-1')
    loadProjects(project('p-other', 'host-1'))
    const { result } = renderHook(() => useHostUnavailable({ wsId: 'ws-1' }))
    expect(result.current).toBeNull()
    act(() => recordScope('ws-1', 'p-remote'))
    expect(result.current).toBe('project-missing')
    act(() => loadProjects(project('p-other', 'host-1'), project('p-remote', 'host-1')))
    expect(result.current).toBeNull()
  })

  it('awaitHost answers true when the project lands, false when the list lands without it', async () => {
    configureHosts('host-1')
    const lands = awaitHost({ projectId: 'p-remote' })
    const absent = awaitHost({ projectId: 'p-gone' })
    loadProjects(project('p-remote', 'host-1'))
    await expect(lands).resolves.toBe(true)
    await expect(absent).resolves.toBe(false)
  })

  it('awaitHost answers at once for a host that already resolves', async () => {
    configureHosts('host-1')
    loadProjects(project('p-remote', 'host-1'))
    await expect(awaitHost({ projectId: 'p-remote' })).resolves.toBe(true)
  })
})
