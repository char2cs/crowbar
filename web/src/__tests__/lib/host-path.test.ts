import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  HostNotResolvedError,
  hostIdForProject,
  hostIdForWorkspace,
  qualify,
  splitHostPath,
  useHostResolved,
} from '@/lib/host-path'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { failed, idle, loading, success } from '@/lib/loadable'
import {
  __resetWorkspaceScopesForTest,
  bindActiveWorkspaceId,
  recordWorkspaceScope,
} from '@/lib/workspace-scope'
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
  useHostConfigStore.setState({ hosts: [] })
  useProjectDataStore.setState({ data: idle() })
  __resetWorkspaceScopesForTest()
})

afterEach(() => {
  bindActiveWorkspaceId(() => null)
  vi.restoreAllMocks()
})

describe('qualify', () => {
  it('leaves a local path byte-identical', () => {
    expect(qualify('local', '/v0/projects/p1/repos?x=1')).toBe('/v0/projects/p1/repos?x=1')
  })

  it('prefixes a remote host, keeping the query', () => {
    expect(qualify('host-1', '/v0/ws/git?wsId=w1')).toBe('crowbar://host-1/v0/ws/git?wsId=w1')
    expect(qualify('0f3c9a2e-7b1d-4e6a-9c55-2d8f1b0a6e43', '/v0/projects')).toBe(
      'crowbar://0f3c9a2e-7b1d-4e6a-9c55-2d8f1b0a6e43/v0/projects',
    )
  })

  it.each(['', 'localhost', 'Host-1', 'host@1', 'host:1', 'host/1', 'host.1', 'host 1'])(
    'rejects the host id %j that the desktop proxy refuses',
    (hostId) => {
      expect(() => qualify(hostId, '/v0/projects')).toThrow(/invalid host id/)
    },
  )

  it('rejects a remote path that does not start with a slash', () => {
    expect(() => qualify('host-1', 'v0/projects')).toThrow(/must start with/)
  })
})

describe('splitHostPath', () => {
  it('treats a bare path as local and returns it untouched', () => {
    expect(splitHostPath('/v0/projects/p1')).toEqual({ hostId: 'local', path: '/v0/projects/p1' })
    expect(splitHostPath('http://127.0.0.1:3737/v0/x')).toEqual({
      hostId: 'local',
      path: 'http://127.0.0.1:3737/v0/x',
    })
  })

  it('splits a qualified path into its host and the path with its query', () => {
    expect(splitHostPath('crowbar://host-1/v0/ws/git?wsId=w1')).toEqual({
      hostId: 'host-1',
      path: '/v0/ws/git?wsId=w1',
    })
  })

  it('is the inverse of qualify', () => {
    for (const [hostId, path] of [
      ['local', '/v0/projects'],
      ['host-1', '/v0/projects/p1/repos/r1/workspaces/w1'],
      ['abc123', '/v0/ws/terminal?session=s%201'],
    ]) {
      expect(splitHostPath(qualify(hostId, path))).toEqual({ hostId, path })
    }
  })

  it('reads the desktop local base crowbar://localhost as local, never as a remote host', () => {
    expect(splitHostPath('crowbar://localhost/v0/projects')).toEqual({
      hostId: 'local',
      path: '/v0/projects',
    })
    expect(splitHostPath('crowbar://localhost')).toEqual({ hostId: 'local', path: '' })
  })

  it.each(['crowbar://Host_1/v0/x', 'crowbar://local/v0/x', 'crowbar:///v0/x', 'crowbar://host-1'])(
    'throws on the malformed qualified path %j instead of falling back to local',
    (input) => {
      expect(() => splitHostPath(input)).toThrow()
    },
  )
})

