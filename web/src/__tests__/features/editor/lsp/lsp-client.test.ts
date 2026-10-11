import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// LspClient keeps one session per workspace, addressed by the id the caller passes (the buffer's
// own workspace), never the active one. A session lives while it has an open document or a
// diagnostics handler; it holds a socket while it lives and the owning chat and host are known.

const { apiFetch } = vi.hoisted(() => ({
  apiFetch: vi.fn(async (..._args: unknown[]): Promise<unknown> => null),
}))
const { subscribe, unsubscribes } = vi.hoisted(() => {
  const unsubscribes = new Map<string, ReturnType<typeof vi.fn>>()
  // A channel open at once: the session's documents go out as it subscribes.
  const subscribe = vi.fn(
    (topic: string, _cb: (frame: unknown) => void, onOpenChange?: (open: boolean) => void) => {
      const off = vi.fn()
      unsubscribes.set(topic, off)
      onOpenChange?.(true)
      return off
    },
  )
  return { subscribe, unsubscribes }
})

// apiBaseFor: what the real socket manager dials a remote host's stream through.
vi.mock('@/lib/api', () => ({ apiFetch, apiBaseFor: (hostId: string) => `https://${hostId}.test` }))
vi.mock('@/lib/ws/manager', () => ({ wsManager: { subscribe } }))

import { LspClient } from '@/features/editor/lsp/lsp-client'
import {
  __resetWorkspaceScopesForTest,
  forgetWorkspaceScope,
  recordWorkspaceScope,
  setWorkspaceScope,
} from '@/lib/workspace-scope'
import type { Project } from '@/lib/types'
import type { HostStatus } from '@/features/hosts/types'

const TOPIC_1 = '/v0/chats/chat-1/lsp/ws'
const TOPIC_2 = '/v0/chats/chat-2/lsp/ws'

const posts = () => apiFetch.mock.calls.map((c) => String(c[0]))
const postsTo = (suffix: string) => posts().filter((p) => p.endsWith(suffix))
const bodyOf = (index: number) =>
  JSON.parse(String((apiFetch.mock.calls[index]?.[1] as RequestInit).body)) as unknown

function frameHandler(topic: string): (frame: unknown) => void {
  const calls = subscribe.mock.calls as unknown as [string, (frame: unknown) => void][]
  const call = calls.filter(([t]) => t === topic).at(-1)
  if (!call) throw new Error(`never subscribed to ${topic}`)
  return call[1]
}

/** Sessions on the real socket manager, over sockets that open when the test says so. */
async function onRealSockets() {
  const sockets: Record<string, (() => void) | null>[] = []
  vi.stubGlobal(
    'WebSocket',
    class {
      static CONNECTING = 0
      static OPEN = 1
      close() {}
      constructor() {
        sockets.push(this as never)
      }
    },
  )
  const actual = await vi.importActual<typeof import('@/lib/ws/manager')>('@/lib/ws/manager')
  const manager = actual.createWSManager()
  const fake = subscribe.getMockImplementation()!
  subscribe.mockImplementation((topic, onFrame, onOpenChange) =>
    vi.fn(manager.subscribe(topic, onFrame, onOpenChange)),
  )
  const restore = () => {
    subscribe.mockImplementation(fake)
    vi.unstubAllGlobals()
  }
  return { sockets, restore }
}

const diag = (filePath: string) => ({
  filePath,
  range: { start: { line: 0, character: 0 }, end: { line: 0, character: 1 } },
  severity: 'error',
  message: 'boom',
})

beforeEach(() => {
  apiFetch.mockReset()
  apiFetch.mockResolvedValue(null)
  subscribe.mockClear()
  unsubscribes.clear()
  __resetWorkspaceScopesForTest()
  setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w1', owningChatId: 'chat-1' })
  recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w2', owningChatId: 'chat-2' })
})

