import { describe, it, expect } from 'vitest'
import {
  planRetention,
  workspacesWithViewChat,
  RETENTION_CAP,
} from '@/features/workspace/lib/keep-alive-policy'
import type { PaneGroup } from '@/features/panes/types/pane'
import type { RecentsEntry } from '@/features/panes/types/recents-entry'

describe('planRetention', () => {
  it('retains a lone active workspace with no view chat', () => {
    const plan = planRetention([{ wsId: 'a', hasViewChat: false, lastActiveAt: 1000 }], 'a')
    expect(plan.retain).toEqual(['a'])
    expect(plan.evict).toEqual([])
  })

  it('returns empty plan for no entries', () => {
    const plan = planRetention([], 'a')
    expect(plan).toEqual({ retain: [], evict: [] })
  })

  it('retains a non-active workspace that has a view chat', () => {
    const plan = planRetention(
      [
        { wsId: 'active', hasViewChat: false, lastActiveAt: 100 },
        { wsId: 'viewed', hasViewChat: true, lastActiveAt: 50 },
      ],
      'active',
    )
    expect(plan.retain).toEqual(['active', 'viewed'])
    expect(plan.evict).toEqual([])
  })

  it('evicts a non-active workspace the instant it has no view chat', () => {
    const plan = planRetention(
      [
        { wsId: 'active', hasViewChat: false, lastActiveAt: 100 },
        { wsId: 'stale', hasViewChat: false, lastActiveAt: 99 },
      ],
      'active',
    )
    expect(plan.retain).toEqual(['active'])
    expect(plan.evict).toEqual(['stale'])
  })

  it('never evicts the active workspace even with no view chat of its own', () => {
    const plan = planRetention([{ wsId: 'active', hasViewChat: false, lastActiveAt: 1 }], 'active')
    expect(plan.retain).toEqual(['active'])
    expect(plan.evict).toEqual([])
  })

  it('retains the active workspace even when it is not present in entries at all', () => {
    // Defensive: planRetention only reports on entries it was handed; a
    // caller must include the active id in `entries` for it to be reported,
    // but nothing here should crash or misbehave if it's asked about ids
    // that don't include it.
    const plan = planRetention([{ wsId: 'other', hasViewChat: false, lastActiveAt: 1 }], 'active')
    expect(plan.retain).toEqual([])
    expect(plan.evict).toEqual(['other'])
  })

  it('with no active workspace (home route), retains only entries with a view chat', () => {
    const plan = planRetention(
      [
        { wsId: 'a', hasViewChat: true, lastActiveAt: 10 },
        { wsId: 'b', hasViewChat: false, lastActiveAt: 20 },
      ],
      null,
    )
    expect(plan.retain).toEqual(['a'])
    expect(plan.evict).toEqual(['b'])
  })

  it('caps the retained set at the hard cap, evicting the least-recently-active over the cap', () => {
    const active = { wsId: 'active', hasViewChat: false, lastActiveAt: 1000 }
    // 6 more with view chats, all candidates — 7 total, cap is 6.
    const viewed = Array.from({ length: 6 }, (_, i) => ({
      wsId: `v${i}`,
      hasViewChat: true,
      lastActiveAt: i, // v0 oldest ... v5 newest
    }))
    const plan = planRetention([active, ...viewed], 'active', 6)
    expect(plan.retain).toEqual(['active', 'v1', 'v2', 'v3', 'v4', 'v5'])
    expect(plan.evict).toEqual(['v0'])
  })

  it('the active workspace always wins a cap slot regardless of recency', () => {
    const entries = [
      { wsId: 'active', hasViewChat: false, lastActiveAt: 0 }, // oldest timestamp, but active
      ...Array.from({ length: 6 }, (_, i) => ({
        wsId: `v${i}`,
        hasViewChat: true,
        lastActiveAt: 100 + i,
      })),
    ]
    const plan = planRetention(entries, 'active', 6)
    expect(plan.retain).toContain('active')
    expect(plan.retain).toHaveLength(6)
    // The single oldest viewed workspace (v0) is the one pushed out.
    expect(plan.evict).toEqual(['v0'])
  })

  it('preserves input order in retain and evict arrays', () => {
    const plan = planRetention(
      [
        { wsId: 'x', hasViewChat: false, lastActiveAt: 1 }, // no view chat, evicted
        { wsId: 'y', hasViewChat: false, lastActiveAt: 3 }, // active
        { wsId: 'z', hasViewChat: true, lastActiveAt: 2 }, // has a view chat
      ],
      'y',
    )
    expect(plan.retain).toEqual(['y', 'z'])
    expect(plan.evict).toEqual(['x'])
  })

  it('exposes a hard cap of 6', () => {
    expect(RETENTION_CAP).toBe(6)
  })
})

describe('workspacesWithViewChat', () => {
  function pane(id: string, chatId: string | null, viewId?: string): PaneGroup {
    return {
      id,
      type: 'group',
      chatId,
      runnerId: null,
      editorTabIds: [],
      editorOpen: false,
      activeEditorTabId: null,
      viewId: viewId ?? id,
    }
  }

  it('includes the owner of a chat currently held by a live pane', () => {
    const owners = workspacesWithViewChat([pane('p1', 'chat-1')], {}, [], new Map([['chat-1', 'ws-a']]))
    expect(owners).toEqual(new Set(['ws-a']))
  })

  it('includes the owner of a chat in a dormant/parked arrangement, with no live pane', () => {
    const dormant: RecentsEntry[] = [{ id: 'd1', chatIds: ['chat-2'], state: 'dormant' }]
    const owners = workspacesWithViewChat([], {}, dormant, new Map([['chat-2', 'ws-b']]))
    expect(owners).toEqual(new Set(['ws-b']))
  })

  it('includes the owner of a chat that is merely "working" with no pane or dormant record', () => {
    const owners = workspacesWithViewChat([], { 'chat-3': true }, [], new Map([['chat-3', 'ws-c']]))
    expect(owners).toEqual(new Set(['ws-c']))
  })

  it('excludes a workspace once its last chat is gone from every entry', () => {
    // No pane, no dormant record, not working: `chat-4` is in no Recents
    // entry at all, so its owner is not "in a view" any more.
    const owners = workspacesWithViewChat([], {}, [], new Map([['chat-4', 'ws-d']]))
    expect(owners).toEqual(new Set())
  })

  it('ignores a chat with no known owner (not a currently-registered workspace)', () => {
    const dormant: RecentsEntry[] = [{ id: 'd1', chatIds: ['orphan-chat'], state: 'dormant' }]
    const owners = workspacesWithViewChat([], {}, dormant, new Map())
    expect(owners).toEqual(new Set())
  })

  it('unions owners across multiple entries and multiple chats per entry', () => {
    const dormant: RecentsEntry[] = [{ id: 'set1', chatIds: ['chat-5', 'chat-6'], state: 'set' }]
    const owners = workspacesWithViewChat(
      [pane('p1', 'chat-7')],
      {},
      dormant,
      new Map([
        ['chat-5', 'ws-x'],
        ['chat-6', 'ws-y'],
        ['chat-7', 'ws-z'],
      ]),
    )
    expect(owners).toEqual(new Set(['ws-x', 'ws-y', 'ws-z']))
  })
})
