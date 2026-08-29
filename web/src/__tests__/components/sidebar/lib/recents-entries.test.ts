import { describe, expect, it } from 'vitest'
import { deriveRecentsEntries } from '@/components/sidebar/lib/recents-entries'
import type { PaneGroup } from '@/features/panes/types/pane'
import type { RecentsEntry } from '@/features/panes/types/recents-entry'

let paneCounter = 0

function makePane(overrides: Partial<PaneGroup> = {}): PaneGroup {
  paneCounter += 1
  return {
    id: `pane-${paneCounter}`,
    type: 'group',
    chatId: null,
    runnerId: null,
    editorTabIds: [],
    activeEditorTabId: null,
    editorOpen: false,
    ...overrides,
  }
}

describe('deriveRecentsEntries', () => {
  it('a chat appears once, in the highest band that claims it', () => {
    const panes = [makePane({ chatId: 'chat-1' })]
    const working = { 'chat-1': true, 'chat-2': true }
    const dormant: RecentsEntry[] = []
    const entries = deriveRecentsEntries(panes, working, dormant)
    const chat1Entries = entries.filter((e) => e.chatIds.includes('chat-1'))
    expect(chat1Entries).toHaveLength(1)
    expect(chat1Entries[0].state).toBe('live')
  })

  it('closing a working view keeps it in the band as "working", not dormant', () => {
    const panes: PaneGroup[] = [] // view closed
    const working = { 'chat-1': true }
    const dormant: RecentsEntry[] = []
    const entries = deriveRecentsEntries(panes, working, dormant)
    expect(entries.find((e) => e.chatIds.includes('chat-1'))?.state).toBe('working')
  })

  it('an arrangement keeps its slot as it gains a pane', () => {
    const dormant: RecentsEntry[] = [{ id: 'view-a', chatIds: ['chat-1'], state: 'dormant' }]
    const panes = [makePane({ id: 'view-a', chatId: 'chat-1' })]
    const entries = deriveRecentsEntries(panes, {}, dormant)
    const idx = entries.findIndex((e) => e.id === 'view-a')
    expect(idx).toBe(0) // same slot dormant held, not re-sorted to the end
  })

  it('a chat with no pane and no working flag does not appear', () => {
    const entries = deriveRecentsEntries([], {}, [])
    expect(entries).toHaveLength(0)
  })

  it('a dormant view with no live pane and no working flag stays dormant', () => {
    const dormant: RecentsEntry[] = [{ id: 'view-a', chatIds: ['chat-1'], state: 'dormant' }]
    const entries = deriveRecentsEntries([], {}, dormant)
    expect(entries).toEqual([{ id: 'view-a', chatIds: ['chat-1'], state: 'dormant' }])
  })

  it('a dormant 2+ chat arrangement is drawn as a set at rest', () => {
    const dormant: RecentsEntry[] = [
      { id: 'view-a', chatIds: ['chat-1', 'chat-2'], state: 'dormant' },
    ]
    const entries = deriveRecentsEntries([], {}, dormant)
    expect(entries[0].state).toBe('set')
  })

  it("order is the user's: earlier dormant slots lead, new views append after", () => {
    const dormant: RecentsEntry[] = [
      { id: 'view-a', chatIds: ['chat-1'], state: 'dormant' },
      { id: 'view-b', chatIds: ['chat-2'], state: 'dormant' },
    ]
    const panes = [makePane({ id: 'view-c', chatId: 'chat-3' })]
    const entries = deriveRecentsEntries(panes, {}, dormant)
    expect(entries.map((e) => e.id)).toEqual(['view-a', 'view-b', 'view-c'])
  })

  it('restoring a dormant row (closing the live pane again) does not move it or duplicate it', () => {
    const dormant: RecentsEntry[] = [
      { id: 'view-a', chatIds: ['chat-1'], state: 'dormant' },
      { id: 'view-b', chatIds: ['chat-2'], state: 'dormant' },
    ]
    // view-a is live again; view-b stays dormant. Order must not reshuffle.
    const panes = [makePane({ id: 'view-a', chatId: 'chat-1' })]
    const entries = deriveRecentsEntries(panes, {}, dormant)
    expect(entries.map((e) => ({ id: e.id, state: e.state }))).toEqual([
      { id: 'view-a', state: 'live' },
      { id: 'view-b', state: 'dormant' },
    ])
  })
})