describe('LspClient document sync', () => {
  it("addresses each document to its own workspace's chat, and says where it opened", async () => {
    const client = new LspClient()
    const opened = vi.fn()
    client.onDocumentOpened(opened)
    await client.documentOpen('w1', 'src/a.ts', () => 'one', 'typescript')
    await client.documentOpen('w2', 'src/a.ts', () => 'two', 'typescript')
    await client.documentSave('w2', 'src/a.ts')
    await client.documentClose('w1', 'src/a.ts')

    expect(posts()).toEqual([
      '/v0/chats/chat-1/lsp/didOpen',
      '/v0/chats/chat-2/lsp/didOpen',
      '/v0/chats/chat-2/lsp/didSave',
      '/v0/chats/chat-1/lsp/didClose',
    ])
    expect(bodyOf(1)).toEqual({ path: 'src/a.ts', languageId: 'typescript', text: 'two' })
    expect(opened).toHaveBeenLastCalledWith('w2', 'src/a.ts')
  })

  it('opens a document once however many panes hold it, and closes it with the last', async () => {
    const client = new LspClient()
    await client.documentClose('w1', 'a.ts')
    await client.documentSave('w1', 'a.ts')
    expect(apiFetch).not.toHaveBeenCalled()
    await client.documentOpen('w1', 'a.ts', () => 'x', 'typescript')
    await client.documentOpen('w1', 'a.ts', () => 'x', 'typescript')
    expect(postsTo('/didOpen')).toHaveLength(1)

    await client.documentClose('w1', 'a.ts')
    expect(postsTo('/didClose')).toHaveLength(0)
    await client.documentClose('w1', 'a.ts')
    expect(postsTo('/didClose')).toHaveLength(1)

    await client.documentOpen('w1', 'a.ts', () => 'x', 'typescript')
    expect(postsTo('/didOpen')).toHaveLength(2)
  })

  it('defers didOpen until the owning chat is recorded, with the latest text', async () => {
    setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w3' })
    const client = new LspClient()
    await client.documentOpen('w3', 'a.ts', () => 'old', 'typescript')
    client.scheduleChange('w3', 'a.ts', () => 'new')
    await client.flushChange('w3', 'a.ts')
    await expect(client.request('w3', 'hover', {})).resolves.toBeNull()
    expect(apiFetch).not.toHaveBeenCalled()
    expect(subscribe).not.toHaveBeenCalled()

    recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w3', owningChatId: 'chat-3' })

    expect(posts()).toEqual(['/v0/chats/chat-3/lsp/didOpen'])
    expect(bodyOf(0)).toEqual({ path: 'a.ts', languageId: 'typescript', text: 'new' })
    expect(subscribe.mock.calls[0]?.[0]).toBe('/v0/chats/chat-3/lsp/ws')
  })

  it('a document closed before its deferred didOpen went out is never sent', async () => {
    setWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w3' })
    const client = new LspClient()
    await client.documentOpen('w3', 'a.ts', () => 'code', 'typescript')
    await client.documentClose('w3', 'a.ts')
    recordWorkspaceScope({ projectId: 'p', repoId: 'r', wsId: 'w3', owningChatId: 'chat-3' })
    expect(apiFetch).not.toHaveBeenCalled()
    expect(subscribe).not.toHaveBeenCalled()
  })

  it('the home workspace has no LSP surface', async () => {
    setWorkspaceScope({ projectId: 'p', repoId: '', wsId: 'home-ws' })
    const client = new LspClient()
    client.onDiagnosticsUpdate('home-ws', () => {})
    await client.documentOpen('home-ws', 'a.md', () => 'x', 'markdown')
    expect(apiFetch).not.toHaveBeenCalled()
    expect(subscribe).not.toHaveBeenCalled()
  })

  it('reopen re-sends didOpen only for an open document', async () => {
    const client = new LspClient()
    await client.reopen('w1', 'a.ts', 'x', 'typescript')
    expect(apiFetch).not.toHaveBeenCalled()
    await client.documentOpen('w1', 'a.ts', () => 'x', 'typescript')
    await client.reopen('w1', 'a.ts', 'y', 'typescript')
    expect(postsTo('/didOpen')).toHaveLength(2)
  })
})

describe('LspClient didChange debounce and flush', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  const changes = () => apiFetch.mock.calls.filter((c) => String(c[0]).endsWith('/didChange'))

  it('coalesces a burst of edits into one didChange carrying the latest text', async () => {
    const client = new LspClient()
    await client.documentOpen('w2', 'a.ts', () => 'a', 'typescript')
    let text = 'a'
    client.scheduleChange('w2', 'a.ts', () => text)
    text = 'ab'
    client.scheduleChange('w2', 'a.ts', () => text)
    text = 'abc'
    await vi.advanceTimersByTimeAsync(1000)

    expect(changes()).toHaveLength(1)
    expect(String(changes()[0]?.[0])).toBe('/v0/chats/chat-2/lsp/didChange')
    expect(JSON.parse(String((changes()[0]?.[1] as RequestInit).body)).text).toBe('abc')
  })

  it('flushChange sends the pending text immediately, once', async () => {
    const client = new LspClient()
    await client.documentOpen('w1', 'a.ts', () => 'foo', 'typescript')
    client.scheduleChange('w1', 'a.ts', () => 'foo.')
    await client.flushChange('w1', 'a.ts')
    expect(changes()).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(1000)
    expect(changes()).toHaveLength(1)
  })

  it('sends nothing when the text is gone by flush time, or the document is not open', async () => {
    const client = new LspClient()
    client.scheduleChange('w1', 'closed.ts', () => 'x')
    await client.documentOpen('w1', 'a.ts', () => 'x', 'typescript')
    client.scheduleChange('w1', 'a.ts', () => null)
    await vi.advanceTimersByTimeAsync(1000)
    expect(changes()).toHaveLength(0)
  })

  it('closing a document drops its pending change', async () => {
    const client = new LspClient()
    await client.documentOpen('w1', 'a.ts', () => 'x', 'typescript')
    client.scheduleChange('w1', 'a.ts', () => 'y')
    await client.documentClose('w1', 'a.ts')
    await vi.advanceTimersByTimeAsync(1000)
    expect(changes()).toHaveLength(0)
  })
})

