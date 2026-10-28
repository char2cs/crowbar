import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiFetch = vi.fn()
vi.mock('@/lib/api/fetch', () => ({ apiFetch: (...a: unknown[]) => apiFetch(...a) }))
// Only the scope REGISTRY is faked; the URL builders themselves run for real,
// so these assertions pin the actual paths this client dials rather than a
// stand-in shape. 'w1' carries a real repo, so chatBase builds the Task-17
// repo-scoped shape; 'ws-home' has repoId '' (a project-home workspace) and
// falls back to the still-live /home mount; anything else is unrecorded scope.
vi.mock('@/lib/workspace-scope', () => ({
  bindActiveWorkspaceId: () => {},
  getWorkspaceScope: (id: string) => {
    if (id === 'w1') return { projectId: 'p1', repoId: 'r1', wsId: 'w1' }
    if (id === 'ws-home') return { projectId: 'p1', repoId: '', wsId: 'ws-home' }
    return null
  },
  getOwningChatId: (id: string) => (id === 'w1' ? 'chat-w1' : null),
}))

import * as api from '@/features/agent/api/agent-chat-creates'

describe('agent-chat-creates', () => {
  beforeEach(() => apiFetch.mockReset())

  it('createChat POSTs the provider and returns the new id', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    const id = await api.createChat('w1', 'codex')
    expect(id).toBe('c9')
    const [url, init] = apiFetch.mock.calls[0]
    expect(url).toBe('/v0/projects/p1/repos/r1/chats')
    // No parent given is the workspace root, said explicitly rather than by
    // omission — one shape on the wire whether or not the chat is a thread.
    expect(init).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ provider: 'codex', parentId: '', workspaceId: 'w1' }),
    })
  })

  // Regression: the repo-scoped mount (Task 17) binds no :wsId, so the ONLY
  // way the backend's Create learns which workspace to anchor a top-level
  // chat to is body.workspaceId. Omitting it minted the chat fine but left it
  // anchored to "" — a chat with no ancestor to resolve a cwd workspace from —
  // and its runner spawn 404'd (confirmed live: "agentchat: not found").
  it('createChat sends workspaceId in the body, not just the URL', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChat('w1', 'codex')
    const body = JSON.parse((apiFetch.mock.calls[0][1] as RequestInit).body as string)
    expect(body.workspaceId).toBe('w1')
  })

  it('createChat carries the PARENT, so the edge exists before the runner starts', async () => {
    // Placement rides on the create. It used to be a second call, which meant
    // the thread's first CLI was spawned before its lineage existed — the very
    // turn the user asked the thread for ran with the agent as a stranger.
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChat('w1', 'codex', 'parent-chat')
    expect(apiFetch.mock.calls[0][1]).toMatchObject({
      body: JSON.stringify({ provider: 'codex', parentId: 'parent-chat', workspaceId: 'w1' }),
    })
    // ONE call: no follow-up placement to leave an orphan at the root if it
    // failed.
    expect(apiFetch).toHaveBeenCalledTimes(1)
  })

  // THE BUG: a chat "started on the CLI" was only ever a client-side landing
  // seed. The daemon spawned it on the provider's own default face, which for
  // a mixed-transport provider means an api connection — and a chat with one
  // of those has its PTY hidden outright, so the terminal surface it landed on
  // showed "This agent has no terminal view attached right now". The surface
  // has to travel on the CREATE, because it decides what gets forked.
  it('createChat carries the SURFACE the chat is born on', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChat('w1', 'codex', '', 'terminal')
    const body = JSON.parse((apiFetch.mock.calls[0][1] as RequestInit).body as string)
    expect(body.surface).toBe('terminal')
  })

  // Omitted means the provider's own default face — and the key is ABSENT,
  // not "", so the wire shape for an ordinary create is byte-identical to
  // what it was before this argument existed.
  it('createChat omits surface entirely when none is asked for', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChat('w1', 'codex')
    const body = JSON.parse((apiFetch.mock.calls[0][1] as RequestInit).body as string)
    expect('surface' in body).toBe(false)
  })

  // Task 8: the sidebar's "create workspace" affordance — no workspace exists
  // yet to derive a chatBase scope from, so this is built straight off
  // project+repo instead.
  it('createChatWithOwnWorktree POSTs ownWorktree:true with no workspaceId, and returns the new id', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    const id = await api.createChatWithOwnWorktree('p1', 'r1', 'codex', 'home-1')
    expect(id).toBe('c9')
    const [url, init] = apiFetch.mock.calls[0]
    expect(url).toBe('/v0/projects/p1/repos/r1/chats')
    expect(init).toMatchObject({
      method: 'POST',
      body: JSON.stringify({ provider: 'codex', parentId: 'home-1', ownWorktree: true }),
    })
    const body = JSON.parse((init as RequestInit).body as string)
    expect(body).not.toHaveProperty('workspaceId')
  })

  it('createChatWithOwnWorktree defaults parentId to the panel root', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChatWithOwnWorktree('p1', 'r1', 'codex')
    expect(apiFetch.mock.calls[0][1]).toMatchObject({
      body: JSON.stringify({ provider: 'codex', parentId: '', ownWorktree: true }),
    })
  })

  // TestRegression: the sidebar's inline create input (2026-09-09) types a
  // branch name before the fork happens — it has to reach the wire, not be
  // silently dropped in favour of the server-generated name every caller
  // before this got.
  it('createChatWithOwnWorktree sends the caller-supplied branch name', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChatWithOwnWorktree('p1', 'r1', 'codex', 'home-1', 'feature/typed-name')
    const body = JSON.parse((apiFetch.mock.calls[0][1] as RequestInit).body as string)
    expect(body).toMatchObject({ branch: 'feature/typed-name' })
  })

  it('createChatWithOwnWorktree omits branch entirely when not given', async () => {
    apiFetch.mockResolvedValue({ id: 'c9' })
    await api.createChatWithOwnWorktree('p1', 'r1', 'codex', 'home-1')
    const body = JSON.parse((apiFetch.mock.calls[0][1] as RequestInit).body as string)
    expect(body).not.toHaveProperty('branch')
  })
})
