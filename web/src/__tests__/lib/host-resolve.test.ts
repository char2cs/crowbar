import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HostNotResolvedError } from '@/lib/host-path'
import {
  hostIdForChat,
  hostIdForProject,
  hostIdForWorkspace,
  isHostResolved,
  resolvedHostOf,
  useHostResolved,
  useResolvedHost,
} from '@/lib/host-resolve'
import {
  chatEntity,
  putEntities,
  resetEntities,
  seedProjects,
} from '@/__tests__/__fixtures__/sidebar-entities'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import { setActiveWorkspaceId } from '@/features/workspace/stores/workspace-store-registry'
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

// As pairing does: an install that ever had a host remembers it.
function configureHosts(...ids: string[]): void {
  useHostConfigStore.setState((s) => ({
    hosts: ids.map(host),
    remoteEverPaired: s.remoteEverPaired || ids.length > 0,
  }))
}

function loadProjects(...projects: Project[]): void {
  seedProjects(projects)
}

function unresolved(call: () => unknown): HostNotResolvedError {
  try {
    call()
  } catch (err) {
    expect(err).toBeInstanceOf(HostNotResolvedError)
    return err as HostNotResolvedError
  }
  throw new Error('expected the call to throw HostNotResolvedError')
}

function recordScope(wsId: string, projectId: string): void {
  recordWorkspaceScope({ projectId, repoId: `repo-of-${projectId}`, wsId })
}

beforeEach(() => {
  useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  resetEntities()
  __resetWorkspaceScopesForTest()
})

afterEach(() => {
  vi.restoreAllMocks()
})

describe('hostIdForProject', () => {
  it('is local with no hosts and no remote rows, even before the list loads', () => {
    expect(hostIdForProject('anything')).toBe('local')
    loadProjects()
    expect(hostIdForProject('anything')).toBe('local')
    loadProjects(project('p-local', 'local'))
    expect(hostIdForProject('p-local')).toBe('local')
    expect(hostIdForProject('p-unknown')).toBe('local')
  })

  it('returns the host recorded on a known remote project while that host is configured', () => {
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
    const err = unresolved(() => hostIdForProject('p-missing'))
    expect(err.message).toMatch(/project p-missing: the project is not in the project list/)
  })

  it('throws while the project list has not loaded', () => {
    configureHosts('host-1')
    expect(unresolved(() => hostIdForProject('p1')).message).toMatch(/project list is not loaded/)
  })

  // Settings -> Hosts -> Revoke removes the host, and its project rows with it.
  it('throws for a remote project once its host is no longer configured', () => {
    configureHosts('host-1')
    loadProjects(project('p-remote', 'host-1'))
    expect(hostIdForProject('p-remote')).toBe('host-1')
    configureHosts()
    expect(unresolved(() => hostIdForProject('p-remote')).message).toMatch(
      /not in the project list/,
    )
    expect(unresolved(() => hostIdForProject('p-other')).message).toMatch(/not in the project list/)
  })

  it('throws for a project on a host that is gone while another host is still configured', () => {
    configureHosts('host-1', 'host-2')
    loadProjects(project('p-on-1', 'host-1'))
    expect(hostIdForProject('p-on-1')).toBe('host-1')
    configureHosts('host-2')
    expect(unresolved(() => hostIdForProject('p-on-1')).message).toMatch(/not in the project list/)
  })
})