describe('LspClient diagnostics subscription', () => {
  it("subscribes to a workspace's topic with its first holder and delivers only its batches", () => {
    const client = new LspClient()
    const h1 = vi.fn()
    const h2 = vi.fn()
    client.onDiagnosticsUpdate('w1', h1)
    client.onDiagnosticsUpdate('w2', h2)
    expect(subscribe.mock.calls.map(([t]) => t)).toEqual([TOPIC_1, TOPIC_2])

    frameHandler(TOPIC_1)({ wsId: 'w1', diagnostics: [diag('src/shared.ts')] })
    expect(h1).toHaveBeenCalledWith('src/shared.ts', [diag('src/shared.ts')])
    expect(h2).not.toHaveBeenCalled()
  })

  it('replays the last batch to a late handler and clears files that lost their diagnostics', () => {
    const client = new LspClient()
    client.onDiagnosticsUpdate('w1', () => {})
    frameHandler(TOPIC_1)({ wsId: 'w1', diagnostics: [diag('a.ts')] })

    const late = vi.fn()
    client.onDiagnosticsUpdate('w1', late)
    expect(late).toHaveBeenCalledWith('a.ts', [diag('a.ts')])

    frameHandler(TOPIC_1)({ wsId: 'w1', diagnostics: [] })
    expect(late).toHaveBeenLastCalledWith('a.ts', [])
    expect(subscribe).toHaveBeenCalledTimes(1)
  })

  it('unsubscribes when the last handler and document of the workspace leave', async () => {
    const client = new LspClient()
    const off1 = client.onDiagnosticsUpdate('w1', () => {})
    const off2 = client.onDiagnosticsUpdate('w1', () => {})
    await client.documentOpen('w1', 'a.ts', () => 'x', 'typescript')
    client.onDiagnosticsUpdate('w2', () => {})

    off1()
    off2()
    expect(unsubscribes.get(TOPIC_1)).not.toHaveBeenCalled()
    await client.documentClose('w1', 'a.ts')
    expect(unsubscribes.get(TOPIC_1)).toHaveBeenCalledTimes(1)
    expect(unsubscribes.get(TOPIC_2)).not.toHaveBeenCalled()

    client.onDiagnosticsUpdate('w1', () => {})
    expect(subscribe.mock.calls.filter(([t]) => t === TOPIC_1)).toHaveLength(2)
  })

  it('unsubscribes immediately when the workspace is deleted, and never resubscribes', async () => {
    const client = new LspClient()
    const handler = vi.fn()
    client.onDiagnosticsUpdate('w2', handler)
    await client.documentOpen('w2', 'a.ts', () => 'x', 'typescript')
    frameHandler(TOPIC_2)({ wsId: 'w2', diagnostics: [diag('a.ts')] })

    forgetWorkspaceScope('w2')

    expect(unsubscribes.get(TOPIC_2)).toHaveBeenCalledTimes(1)
    expect(handler).toHaveBeenLastCalledWith('a.ts', [])
    apiFetch.mockClear()
    await client.documentSave('w2', 'a.ts')
    await client.documentClose('w2', 'a.ts')
    client.onDiagnosticsUpdate('w2', () => {})
    expect(apiFetch).not.toHaveBeenCalled()
    expect(subscribe.mock.calls.filter(([t]) => t === TOPIC_2)).toHaveLength(1)
  })
})

