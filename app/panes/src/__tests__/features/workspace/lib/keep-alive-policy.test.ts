import { describe, it, expect } from 'vitest'
import { planRetention, workspacesWithViewChat } from '@/features/workspace/lib/keep-alive-policy'
import type { PaneGroup } from '@/features/panes/types/pane'

describe('planRetention', () => {
  it('retains a lone active workspace with no view chat', () => {
    const plan = planRetention([{ wsId: 'a', hasViewChat: false }], 'a')
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
        { wsId: 'active', hasViewChat: false },
        { wsId: 'viewed', hasViewChat: true },
      ],
      'active',
    )
    expect(plan.retain).toEqual(['active', 'viewed'])
    expect(plan.evict).toEqual([])
  })

  it('evicts a non-active workspace the instant it has no view chat', () => {
    const plan = planRetention(
      [
        { wsId: 'active', hasViewChat: false },
        { wsId: 'stale', hasViewChat: false },
      ],
      'active',
    )
    expect(plan.retain).toEqual(['active'])
    expect(plan.evict).toEqual(['stale'])
  })

  it('never evicts the active workspace even with no view chat of its own', () => {
    const plan = planRetention([{ wsId: 'active', hasViewChat: false }], 'active')
    expect(plan.retain).toEqual(['active'])
    expect(plan.evict).toEqual([])
  })

  it('retains the active workspace even when it is not present in entries at all', () => {
    // Defensive: planRetention only reports on entries it was handed; a
    // caller must include the active id in `entries` for it to be reported,
    // but nothing here should crash or misbehave if it's asked about ids
    // that don't include it.
    const plan = planRetention([{ wsId: 'other', hasViewChat: false }], 'active')
    expect(plan.retain).toEqual([])
    expect(plan.evict).toEqual(['other'])
  })

  it('with no active workspace (home route), retains only entries with a view chat', () => {
    const plan = planRetention(
      [
        { wsId: 'a', hasViewChat: true },
        { wsId: 'b', hasViewChat: false },
      ],
      null,
    )
    expect(plan.retain).toEqual(['a'])
    expect(plan.evict).toEqual(['b'])
  })

  it('retains every workspace with a view chat, however many there are', () => {
    const viewed = Array.from({ length: 12 }, (_, i) => ({ wsId: `v${i}`, hasViewChat: true }))
    const plan = planRetention([{ wsId: 'active', hasViewChat: false }, ...viewed], 'active')
    expect(plan.retain).toEqual(['active', ...viewed.map((v) => v.wsId)])
    expect(plan.evict).toEqual([])
  })

  it('preserves input order in retain and evict arrays', () => {
    const plan = planRetention(
      [
        { wsId: 'x', hasViewChat: false }, // no view chat, evicted
        { wsId: 'y', hasViewChat: false }, // active
        { wsId: 'z', hasViewChat: true }, // has a view chat
      ],
      'y',
    )
    expect(plan.retain).toEqual(['y', 'z'])
    expect(plan.evict).toEqual(['x'])
  })
})

describe('workspacesWithViewChat', () => {
  function pane(
    id: string,
    chatId: string | null,
    viewId: string | null = `view-${id}`,
  ): PaneGroup {
    return {
      id,
      type: 'group',
      chatId,
      runnerId: null,
      editorTabIds: [],
      editorOpen: false,
      activeEditorTabId: null,
      viewId,
    }
  }

  it('includes the owner of a chat held by a record, showing or not', () => {
    const owners = workspacesWithViewChat([pane('p1', 'chat-1')], new Map([['chat-1', 'ws-a']]))
    expect(owners).toEqual(new Set(['ws-a']))
  })

  it('excludes a workspace once no record holds its chats', () => {
    expect(workspacesWithViewChat([], new Map([['chat-4', 'ws-d']]))).toEqual(new Set())
  })

  it('ignores chatless panes and panes outside any record', () => {
    const owners = workspacesWithViewChat(
      [pane('stage', null, null), pane('p2', null)],
      new Map([['chat-1', 'ws-a']]),
    )
    expect(owners).toEqual(new Set())
  })

  it('ignores a chat with no known owner (not a registered workspace)', () => {
    expect(workspacesWithViewChat([pane('p1', 'orphan')], new Map())).toEqual(new Set())
  })

  it('unions owners across records and members of a group', () => {
    const owners = workspacesWithViewChat(
      [pane('p1', 'chat-5', 'g'), pane('p2', 'chat-6', 'g'), pane('p3', 'chat-7')],
      new Map([
        ['chat-5', 'ws-x'],
        ['chat-6', 'ws-y'],
        ['chat-7', 'ws-z'],
      ]),
    )
    expect(owners).toEqual(new Set(['ws-x', 'ws-y', 'ws-z']))
  })
})
