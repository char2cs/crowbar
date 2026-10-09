import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { HostNotResolvedError } from '@/lib/host-path'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { idle, success } from '@/lib/loadable'
import {
  __resetWorkspaceScopesForTest,
  getWorkspaceScope,
  recordWorkspaceScope,
  type WorkspaceScope,
} from '@/lib/workspace-scope'
import {
  chatBase,
  chatTerminalsBaseForWorkspace,
  filesBaseForWorkspace,
  gitBaseForWorkspace,
  identityBaseForWorkspace,
  isHomeWorkspace,
  lspBaseForWorkspace,
  OwningChatNotRecordedError,
  repoChatsBaseForWorkspace,
  reviewBaseForWorkspace,
  terminalsBaseForWorkspace,
  workspaceBase,
  worktreeVerbBaseForWorkspace,
} from '@/lib/workspace-scope-url'
import type { Host } from '@/features/hosts/types'
import type { Project } from '@/lib/types'

// A repo workspace with an owning chat, and a project HOME (no repo, no chat).
const REPO_WS = 'ws-1'
const HOME_WS = 'home-ws'
const REPO: WorkspaceScope = {
  projectId: 'p1',
  repoId: 'r1',
  wsId: REPO_WS,
  owningChatId: 'chat-1',
}
const HOME: WorkspaceScope = { projectId: 'p1', repoId: '', wsId: HOME_WS }

const THROWS = 'throws'
interface Case {
  name: string
  call: (wsId: string) => string
  /** The bare `/v0/...` string recorded from the builder BEFORE it became host-aware. */
  repo: string
  home: string | typeof THROWS
}
const BUILDERS: Case[] = [
  {
    name: 'terminals',
    call: terminalsBaseForWorkspace,
    repo: '/v0/chats/chat-1/terminals',
    home: '/v0/projects/p1/home/terminals',
  },
  { name: 'lsp', call: lspBaseForWorkspace, repo: '/v0/chats/chat-1/lsp', home: THROWS },
  { name: 'git', call: gitBaseForWorkspace, repo: '/v0/chats/chat-1/git', home: THROWS },
  { name: 'review', call: reviewBaseForWorkspace, repo: '/v0/chats/chat-1/review', home: THROWS },
  {
    name: 'identity',
    call: identityBaseForWorkspace,
    repo: '/v0/chats/chat-1/identity',
    home: THROWS,
  },
  {
    name: 'files',
    call: filesBaseForWorkspace,
    repo: '/v0/chats/chat-1/files',
    home: '/v0/projects/p1/home/files',
  },
  {
    name: 'repoChats',
    call: repoChatsBaseForWorkspace,
    repo: '/v0/projects/p1/repos/r1/chats',
    home: '/v0/projects/p1/home/chats',
  },
  {
    name: 'worktreeVerb',
    call: worktreeVerbBaseForWorkspace,
    repo: '/v0/projects/p1/repos/r1/chats/chat-1',
    home: THROWS,
  },
  {
    name: 'workspaceBase',
    call: workspaceBase,
    repo: '/v0/projects/p1/repos/r1/workspaces/ws-1',
    home: '/v0/projects/p1/home',
  },
]
/** Every builder, including the chat-id and chat+workspace ones, by what it returns for REPO_WS. */
const REPO_URLS: [string, () => string, string][] = [
  ...BUILDERS.map((b): [string, () => string, string] => [b.name, () => b.call(REPO_WS), b.repo]),
  ['chatBase', () => chatBase('chat-1'), '/v0/chats/chat-1'],
  [
    'chatTerminalsBase',
    () => chatTerminalsBaseForWorkspace(REPO_WS, 'chat-1'),
    '/v0/chats/chat-1/terminals',
  ],
]

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
const pair = (...ids: string[]) =>
  ids.forEach((id) => useHostConfigStore.getState().addHost(host(id)))
const loadProjects = (...rows: Project[]) => useProjectDataStore.setState({ data: success(rows) })

function resolution(call: () => unknown): HostNotResolvedError {
  try {
    call()
  } catch (err) {
    expect(err).toBeInstanceOf(HostNotResolvedError)
    return err as HostNotResolvedError
  }
  throw new Error('expected HostNotResolvedError, the builder returned')
}

beforeEach(() => {
  // addHost sets a persisted one-way bit; a later test must start never-paired.
  useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  useProjectDataStore.setState({ data: idle() })
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope(REPO)
  recordWorkspaceScope(HOME)
})

afterEach(() => {
  delete (window as { __CROWBAR__?: unknown }).__CROWBAR__
  vi.resetModules()
})