describe('LspClient.request', () => {
  it("POSTs to the given workspace's owning chat, with the caller's abort signal", async () => {
    const range = { start: { line: 3, character: 2 }, end: { line: 3, character: 9 } }
    apiFetch.mockResolvedValue([{ filePath: 'src/lib/target.ts', range }])
    const { signal } = new AbortController()
    const body = { path: 'src/app.ts', position: { line: 12, character: 4 } }

    const result = await new LspClient().request('w2', 'definition', body, signal)

    expect(result).toEqual([{ filePath: 'src/lib/target.ts', range }])
    expect(posts()).toEqual(['/v0/chats/chat-2/lsp/definition'])
    expect(apiFetch.mock.calls[0]?.[1]).toMatchObject({ method: 'POST', signal })
    expect(bodyOf(0)).toEqual(body)
  })

  it('propagates a daemon failure so the caller can surface it', async () => {
    apiFetch.mockRejectedValue(new Error('lsp: textDocument/definition: timeout'))
    await expect(new LspClient().request('w1', 'definition', {})).rejects.toThrow('timeout')
  })

  it('reads the server status and restarts through the daemon', async () => {
    const running = { state: 'running', command: 'gopls', languageId: 'go' }
    apiFetch.mockResolvedValue(running)
    const client = new LspClient()

    expect(await client.status('w1', 'src/main go.go')).toEqual(running)
    expect(posts()[0]).toBe('/v0/chats/chat-1/lsp/status?path=src%2Fmain%20go.go')

    await client.restart('w1', 'main.go')
    expect(posts()[1]).toBe('/v0/chats/chat-1/lsp/restart')
    expect(bodyOf(1)).toEqual({ path: 'main.go' })
  })
})

