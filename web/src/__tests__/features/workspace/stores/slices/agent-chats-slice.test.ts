import { describe, expect, it, beforeEach, vi } from 'vitest'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { orderedChats } from '@/features/workspace/stores/slices/agent-chats-slice'
import type { AgentChat } from '@/features/agent/api/agent-api'

const chat = (id: string, createdAt: string): AgentChat => ({
  id,
  workspaceId: 'w1',
  title: id,
  activeSegmentId: `${id}-s`,
  activeProviderId: 'claude',
  createdAt,
})

describe('agent-chats-slice', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('upserts (insert then replace by id) and removes', () => {
    const s = createWorkspaceStore('w1')
    s.getState().upsertAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    s.getState().upsertAgentChat(chat('c1', '2026-01-02T00:00:00Z')) // replace
    expect(s.getState().agentChats.chats).toHaveLength(1)
    expect(s.getState().agentChats.chats[0].createdAt).toBe('2026-01-02T00:00:00Z')
    s.getState().removeAgentChat('c1')
    expect(s.getState().agentChats.chats).toHaveLength(0)
  })

  // ── seedAgentChats: the initial-load / WS-reconnect reconcile ──────────────
  // The reseed is the ONLY thing that can repair state the socket missed while it
  // was down, so it must be a full reconcile, not a merge of upserts.

  it('seedAgentChats CLEARS the working map — a turn_stopped dropped during a WS outage must not strand a spinner', () => {
    const s = createWorkspaceStore('w1')
    s.getState().upsertAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    s.getState().setAgentChatWorking('c1', true) // mid-turn when the socket dropped
    expect(s.getState().agentChats.working.c1).toBe(true)

    // Reconnect reseed. Working state is not carried in the seed → it is UNKNOWN,
    // and spec §2 mandates unknown → idle. Without this the row spins forever.
    s.getState().seedAgentChats([chat('c1', '2026-01-01T00:00:00Z')])

    expect(s.getState().agentChats.working.c1).toBeUndefined()
    expect(s.getState().agentChats.chats).toHaveLength(1)
  })

  it('seedAgentChats DROPS chats absent from the response — a delete missed during an outage leaves no ghost row', () => {
    const s = createWorkspaceStore('w1')
    s.getState().upsertAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    s.getState().upsertAgentChat(chat('c2', '2026-01-02T00:00:00Z'))
    s.getState().setAgentChatOrder(['c2', 'c1'])
    s.getState().setActiveAgentChatId('c2')
    s.getState().setAgentChatWorking('c2', true)

    // c2 was deleted while the WS was down: the GET no longer returns it.
    s.getState().seedAgentChats([chat('c1', '2026-01-01T00:00:00Z')])

    expect(s.getState().agentChats.chats.map((c) => c.id)).toEqual(['c1'])
    expect(s.getState().agentChats.order).toEqual(['c1']) // stale order entry pruned
    expect(s.getState().agentChats.activeChatId).toBeNull() // the active chat is gone
    expect(s.getState().agentChats.working.c2).toBeUndefined()
  })

  it('seedAgentChats keeps an active id that still exists, and takes the server copy of each chat', () => {
    const s = createWorkspaceStore('w1')
    s.getState().upsertAgentChat({ ...chat('c1', '2026-01-01T00:00:00Z'), title: 'stale title' })
    s.getState().setActiveAgentChatId('c1')

    s.getState().seedAgentChats([{ ...chat('c1', '2026-01-01T00:00:00Z'), title: 'server title' }])

    expect(s.getState().agentChats.activeChatId).toBe('c1')
    expect(s.getState().agentChats.chats[0].title).toBe('server title')
  })

  it('toggles the working map and stores providers/active id', () => {
    const s = createWorkspaceStore('w1')
    s.getState().setAgentChatWorking('c1', true)
    expect(s.getState().agentChats.working.c1).toBe(true)
    s.getState().setAgentChatWorking('c1', false)
    expect(s.getState().agentChats.working.c1).toBe(false)
    s.getState().setAgentProviders([{ id: 'claude', displayName: 'Claude', icon: '<svg/>' }])
    expect(s.getState().agentChats.providers).toHaveLength(1)
    s.getState().setActiveAgentChatId('c1')
    expect(s.getState().agentChats.activeChatId).toBe('c1')
  })

  it('working map defaults to idle (undefined) for chats never toggled', () => {
    const s = createWorkspaceStore('w1')
    expect(s.getState().agentChats.working['never-touched']).toBeUndefined()
  })

  it('removeAgentChat clears the working entry, order membership, and active id when it matches', () => {
    const s = createWorkspaceStore('w2')
    s.getState().upsertAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    s.getState().setAgentChatWorking('c1', true)
    s.getState().setAgentChatOrder(['c1'])
    s.getState().setActiveAgentChatId('c1')

    s.getState().removeAgentChat('c1')

    expect(s.getState().agentChats.chats).toHaveLength(0)
    expect(s.getState().agentChats.working.c1).toBeUndefined()
    expect(s.getState().agentChats.order).toEqual([])
    expect(s.getState().agentChats.activeChatId).toBeNull()
  })

  it('removeAgentChat leaves an unrelated active id untouched', () => {
    const s = createWorkspaceStore('w3')
    s.getState().upsertAgentChat(chat('c1', '2026-01-01T00:00:00Z'))
    s.getState().upsertAgentChat(chat('c2', '2026-01-02T00:00:00Z'))
    s.getState().setActiveAgentChatId('c2')

    s.getState().removeAgentChat('c1')

    expect(s.getState().agentChats.activeChatId).toBe('c2')
    expect(s.getState().agentChats.chats.map((c) => c.id)).toEqual(['c2'])
  })

  it('setAgentChatOrder updates state and persists per-workspace to localStorage', () => {
    const s = createWorkspaceStore('w4')
    s.getState().setAgentChatOrder(['b', 'a'])
    expect(s.getState().agentChats.order).toEqual(['b', 'a'])
    expect(localStorage.getItem('crowbar:agent-chat-order:w4')).toBe(JSON.stringify(['b', 'a']))
  })

  it('hydrateAgentChatOrder loads a previously persisted order for the workspace (round trip)', () => {
    const writer = createWorkspaceStore('w5')
    writer.getState().setAgentChatOrder(['x', 'y'])

    // A fresh store for the same workspace starts un-hydrated...
    const reader = createWorkspaceStore('w5')
    expect(reader.getState().agentChats.order).toEqual([])
    // ...until explicitly hydrated, at which point it picks up the persisted order.
    reader.getState().hydrateAgentChatOrder()
    expect(reader.getState().agentChats.order).toEqual(['x', 'y'])
  })

  it('hydrateAgentChatOrder defaults to an empty order when nothing was persisted', () => {
    const s = createWorkspaceStore('w6')
    s.getState().hydrateAgentChatOrder()
    expect(s.getState().agentChats.order).toEqual([])
  })

  it('hydrateAgentChatOrder recovers from corrupt persisted JSON', () => {
    localStorage.setItem('crowbar:agent-chat-order:w7', 'not-json{{{')
    const s = createWorkspaceStore('w7')
    s.getState().hydrateAgentChatOrder()
    expect(s.getState().agentChats.order).toEqual([])
  })

  it('setAgentChatOrder is best-effort when localStorage.setItem throws', () => {
    const s = createWorkspaceStore('w8')
    const spy = vi.spyOn(localStorage, 'setItem').mockImplementation(() => {
      throw new Error('quota exceeded')
    })
    expect(() => s.getState().setAgentChatOrder(['a'])).not.toThrow()
    expect(s.getState().agentChats.order).toEqual(['a'])
    spy.mockRestore()
  })

  it('orderedChats: saved order first, unknown chats appended by createdAt', () => {
    const chats = [
      chat('a', '2026-01-03T00:00:00Z'),
      chat('b', '2026-01-01T00:00:00Z'),
      chat('c', '2026-01-02T00:00:00Z'),
    ]
    // order pins b then a; c is absent → appended after, sorted by createdAt.
    expect(orderedChats(chats, ['b', 'a']).map((x) => x.id)).toEqual(['b', 'a', 'c'])
    // empty order → pure createdAt ascending.
    expect(orderedChats(chats, []).map((x) => x.id)).toEqual(['b', 'c', 'a'])
  })

  it('orderedChats ignores order entries that no longer correspond to a chat', () => {
    const chats = [chat('a', '2026-01-01T00:00:00Z')]
    expect(orderedChats(chats, ['ghost', 'a']).map((x) => x.id)).toEqual(['a'])
  })
})
