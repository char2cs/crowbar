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

/**
 * A Recents row is a VIEW, never a pane. Grouping used to be recorded twice —
 * once here as `dormantArrangements` entries a merge had to write by hand,
 * once (not at all) in the pane layout — and the two could disagree about
 * what was one view. `viewId` on the panes is the single fact now.
 */
describe('deriveRecentsEntries — one row per VIEW', () => {
  it('panes sharing a view are ONE row carrying every chat in it', () => {
    const panes = [
      makePane({ chatId: 'chat-1', viewId: 'view-a' }),
      makePane({ chatId: 'chat-2', viewId: 'view-a' }),
    ]

    const entries = deriveRecentsEntries(panes, {}, [])

    expect(entries).toEqual([{ id: 'view-a', chatIds: ['chat-1', 'chat-2'], state: 'live' }])
  })

  it('panes in different views are separate rows', () => {
    const panes = [
      makePane({ chatId: 'chat-1', viewId: 'view-a' }),
      makePane({ chatId: 'chat-2', viewId: 'view-b' }),
    ]

    const entries = deriveRecentsEntries(panes, {}, [])

    expect(entries.map((e) => e.chatIds)).toEqual([['chat-1'], ['chat-2']])
  })

  // The grouped row is addressed by the VIEW, so `recentsOrder` (keyed by
  // entry id) survives a merge instead of the row jumping slots.
  it('keeps the view id as the row id, so a merge never moves the row', () => {
    const before = deriveRecentsEntries([makePane({ chatId: 'chat-1', viewId: 'view-a' })], {}, [])
    const after = deriveRecentsEntries(
      [
        makePane({ chatId: 'chat-1', viewId: 'view-a' }),
        makePane({ chatId: 'chat-2', viewId: 'view-a' }),
      ],
      {},
      [],
    )

    expect(after[0].id).toBe(before[0].id)
  })

  // An untagged pane is its own view (a layout persisted before views
  // existed), so nothing groups by accident.
  it('never groups untagged panes together', () => {
    const panes = [makePane({ chatId: 'chat-1' }), makePane({ chatId: 'chat-2' })]

    const entries = deriveRecentsEntries(panes, {}, [])

    expect(entries.map((e) => e.chatIds)).toEqual([['chat-1'], ['chat-2']])
  })

  // Zen's rule, and the reason no code has to notice it: a group of one and
  // an ungrouped pane are the same thing.
  it('a view down to one pane is an ordinary single-chat row', () => {
    const entries = deriveRecentsEntries([makePane({ chatId: 'chat-1', viewId: 'view-a' })], {}, [])

    expect(entries).toEqual([{ id: 'view-a', chatIds: ['chat-1'], state: 'live' }])
  })

  it('a dormant record still claims its chat ahead of the live view loop', () => {
    const panes = [makePane({ chatId: 'chat-2', viewId: 'view-a' })]
    const dormant: RecentsEntry[] = [{ id: 'slot', chatIds: ['chat-1'], state: 'dormant' }]

    const entries = deriveRecentsEntries(panes, {}, dormant)

    expect(entries.map((e) => [e.id, e.chatIds])).toEqual([
      ['slot', ['chat-1']],
      ['view-a', ['chat-2']],
    ])
  })

  // Measured live: merging into a chat that had been closed once split the
  // view back across two rows — the reopened chat at its old slot, plus a
  // second row for whatever joined it. The dormant record says WHERE the row
  // is drawn; the panes say what is IN it.
  it('a reopened chat brings its whole view to its own remembered slot', () => {
    const panes = [
      makePane({ chatId: 'chat-1', viewId: 'view-a' }),
      makePane({ chatId: 'chat-2', viewId: 'view-a' }),
    ]
    const dormant: RecentsEntry[] = [{ id: 'slot', chatIds: ['chat-1'], state: 'dormant' }]

    const entries = deriveRecentsEntries(panes, {}, dormant)

    expect(entries).toEqual([{ id: 'slot', chatIds: ['chat-1', 'chat-2'], state: 'live' }])
  })

  // The absorbed member must not then be re-emitted by the live-view pass,
  // and a second record that only named it has nothing left to draw.
  it('never draws an absorbed member twice, from either source', () => {
    const panes = [
      makePane({ chatId: 'chat-1', viewId: 'view-a' }),
      makePane({ chatId: 'chat-2', viewId: 'view-a' }),
    ]
    const dormant: RecentsEntry[] = [
      { id: 'slot-1', chatIds: ['chat-1'], state: 'dormant' },
      { id: 'slot-2', chatIds: ['chat-2'], state: 'dormant' },
    ]

    const entries = deriveRecentsEntries(panes, {}, dormant)

    expect(entries).toEqual([{ id: 'slot-1', chatIds: ['chat-1', 'chat-2'], state: 'live' }])
  })

  // A DORMANT record naming a chat nothing holds pulls nothing in — there is
  // no view to bring.
  it('absorbs nothing for a chat that is not live', () => {
    const dormant: RecentsEntry[] = [{ id: 'slot', chatIds: ['chat-1'], state: 'dormant' }]

    const entries = deriveRecentsEntries([], {}, dormant)

    expect(entries).toEqual([{ id: 'slot', chatIds: ['chat-1'], state: 'dormant' }])
  })
})

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

  // Spec §8.1: "above / below a Recents entry → it moves to that slot" — the
  // fourth argument, `pane-slice.ts`'s persisted `recentsOrder`.
  describe('the persisted `order` argument', () => {
    it('a real drag order overrides the append/population order entirely', () => {
      const dormant: RecentsEntry[] = [
        { id: 'view-a', chatIds: ['chat-1'], state: 'dormant' },
        { id: 'view-b', chatIds: ['chat-2'], state: 'dormant' },
      ]
      const panes = [makePane({ id: 'view-c', chatId: 'chat-3' })]
      // Natural/append order would be [view-a, view-b, view-c] (as the test
      // above pins) — dragged order says otherwise.
      const entries = deriveRecentsEntries(panes, {}, dormant, ['view-c', 'view-a', 'view-b'])
      expect(entries.map((e) => e.id)).toEqual(['view-c', 'view-a', 'view-b'])
    })

    it('an entry never named in `order` keeps its natural position, after every named one', () => {
      const dormant: RecentsEntry[] = [
        { id: 'view-a', chatIds: ['chat-1'], state: 'dormant' },
        { id: 'view-b', chatIds: ['chat-2'], state: 'dormant' },
        { id: 'view-c', chatIds: ['chat-3'], state: 'dormant' },
      ]
      // Only view-b has ever been dragged — view-a and view-c are untouched
      // and keep their own natural relative order, both after view-b.
      const entries = deriveRecentsEntries([], {}, dormant, ['view-b'])
      expect(entries.map((e) => e.id)).toEqual(['view-b', 'view-a', 'view-c'])
    })

    it('an id in `order` that names nothing currently drawn is simply inert', () => {
      const dormant: RecentsEntry[] = [{ id: 'view-a', chatIds: ['chat-1'], state: 'dormant' }]
      const entries = deriveRecentsEntries([], {}, dormant, ['ghost', 'view-a'])
      expect(entries.map((e) => e.id)).toEqual(['view-a'])
    })

    it('an empty order (the default) falls back to the plain append order untouched', () => {
      const dormant: RecentsEntry[] = [
        { id: 'view-a', chatIds: ['chat-1'], state: 'dormant' },
        { id: 'view-b', chatIds: ['chat-2'], state: 'dormant' },
      ]
      expect(deriveRecentsEntries([], {}, dormant)).toEqual(
        deriveRecentsEntries([], {}, dormant, []),
      )
    })
  })
})