// A remote workspace's LSP route names its host: unknown until the project list loads, gone when
// the host is revoked. Neither may throw out of a scope write or leave a socket behind. Each test
// loads a fresh module graph, so a session another test left open cannot react to its changes.
describe('LspClient on a remote host', () => {
  const REMOTE_TOPIC = 'crowbar://host-1/v0/chats/chat-4/lsp/ws'
  const remoteProject = (): Project => ({
    id: 'p-r',
    name: 'p-r',
    path: '/work/p-r',
    lastActivity: new Date(0),
    hostId: 'host-1',
  })
  const remoteScope = { projectId: 'p-r', repoId: 'r', wsId: 'w-remote', owningChatId: 'chat-4' }

  async function freshGraph() {
    vi.resetModules()
    const [lsp, scopes, hosts, projects, loadable, runtimes] = await Promise.all([
      import('@/features/editor/lsp/lsp-client'),
      import('@/lib/workspace-scope'),
      import('@/features/hosts/stores/host-config-store'),
      import('@/lib/store/projects'),
      import('@/lib/loadable'),
      import('@/features/hosts/stores/host-runtime-registry'),
    ])
    const pairHost = () =>
      hosts.useHostConfigStore.getState().addHost({
        id: 'host-1',
        name: 'host-1',
        endpoint: 'host-1.example:3737',
        pairedFingerprint: 'SHA256:aaaa',
        clientId: 'client-1',
      })
    pairHost()
    return {
      client: new lsp.LspClient(),
      scopes,
      pairHost,
      removeHost: () => hosts.useHostConfigStore.getState().removeHost('host-1'),
      setStatus: (status: HostStatus) =>
        runtimes.getOrCreateHostRuntime('host-1').getState().setStatus(status),
      loadProjects: (...rows: Project[]) =>
        projects.useProjectDataStore.setState({ data: loadable.success(rows) }),
    }
  }

  it('defers an open made before the project list loads, sends it once when the host resolves, and again with what was typed once it resolves anew', async () => {
    const { client, scopes, loadProjects, removeHost, pairHost } = await freshGraph()
    scopes.recordWorkspaceScope(remoteScope)
    let text = 'x'

    await expect(
      client.documentOpen('w-remote', 'a.ts', () => text, 'typescript'),
    ).resolves.toBeUndefined()
    expect(() => scopes.recordWorkspaceScope(remoteScope)).not.toThrow()
    expect(apiFetch).not.toHaveBeenCalled()
    expect(subscribe).not.toHaveBeenCalled()

    loadProjects(remoteProject())

    expect(subscribe.mock.calls.map(([topic]) => topic)).toEqual([REMOTE_TOPIC])
    expect(posts()).toEqual(['crowbar://host-1/v0/chats/chat-4/lsp/didOpen'])
    scopes.recordWorkspaceScope(remoteScope)
    expect(postsTo('/didOpen')).toHaveLength(1)

    removeHost()
    text = 'y'
    client.scheduleChange('w-remote', 'a.ts', () => text)
    await client.flushChange('w-remote', 'a.ts')
    pairHost()
    expect(subscribe.mock.calls.map(([topic]) => topic)).toEqual([REMOTE_TOPIC, REMOTE_TOPIC])
    expect(postsTo('/didOpen')).toHaveLength(2)
    expect(bodyOf(1)).toMatchObject({ path: 'a.ts', text: 'y' })
  })

  it('answers a request or status with nothing, not a throw, until the host resolves', async () => {
    const { client, scopes, loadProjects } = await freshGraph()
    scopes.recordWorkspaceScope(remoteScope)
    await expect(client.request('w-remote', 'hover', {})).resolves.toBeNull()
    await expect(client.status('w-remote', 'a.ts')).resolves.toBeNull()
    expect(apiFetch).not.toHaveBeenCalled()

    loadProjects(remoteProject())
    await client.request('w-remote', 'hover', {})
    expect(posts()).toEqual(['crowbar://host-1/v0/chats/chat-4/lsp/hover'])
  })

  it('closes its socket when the host is revoked, and a later scope write neither throws nor starves a listener', async () => {
    const { client, scopes, loadProjects, removeHost } = await freshGraph()
    loadProjects(remoteProject())
    scopes.recordWorkspaceScope({ ...remoteScope, repoId: 'r1', wsId: 'ws-1' })
    client.onDiagnosticsUpdate('ws-1', () => {})
    expect(subscribe).toHaveBeenCalledTimes(1)
    const topic = String(subscribe.mock.calls[0]?.[0])

    removeHost()
    expect(unsubscribes.get(topic)).toHaveBeenCalledTimes(1)
    loadProjects() // the re-read that no longer holds the host's rows

    const later = vi.fn()
    scopes.subscribeToWorkspaceScope('ws-1', later)
    expect(() => scopes.recordWorkspaceScopeFromPath('/ide/p-r/r1/ws-1')).not.toThrow()
    expect(later).toHaveBeenCalledTimes(1)
    expect(subscribe).toHaveBeenCalledTimes(1)
  })

  // Re-paired in place, a host keeps its id and so the route base: the socket manager parks the
  // channel while the host needs repair and reopens it once it is up. Each open document goes on
  // each socket only once it is open: the daemon closes them all when a workspace's socket drops.
  it('opens what is open, with its text now, once a reopened or new socket is open', async () => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
    const { client, scopes, loadProjects, setStatus } = await freshGraph()
    const real = await onRealSockets()
    try {
      setStatus('up')
      loadProjects(remoteProject())
      scopes.recordWorkspaceScope(remoteScope)
      let text = 'a1'
      for (const path of ['a.ts', 'b.ts', 'closed.ts'])
        await client.documentOpen('w-remote', path, () => (path === 'a.ts' ? text : path), 'ts')
      expect(posts()).toEqual([])
      real.sockets[0].onopen?.()
      await client.documentClose('w-remote', 'closed.ts')
      expect(postsTo('/didOpen')).toHaveLength(3)

      setStatus('needs-repair')
      real.sockets[0].onclose?.()
      apiFetch.mockClear()
      text = 'a2'
      client.scheduleChange('w-remote', 'a.ts', () => text)
      await client.documentOpen('w-remote', 'd.ts', () => 'd.ts', 'ts')
      await vi.advanceTimersByTimeAsync(60_000)
      expect(real.sockets).toHaveLength(1)

      setStatus('up')
      expect(posts()).toEqual([])
      real.sockets[1].onopen?.()

      expect(subscribe).toHaveBeenCalledTimes(1)
      expect(posts()).toEqual(Array(3).fill(REMOTE_TOPIC.replace(/ws$/, 'didOpen')))
      expect([0, 1, 2].map(bodyOf)).toMatchObject([
        { path: 'a.ts', text: 'a2' },
        { path: 'b.ts', text: 'b.ts' },
        { path: 'd.ts', text: 'd.ts' },
      ])

      apiFetch.mockClear()
      scopes.recordWorkspaceScope({ ...remoteScope, owningChatId: 'chat-5' })
      expect(posts()).toEqual([])
      real.sockets[2].onopen?.()
      expect(posts()).toEqual(Array(3).fill('crowbar://host-1/v0/chats/chat-5/lsp/didOpen'))
    } finally {
      real.restore()
      vi.useRealTimers()
    }
  })

  it('leaves no scope listener behind when a new session fails to open its socket', async () => {
    const { client, scopes, loadProjects } = await freshGraph()
    loadProjects(remoteProject())
    scopes.recordWorkspaceScope(remoteScope)
    subscribe.mockImplementationOnce(() => {
      throw new Error('socket refused')
    })
    expect(() => client.onDiagnosticsUpdate('w-remote', () => {})).toThrow('socket refused')

    scopes.recordWorkspaceScope(remoteScope)

    expect(subscribe).toHaveBeenCalledTimes(1)
  })
})