describe('golden: no remote ever paired (I3) — byte-identical to the pre-host-aware builders', () => {
  it.each(BUILDERS)('$name returns the recorded bare path for a repo workspace', (b) => {
    expect(b.call(REPO_WS)).toBe(b.repo)
  })

  it.each(BUILDERS)('$name for the project home', (b) => {
    if (b.home === THROWS) expect(() => b.call(HOME_WS)).toThrow(OwningChatNotRecordedError)
    else expect(b.call(HOME_WS)).toBe(b.home)
  })

  it('chat-id and chat+workspace builders return the recorded bare paths, ids encoded', () => {
    expect(chatBase('chat-1')).toBe('/v0/chats/chat-1')
    expect(chatBase('a/b')).toBe('/v0/chats/a%2Fb')
    expect(chatTerminalsBaseForWorkspace(REPO_WS, 'sibling/1')).toBe(
      '/v0/chats/sibling%2F1/terminals',
    )
  })

  it('encodes scope ids and keeps the existing errors for an unrecorded scope', () => {
    recordWorkspaceScope({ projectId: 'p/1', repoId: 'r 1', wsId: 'w/1', owningChatId: 'c/1' })
    expect(workspaceBase('w/1')).toBe('/v0/projects/p%2F1/repos/r%201/workspaces/w%2F1')
    expect(worktreeVerbBaseForWorkspace('w/1')).toBe('/v0/projects/p%2F1/repos/r%201/chats/c%2F1')
    expect(() => workspaceBase('nope')).toThrow('no project/repo scope recorded for workspace nope')
    expect(() => gitBaseForWorkspace('nope')).toThrow(OwningChatNotRecordedError)
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'no-chat' })
    expect(() => filesBaseForWorkspace('no-chat')).toThrow('no owning chat recorded')
  })

  it('isHomeWorkspace stays a plain predicate', () => {
    expect([HOME_WS, REPO_WS, 'unknown'].map(isHomeWorkspace)).toEqual([true, false, false])
  })

  it('does not consult the project list before it loads, even with a local row held', () => {
    loadProjects(project('p1', 'local'))
    expect(gitBaseForWorkspace(REPO_WS)).toBe('/v0/chats/chat-1/git')
  })
})

// jsdom runs with API_BASE='' but the desktop's is `crowbar://localhost`: a builder
// that fed an API_BASE-prefixed string to qualify('local', …) would throw only there.
// The builders also sit in an import cycle with the transport (workspace-scope-url ->
// host-resolve -> projects -> api -> workspace-scope-url), so each member of it must
// work when it is the first module loaded.
const CYCLE = [
  '@/lib/api',
  '@/lib/crowbar-bridge',
  '@/lib/ws/url',
  '@/lib/ws/manager',
  '@/lib/store/loadable-slice',
  '@/lib/store/projects',
  '@/lib/host-resolve',
  '@/lib/workspace-scope-url',
]
describe.each([
  ['browser', ''],
  ['desktop', 'crowbar://localhost'],
])('under the %s API base', (_name, api) => {
  it.each(CYCLE)('the builders are unchanged when %s loads first', async (entry) => {
    vi.resetModules()
    ;(window as { __CROWBAR__?: unknown }).__CROWBAR__ = { api }
    await import(/* @vite-ignore */ entry)
    const urls = await import('@/lib/workspace-scope-url')
    const scopes = await import('@/lib/workspace-scope')
    scopes.recordWorkspaceScope(REPO)
    scopes.recordWorkspaceScope(HOME)
    expect((await import('@/lib/api')).API_BASE).toBe(api)
    expect(urls.gitBaseForWorkspace(REPO_WS)).toBe('/v0/chats/chat-1/git')
    expect(urls.workspaceBase(REPO_WS)).toBe('/v0/projects/p1/repos/r1/workspaces/ws-1')
    expect(urls.terminalsBaseForWorkspace(HOME_WS)).toBe('/v0/projects/p1/home/terminals')
    expect(urls.chatBase('chat-1')).toBe('/v0/chats/chat-1')
  })
})