describe('after a remote host was paired and revoked', () => {
  function pairThenRevoke(): void {
    useHostConfigStore.getState().addHost(host('host-1'))
    loadProjects(project('p-local', 'local'), project('p-remote', 'host-1'))
    recordScope('ws-r', 'p-remote')
    useHostConfigStore.getState().removeHost('host-1')
  }

  it('never answers local for the remote ids, even when a re-read returns no remote rows', () => {
    const { result } = renderHook(() => useHostResolved({ wsId: 'ws-r' }))
    act(pairThenRevoke)
    expect(result.current).toBe(false)

    act(() => loadProjects(project('p-local', 'local')))
    expect(result.current).toBe(false)
    expect(unresolved(() => hostIdForWorkspace('ws-r')).projectId).toBe('p-remote')
    expect(unresolved(() => hostIdForProject('p-remote')).message).toMatch(
      /not in the project list/,
    )
  })

  it('still resolves a local project as soon as its row is held', () => {
    pairThenRevoke()
    loadProjects(project('p-local', 'local'))
    recordScope('ws-l', 'p-local')
    expect(hostIdForProject('p-local')).toBe('local')
    expect(hostIdForWorkspace('ws-l')).toBe('local')
  })

  it('does not resolve an unknown id, whether the list is empty or has not loaded', () => {
    pairThenRevoke()
    resetEntities()
    expect(() => hostIdForProject('p-unknown')).toThrow(HostNotResolvedError)
    loadProjects()
    expect(() => hostIdForProject('p-unknown')).toThrow(HostNotResolvedError)
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

  it('throws for a workspace with no recorded scope even while one on another host is recorded', () => {
    configureHosts('host-1')
    loadProjects(project('p-remote', 'host-1'))
    recordScope('ws-recorded', 'p-remote')
    const err = unresolved(() => hostIdForWorkspace('ws-unrecorded'))
    expect(err.message).toMatch(/workspace ws-unrecorded: no scope is recorded/)
  })

  it('throws, naming the project, when the recorded project is not in the loaded list', () => {
    configureHosts('host-1')
    loadProjects(project('p-local', 'local'))
    recordScope('ws-1', 'p-unknown')
    const err = unresolved(() => hostIdForWorkspace('ws-1'))
    expect(err.projectId).toBe('p-unknown')
    expect(err.message).toMatch(/workspace ws-1 \(project p-unknown\): the project is not in/)
  })

  it('is local with no hosts and no remote rows, even before a scope is recorded', () => {
    expect(hostIdForWorkspace('ws-unrecorded')).toBe('local')
    loadProjects(project('p-local', 'local'))
    expect(hostIdForWorkspace('ws-unrecorded')).toBe('local')
  })

  it('throws for a workspace whose project host is no longer configured', () => {
    configureHosts('host-1')
    loadProjects(project('p-remote', 'host-1'))
    recordScope('ws-remote', 'p-remote')
    expect(hostIdForWorkspace('ws-remote')).toBe('host-1')
    configureHosts()
    expect(unresolved(() => hostIdForWorkspace('ws-remote')).projectId).toBe('p-remote')
  })

  it('throws for an unrecorded workspace while remote rows are held, with or without hosts', () => {
    loadProjects(project('p-remote', 'host-1'))
    expect(() => hostIdForWorkspace('ws-unrecorded')).toThrow(HostNotResolvedError)
    configureHosts('host-1')
    expect(() => hostIdForWorkspace('ws-unrecorded')).toThrow(HostNotResolvedError)
  })
})

describe('hostIdForChat', () => {
  const ownedBy = (wsId: string, projectId: string, owningChatId: string) =>
    recordWorkspaceScope({ projectId, repoId: 'r1', wsId, owningChatId })

  it('resolves through the workspace that records the chat as its owner', () => {
    configureHosts('host-1')
    loadProjects(project('p-local', 'local'), project('p-remote', 'host-1'))
    ownedBy('ws-remote', 'p-remote', 'chat-r')
    ownedBy('ws-local', 'p-local', 'chat-l')
    expect(hostIdForChat('chat-r')).toBe('host-1')
    expect(hostIdForChat('chat-l')).toBe('local')
  })

  it('throws, naming the project, when the owner workspace cannot be placed', () => {
    configureHosts('host-1')
    ownedBy('ws-1', 'p-remote', 'chat-1')
    const err = unresolved(() => hostIdForChat('chat-1'))
    expect(err.entity).toBe('chat')
    expect(err.projectId).toBe('p-remote')
    expect(err.message).toMatch(/chat chat-1 \(project p-remote\): the project list is not loaded/)
  })

  it('throws for a chat no recorded scope owns once a remote was ever paired', () => {
    configureHosts('host-1')
    loadProjects(project('p-local', 'local'))
    ownedBy('ws-1', 'p-local', 'chat-1')
    expect(unresolved(() => hostIdForChat('sibling')).message).toMatch(/no workspace scope records/)
  })

  it('places a listed chat no scope owns (a sibling) by its own project', () => {
    configureHosts('host-1')
    loadProjects(project('p-local', 'local'), project('p-remote', 'host-1'))
    putEntities('chat', [chatEntity({ id: 'sibling', projectId: 'p-remote' })])
    expect(hostIdForChat('sibling')).toBe('host-1')
  })

  it('is local for an unowned chat only while no remote could exist', () => {
    expect(hostIdForChat('sibling')).toBe('local')
    loadProjects(project('p-local', 'local'))
    expect(hostIdForChat('sibling')).toBe('local')
  })
})

describe('isHostResolved', () => {
  it('is useHostResolved’s answer for code that is not a component', () => {
    configureHosts('host-1')
    expect(isHostResolved({ projectId: 'p-remote' })).toBe(false)
    loadProjects(project('p-remote', 'host-1'))
    expect(isHostResolved({ projectId: 'p-remote' })).toBe(true)
    expect(isHostResolved({ wsId: 'ws-1' })).toBe(false)
    recordScope('ws-1', 'p-remote')
    expect(isHostResolved({ wsId: 'ws-1' })).toBe(true)
  })

  it('never throws, where hostIdForProject does', () => {
    configureHosts('host-1')
    expect(() => hostIdForProject('p-unknown')).toThrow(HostNotResolvedError)
    expect(isHostResolved({ projectId: 'p-unknown' })).toBe(false)
  })
})

// Callers hand '' when they have no workspace yet. It must name nothing: falling back to
// the workspace in view is how a request reaches the wrong host. The registry owns which
// workspace is in view, so the test sets it there.
describe('a workspace id of ""', () => {
  beforeEach(() => {
    configureHosts('host-1')
    loadProjects(project('p-remote', 'host-1'))
    recordScope('ws-active', 'p-remote')
    setActiveWorkspaceId('ws-active')
  })
  afterEach(() => setActiveWorkspaceId(null))

  it('does not resolve to the host of the workspace in view', () => {
    expect(hostIdForWorkspace('ws-active')).toBe('host-1')
    expect(unresolved(() => hostIdForWorkspace('')).message).toMatch(/no scope is recorded/)
    expect(resolvedHostOf({ wsId: '' })).toBeNull()
  })

  it('is no host for a component either', () => {
    const { result } = renderHook(() => useResolvedHost({ wsId: '' }))
    expect(result.current).toBeNull()
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

  it('follows a changed wsId: the new workspace is awaited and flips when ITS scope is recorded', () => {
    configureHosts('host-1')
    loadProjects(project('p-remote', 'host-1'))
    recordScope('ws-1', 'p-remote')
    const { result, rerender } = renderHook(
      ({ wsId }: { wsId: string }) => useHostResolved({ wsId }),
      { initialProps: { wsId: 'ws-1' } },
    )
    expect(result.current).toBe(true)
    rerender({ wsId: 'ws-2' })
    expect(result.current).toBe(false)
    act(() => recordScope('ws-2', 'p-remote'))
    expect(result.current).toBe(true)
  })

  it('follows a changed projectId', () => {
    configureHosts('host-1')
    loadProjects(project('p-a', 'host-1'))
    const { result, rerender } = renderHook(
      ({ projectId }: { projectId: string }) => useHostResolved({ projectId }),
      { initialProps: { projectId: 'p-a' } },
    )
    expect(result.current).toBe(true)
    rerender({ projectId: 'p-missing' })
    expect(result.current).toBe(false)
    rerender({ projectId: 'p-a' })
    expect(result.current).toBe(true)
  })

  it('is true while no remote was ever paired, false once one is paired, and stays false after it is removed', () => {
    const { result } = renderHook(() => useHostResolved({ projectId: 'p1' }))
    expect(result.current).toBe(true)
    act(() => useHostConfigStore.getState().addHost(host('host-1')))
    expect(result.current).toBe(false)
    act(() => useHostConfigStore.getState().removeHost('host-1'))
    expect(result.current).toBe(false)
  })

  it('goes false when the last host is revoked, its project rows leaving with it', () => {
    configureHosts('host-1')
    loadProjects(project('p-remote', 'host-1'))
    const { result } = renderHook(() => useHostResolved({ projectId: 'p-remote' }))
    expect(result.current).toBe(true)
    act(() => configureHosts())
    expect(result.current).toBe(false)
  })
})

describe('resolvedHostOf', () => {
  it('names the host once it resolves and answers null, never local, until then', () => {
    configureHosts('host-1')
    recordScope('ws-1', 'p-remote')
    expect(resolvedHostOf({ wsId: 'ws-1' })).toBeNull()
    loadProjects(project('p-remote', 'host-1'), project('p-local', 'local'))
    expect(resolvedHostOf({ wsId: 'ws-1' })).toBe('host-1')
    expect(resolvedHostOf({ projectId: 'p-local' })).toBe('local')
    expect(resolvedHostOf({ projectId: 'p-gone' })).toBeNull()
  })
})

describe('useResolvedHost', () => {
  it('answers null, never local, until the host resolves, then names it', () => {
    configureHosts('host-1')
    recordScope('ws-1', 'p-remote')
    const { result } = renderHook(() => useResolvedHost({ wsId: 'ws-1' }))
    expect(result.current).toBeNull()
    act(() => loadProjects(project('p-remote', 'host-1')))
    expect(result.current).toBe('host-1')
    act(() => configureHosts())
    expect(result.current).toBeNull()
  })
})
