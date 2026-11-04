import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { hostUnavailability } from '@/lib/host-resolve'
import { useHostUnavailable } from '@/lib/host-unavailability'
import { resetEntities, seedProjects } from '@/__tests__/__fixtures__/sidebar-entities'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
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
  seedProjects(projects)
}

function recordScope(wsId: string, projectId: string): void {
  recordWorkspaceScope({ projectId, repoId: `repo-of-${projectId}`, wsId })
}

function setStatus(hostId: string, status: HostStatus): void {
  getOrCreateHostRuntime(hostId).getState().setStatus(status)
}

beforeEach(() => {
  useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  resetEntities()
  __resetWorkspaceScopesForTest()
})

afterEach(() => {
  destroyHostRuntime('host-1')
  destroyHostRuntime('host-2')
})

// The projects scope is the local daemon's: a project it does not hold may be on a paired host.
describe('a host that will not resolve', () => {
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

  it('keeps waiting on a project no row holds while a paired host may hold it', () => {
    configureHosts('host-1')
    setStatus('host-1', 'up')
    loadProjects(project('p-local', 'local'))
    expect(hostUnavailability({ projectId: 'p-elsewhere' })).toBeNull()
  })

  it('says a pane is waiting only on hosts that do not answer', () => {
    configureHosts('host-1')
    setStatus('host-1', 'down')
    loadProjects(project('p-local', 'local'))
    recordScope('ws-1', 'p-elsewhere')
    const { result } = renderHook(() => useHostUnavailable({ wsId: 'ws-1' }))
    expect(result.current).toBe('host-unavailable')
    act(() => setStatus('host-1', 'up'))
    expect(result.current).toBeNull()
  })
})