describe('a remote project: every builder returns the crowbar://<host> form', () => {
  beforeEach(() => {
    pair('host-1', 'host-2')
    loadProjects(project('p1', 'host-2'), project('p-local', 'local'))
  })

  it.each(REPO_URLS)('%s', (_name, call, bare) => {
    expect(call()).toBe(`crowbar://host-2${bare}`)
  })

  it.each(BUILDERS.filter((b) => b.home !== THROWS))('$name for the remote project home', (b) => {
    expect(b.call(HOME_WS)).toBe(`crowbar://host-2${b.home}`)
  })

  it('stays bare for a local project while a remote is configured', () => {
    recordWorkspaceScope({
      projectId: 'p-local',
      repoId: 'r9',
      wsId: 'ws-local',
      owningChatId: 'c9',
    })
    expect(gitBaseForWorkspace('ws-local')).toBe('/v0/chats/c9/git')
    expect(chatBase('c9')).toBe('/v0/chats/c9')
    expect(chatTerminalsBaseForWorkspace('ws-local', 'sibling-chat')).toBe(
      '/v0/chats/sibling-chat/terminals',
    )
  })

  it('qualifies a sibling chat through the workspace it is opened in, not the chat id', () => {
    // A sibling chat is not any workspace's owning chat, so its id alone proves nothing.
    expect(chatTerminalsBaseForWorkspace(REPO_WS, 'sibling-chat')).toBe(
      'crowbar://host-2/v0/chats/sibling-chat/terminals',
    )
  })

  it('reads the host from the workspace being asked about, never from another', () => {
    recordWorkspaceScope({
      projectId: 'p-local',
      repoId: 'r9',
      wsId: 'ws-local',
      owningChatId: 'c9',
    })
    expect(gitBaseForWorkspace('ws-local')).toBe('/v0/chats/c9/git')
    expect(gitBaseForWorkspace(REPO_WS)).toBe('crowbar://host-2/v0/chats/chat-1/git')
  })

  it('never returns a path that is still bare once qualified', () => {
    for (const [, call] of REPO_URLS) expect(call().startsWith('crowbar://host-2/v0/')).toBe(true)
  })
})

describe('hosts paired but the host cannot be determined: HostNotResolvedError, never local', () => {
  const everyBuilder = (wsId: string): [string, () => unknown][] => [
    ...BUILDERS.map((b): [string, () => unknown] => [b.name, () => b.call(wsId)]),
    ['chatBase', () => chatBase('chat-1')],
    ['chatTerminalsBase', () => chatTerminalsBaseForWorkspace(wsId, 'chat-1')],
  ]

  it.each(everyBuilder(REPO_WS))('%s while the project list has not loaded', (_n, call) => {
    pair('host-1')
    expect(resolution(call).message).toMatch(/project list is not loaded/)
  })

  it.each(everyBuilder(REPO_WS))(
    '%s when the project row names an unconfigured host',
    (_n, call) => {
      pair('host-1')
      loadProjects(project('p1', 'host-gone'))
      expect(resolution(call).message).toMatch(/its host "host-gone" is not configured/)
    },
  )

  it.each(everyBuilder('ws-unrecorded'))(
    '%s with no scope recorded for the workspace',
    (name, call) => {
      pair('host-1')
      loadProjects(project('p1', 'host-1'))
      // chatBase has no workspace to miss: it is covered by the unrecorded-chat test below.
      if (name === 'chatBase') return
      expect(resolution(call).message).toMatch(/no scope is recorded for the workspace/)
    },
  )

  it('raises the host error before the chat error: a repo workspace with no owning chat yet', () => {
    pair('host-1')
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'no-chat' })
    expect(resolution(() => gitBaseForWorkspace('no-chat')).projectId).toBe('p1')
    loadProjects(project('p1', 'host-1'))
    expect(() => gitBaseForWorkspace('no-chat')).toThrow(OwningChatNotRecordedError)
  })

  it('chatBase for a chat no recorded scope owns is unresolved, not local', () => {
    pair('host-1')
    loadProjects(project('p1', 'host-1'))
    expect(resolution(() => chatBase('unknown-chat')).entity).toBe('chat')
  })

  it('isHomeWorkspace never throws while the host is unresolved (it names no URL)', () => {
    pair('host-1')
    expect(isHomeWorkspace(HOME_WS)).toBe(true)
    expect(isHomeWorkspace(REPO_WS)).toBe(false)
  })

  it('recovers the moment the project row arrives', () => {
    pair('host-1')
    expect(() => gitBaseForWorkspace(REPO_WS)).toThrow(HostNotResolvedError)
    loadProjects(project('p1', 'host-1'))
    expect(gitBaseForWorkspace(REPO_WS)).toBe('crowbar://host-1/v0/chats/chat-1/git')
  })
})

describe('after the last host is revoked (its ids outlive its rows)', () => {
  it('every builder keeps throwing for the stale remote scope, never answering local', () => {
    pair('host-1')
    loadProjects(project('p1', 'host-1'))
    expect(gitBaseForWorkspace(REPO_WS)).toBe('crowbar://host-1/v0/chats/chat-1/git')

    useHostConfigStore.getState().removeHost('host-1')
    loadProjects(project('p-local', 'local')) // a re-read that no longer holds the host's rows

    for (const [, call] of REPO_URLS)
      expect(resolution(call).message).toMatch(/not in the project list/)
    expect(getWorkspaceScope(REPO_WS)).not.toBeNull() // the scope really did outlive the host
  })
})