describe('hostIdForProject', () => {
  it('is local with no hosts configured, without reading the project list', () => {
    const read = vi.spyOn(useProjectDataStore, 'getState')
    expect(hostIdForProject('anything')).toBe('local')
    expect(read).not.toHaveBeenCalled()
  })

  it('returns the host recorded on a known remote project', () => {
    configureHosts('host-1', 'host-2')
    loadProjects(project('p-local', 'local'), project('p-remote', 'host-2'))
    expect(hostIdForProject('p-remote')).toBe('host-2')
  })

  it('returns local for a known local project', () => {
    configureHosts('host-1')
    loadProjects(project('p-local', 'local'), project('p-remote', 'host-1'))
    expect(hostIdForProject('p-local')).toBe('local')
  })

  it('throws, rather than answering local, for a project the list does not hold', () => {
    configureHosts('host-1')
    loadProjects(project('p-local', 'local'))
    expect(() => hostIdForProject('p-missing')).toThrow(HostNotResolvedError)
  })

  it('throws while the project list has not loaded', () => {
    configureHosts('host-1')
    expect(() => hostIdForProject('p1')).toThrow(HostNotResolvedError)
  })

  it('keeps resolving from the held list while it re-reads or a re-read failed', () => {
    configureHosts('host-1')
    const held = success([project('p-remote', 'host-1')])
    useProjectDataStore.setState({ data: loading(held) })
    expect(hostIdForProject('p-remote')).toBe('host-1')
    useProjectDataStore.setState({ data: failed(new Error('boom'), held) })
    expect(hostIdForProject('p-remote')).toBe('host-1')
  })
})

describe('hostIdForWorkspace', () => {
  it('resolves through the project recorded for that workspace', () => {
    configureHosts('host-1')
    loadProjects(project('p-local', 'local'), project('p-remote', 'host-1'))
    recordScope('ws-remote', 'p-remote')
    recordScope('ws-local', 'p-local')
    expect(hostIdForWorkspace('ws-remote')).toBe('host-1')
    expect(hostIdForWorkspace('ws-local')).toBe('local')
  })

  it('throws for a workspace with no recorded scope even while a workspace on another host is active', () => {
    configureHosts('host-1')
    loadProjects(project('p-remote', 'host-1'))
    recordScope('ws-active', 'p-remote')
    bindActiveWorkspaceId(() => 'ws-active')
    expect(() => hostIdForWorkspace('ws-unrecorded')).toThrow(HostNotResolvedError)
  })

  it('never reads the active workspace when handed no id', () => {
    configureHosts('host-1')
    loadProjects(project('p-remote', 'host-1'))
    recordScope('ws-active', 'p-remote')
    bindActiveWorkspaceId(() => 'ws-active')
    expect(() => hostIdForWorkspace(undefined as unknown as string)).toThrow(HostNotResolvedError)
  })

  it('throws when the recorded project is not in the loaded list', () => {
    configureHosts('host-1')
    loadProjects(project('p-local', 'local'))
    recordScope('ws-1', 'p-unknown')
    expect(() => hostIdForWorkspace('ws-1')).toThrow(HostNotResolvedError)
  })

  it('is local with no hosts configured, even before a scope is recorded', () => {
    expect(hostIdForWorkspace('ws-unrecorded')).toBe('local')
  })
})

describe('useHostResolved', () => {
  it('flips for a project when the project list loads', () => {
    configureHosts('host-1')
    const { result } = renderHook(() => useHostResolved({ projectId: 'p-remote' }))
    expect(result.current).toBe(false)
    act(() => loadProjects(project('p-remote', 'host-1')))
    expect(result.current).toBe(true)
  })

  it('flips for a workspace only once both its scope is recorded and its project is loaded', () => {
    configureHosts('host-1')
    const { result } = renderHook(() => useHostResolved({ wsId: 'ws-1' }))
    expect(result.current).toBe(false)
    act(() => loadProjects(project('p-remote', 'host-1')))
    expect(result.current).toBe(false)
    act(() => recordScope('ws-1', 'p-remote'))
    expect(result.current).toBe(true)
  })

  it('is true at once when no hosts are configured and settles when the last host is removed', () => {
    const { result } = renderHook(() => useHostResolved({ projectId: 'p1' }))
    expect(result.current).toBe(true)
    act(() => configureHosts('host-1'))
    expect(result.current).toBe(false)
    act(() => configureHosts())
    expect(result.current).toBe(true)
  })

  it('does not flap while a loaded list re-reads', () => {
    configureHosts('host-1')
    const held = success([project('p-remote', 'host-1')])
    useProjectDataStore.setState({ data: held })
    const { result } = renderHook(() => useHostResolved({ projectId: 'p-remote' }))
    act(() => useProjectDataStore.setState({ data: loading(held) }))
    expect(result.current).toBe(true)
  })
})
