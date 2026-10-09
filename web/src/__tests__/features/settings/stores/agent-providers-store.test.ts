/**
 * The GLOBAL agent-provider list.
 *
 * Providers are machine-level — the daemon's own comment on ListProviders says
 * the workspaceID "is only used to resolve crowbar home" — but the frontend kept
 * its only copy inside the per-workspace store. The Settings dialog is global, so
 * opening it with no active workspace (Project Home, the projects screen, mid
 * switch) read an empty list and rendered "No providers available." over a daemon
 * that had them, enabled. This store is the copy that does not depend on a
 * workspace being in view.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const { listProvidersFn, listGlobalProvidersFn } = vi.hoisted(() => ({
  listProvidersFn: vi.fn(),
  listGlobalProvidersFn: vi.fn(),
}))

vi.mock('@/features/agent/api/agent-api', () => ({
  getPendingPrompt: vi.fn().mockResolvedValue(null),
  listProviders: (...a: unknown[]) => listProvidersFn(...a),
  listGlobalProviders: (...a: unknown[]) => listGlobalProvidersFn(...a),
}))

import {
  providersOf,
  useAgentProvidersStore,
} from '@/features/settings/stores/agent-providers-store'
import type { AgentProvider } from '@/features/agent/api/agent-api'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'

const provider = (id: string, enabled = true): AgentProvider => ({
  id,
  displayName: id,
  icon: '',
  connected: true,
  enabled,
  mcpEnabled: true,
  modelSelect: false,
  effortSelect: false,
  compaction: false,
  hasTerminal: true,
  hotswap: false,
  terminalStartHere: false,
})

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

const state = () => useAgentProvidersStore.getState()

beforeEach(() => {
  listProvidersFn.mockReset()
  listGlobalProvidersFn.mockReset()
  useAgentProvidersStore.setState({ status: 'idle', providers: [], remote: {} })
})

afterEach(() => {
  useAgentProvidersStore.setState({ status: 'idle', providers: [], remote: {} })
  resetHostWorld()
})

describe('agent providers store', () => {
  it('starts idle and empty', () => {
    expect(state().status).toBe('idle')
    expect(state().providers).toEqual([])
  })

  it('reports LOADING while the first fetch is in flight, then READY', async () => {
    const inflight = deferred<AgentProvider[]>()
    listProvidersFn.mockReturnValueOnce(inflight.promise)

    const done = state().load('w1')
    expect(state().status).toBe('loading')

    inflight.resolve([provider('claude')])
    await done

    expect(state().status).toBe('ready')
    expect(state().providers.map((p) => p.id)).toEqual(['claude'])
  })

  // The three states the tab used to collapse into one sentence. "No providers
  // available." is a claim about the DAEMON; it must never be what a failed
  // fetch looks like.
  it('reports FAILED when the fetch rejects and nothing was ever loaded', async () => {
    listProvidersFn.mockRejectedValueOnce(new Error('daemon is down'))

    await state().load('w1')

    expect(state().status).toBe('failed')
    expect(state().providers).toEqual([])
  })

  it('reports READY and EMPTY when the daemon genuinely has none', async () => {
    listProvidersFn.mockResolvedValueOnce([])

    await state().load('w1')

    expect(state().status).toBe('ready')
    expect(state().providers).toEqual([])
  })

  it('keeps a good list when a later refresh fails — a blip must not empty the UI', async () => {
    listProvidersFn.mockResolvedValueOnce([provider('claude')])
    await state().load('w1')

    listProvidersFn.mockRejectedValueOnce(new Error('blip'))
    await state().load('w1')

    expect(state().providers.map((p) => p.id)).toEqual(['claude'])
    expect(state().status).toBe('ready')
  })

  it('does not flash LOADING over a list it already has', async () => {
    listProvidersFn.mockResolvedValueOnce([provider('claude')])
    await state().load('w1')

    const inflight = deferred<AgentProvider[]>()
    listProvidersFn.mockReturnValueOnce(inflight.promise)
    const done = state().load('w1')
    expect(state().status).toBe('ready')

    inflight.resolve([provider('claude'), provider('codex')])
    await done
  })

  it('does not flash LOADING over a settled EMPTY answer either', async () => {
    // "The daemon has none" is a settled answer too. Flipping back to a spinner
    // on every refresh makes the tab strobe between two sentences.
    listProvidersFn.mockResolvedValueOnce([])
    await state().load('w1')

    const inflight = deferred<AgentProvider[]>()
    listProvidersFn.mockReturnValueOnce(inflight.promise)
    const done = state().load('w1')
    expect(state().status).toBe('ready')

    inflight.resolve([])
    await done
  })

  it('goes back to LOADING when retrying after a failure', async () => {
    listProvidersFn.mockRejectedValueOnce(new Error('down'))
    await state().load('w1')
    expect(state().status).toBe('failed')

    const inflight = deferred<AgentProvider[]>()
    listProvidersFn.mockReturnValueOnce(inflight.promise)
    const done = state().load('w1')
    expect(state().status).toBe('loading')

    inflight.resolve([provider('claude')])
    await done
    expect(state().status).toBe('ready')
  })

  it('lets only the LATEST load write', async () => {
    const older = deferred<AgentProvider[]>()
    listProvidersFn.mockReturnValueOnce(older.promise)
    const first = state().load('w1')

    listProvidersFn.mockResolvedValueOnce([provider('codex')])
    await state().load('w1')

    older.resolve([provider('stale')])
    await first

    expect(state().providers.map((p) => p.id)).toEqual(['codex'])
  })

  it('setProviders writes a list resolved elsewhere (a workspace seed, a preferences PUT)', () => {
    state().setProviders([provider('claude')])

    expect(state().status).toBe('ready')
    expect(state().providers.map((p) => p.id)).toEqual(['claude'])
  })

  it('markUnavailable says the list could not be reached, without claiming there are none', () => {
    state().markUnavailable()

    expect(state().status).toBe('failed')
    expect(state().providers).toEqual([])
  })

  it('markUnavailable never discards a list already loaded', () => {
    state().setProviders([provider('claude')])
    state().markUnavailable()

    expect(state().status).toBe('ready')
    expect(state().providers.map((p) => p.id)).toEqual(['claude'])
  })
})

// Providers belong to a daemon. The fields above are the LOCAL daemon's — what
// Settings edits — and a remote daemon's list lives beside them, by host.
describe('agent providers store across hosts', () => {
  beforeEach(() => {
    resetHostWorld()
    pairRemoteHost('host-1')
    __resetWorkspaceScopesForTest()
    recordWorkspaceScope({ projectId: 'p-remote', repoId: 'r', wsId: 'w-remote' })
    recordWorkspaceScope({ projectId: 'p-local', repoId: 'r', wsId: 'w-local' })
    loadProjects(projectOn('p-remote', 'host-1'), projectOn('p-local', 'local'))
  })

  it('load reads a local workspace’s daemon into the local list', async () => {
    listProvidersFn.mockResolvedValueOnce([provider('claude')])

    await state().load('w-local')

    expect(state().providers.map((p) => p.id)).toEqual(['claude'])
  })

  it('load asks nothing and writes nothing for a workspace on a remote host', async () => {
    state().setProviders([provider('claude')])

    await expect(state().load('w-remote')).resolves.toBeNull()

    expect(listProvidersFn).not.toHaveBeenCalled()
    expect(state().providers.map((p) => p.id)).toEqual(['claude'])
  })

  it('loadHost reads that host’s own daemon and leaves the local list alone', async () => {
    state().setProviders([provider('claude')])
    listGlobalProvidersFn.mockResolvedValueOnce([provider('codex')])

    await state().loadHost('host-1')

    expect(listGlobalProvidersFn).toHaveBeenCalledWith('host-1')
    expect(providersOf(state(), 'host-1').map((p) => p.id)).toEqual(['codex'])
    expect(providersOf(state(), 'local').map((p) => p.id)).toEqual(['claude'])
    expect(state().status).toBe('ready')
  })

  it('loadHost leaves a host with no answer unlisted', async () => {
    listGlobalProvidersFn.mockRejectedValueOnce(new Error('unreachable'))

    await expect(state().loadHost('host-1')).resolves.toBeNull()

    expect(state().remote['host-1']).toBeUndefined()
    expect(providersOf(state(), 'host-1')).toEqual([])
  })

  it('setHostProviders files a remote list by host, and the local host’s as the local list', () => {
    state().setHostProviders('host-1', [provider('codex')])
    state().setHostProviders('local', [provider('claude')])

    expect(providersOf(state(), 'host-1').map((p) => p.id)).toEqual(['codex'])
    expect(state().providers.map((p) => p.id)).toEqual(['claude'])
    expect(state().remote.local).toBeUndefined()
  })
})
