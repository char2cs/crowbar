/**
 * The Chats tree's own half of drag and drop: which drags a row may accept
 * (`chatAllowedModes`), how wide its reorder band is (`chatEdgeBandFor`), the
 * attribute table its rows publish (`CHAT_ROW_SPEC`, `chatRowProps` /
 * `readChatRow` / `chatRowElementFor` / `findChatDrop`), and where a drop
 * actually lands once the pointer has resolved to a row and a mode
 * (`chatDropPlan`).
 *
 * The shared band arithmetic and hit-testing plumbing already have their own
 * tests in `tree-dnd/drop-core.test.ts` and `tree-dnd/drop-dom.test.ts` —
 * this file only exercises what THIS tree contributes: its own refusal rule,
 * its own attribute names, and its own reading of "where does this land".
 * jsdom harness technique (stub `elementsFromPoint` / `getBoundingClientRect`,
 * spread `chatRowProps` onto real elements) is copied from those two files
 * rather than reinvented.
 */
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ALL_MODES,
  EDGE_BAND_CONTAINER,
  EDGE_BAND_HEAVY,
  NO_MODES,
} from '@/components/tree-dnd/drop-core'
import { PANE_DROP_ATTR } from '@/components/layout/drop-target-dom'
import {
  CHAT_DROP_POLICY,
  CHAT_ROW_SPEC,
  chatAllowedModes,
  chatDropPlan,
  chatEdgeBandFor,
  chatRowElementFor,
  chatRowProps,
  findChatDrop,
  readChatRow,
  type ChatDragSubject,
  type ChatDropRow,
  type ChatMove,
  type ResolvedChatDrop,
} from '@/features/agent/tree/lib/chat-drop'

const subject = (id: string, over: Partial<ChatDragSubject> = {}): ChatDragSubject => ({
  kind: 'chat',
  id,
  ...over,
})

const row = (id: string, over: Partial<ChatDropRow> = {}): ChatDropRow => ({
  kind: 'chat',
  id,
  parentId: '',
  ...over,
})

const resolved = (over: Partial<ResolvedChatDrop> & { id: string }): ResolvedChatDrop => ({
  kind: 'chat',
  parentId: '',
  mode: 'before',
  ...over,
})

describe('chatAllowedModes', () => {
  it('refuses everything when nothing is being dragged', () => {
    expect(chatAllowedModes([], row('t1'))).toEqual(NO_MODES)
  })

  it('refuses dropping a row onto itself', () => {
    expect(chatAllowedModes([subject('a')], row('a'))).toEqual(NO_MODES)
  })

  it('refuses dropping a row into its own descendant', () => {
    // 'a' is dragged; the target's published ancestor chain runs through it.
    const target = row('grandchild', { path: '/root/a/' })
    expect(chatAllowedModes([subject('a')], target)).toEqual(NO_MODES)
  })

  it('does not refuse on a false substring match — "/ab/" is not an ancestry hit for "a"', () => {
    // The whole reason `path` is delimited on both sides: a naive
    // `path.includes(s.id)` would wrongly refuse this, mistaking a sibling
    // folder named "ab" for an ancestor named "a".
    const target = row('t1', { path: '/ab/' })
    expect(chatAllowedModes([subject('a')], target)).toEqual(ALL_MODES)
  })

  it('treats a missing path as the empty chain, refusing nothing extra', () => {
    const target = row('t1')
    expect(target.path).toBeUndefined()
    expect(chatAllowedModes([subject('a')], target)).toEqual(ALL_MODES)
  })

  it('checks every dragged row, not just the first', () => {
    const target = row('grandchild', { path: '/root/b/' })
    // 'a' alone would be fine; 'b' among the dragged rows is the target's
    // own ancestor, and one refusal in the set refuses the whole drag.
    expect(chatAllowedModes([subject('a'), subject('b')], target)).toEqual(NO_MODES)
  })

  it('allows a chat into a folder, a folder into a chat, and everything else — the only rule is structural', () => {
    expect(
      chatAllowedModes(
        [subject('chat1', { kind: 'chat' })],
        row('folder1', { kind: 'chatFolder' }),
      ),
    ).toEqual(ALL_MODES)
    expect(
      chatAllowedModes(
        [subject('folder1', { kind: 'chatFolder' })],
        row('chat1', { kind: 'chat' }),
      ),
    ).toEqual(ALL_MODES)
  })
})

describe('chatEdgeBandFor', () => {
  it('gives a folder the narrow, container band — the cheap, common move', () => {
    expect(chatEdgeBandFor('chatFolder')).toBe(EDGE_BAND_CONTAINER)
  })

  it('gives a chat the wide, heavy band — nesting rewrites what the thread sees', () => {
    expect(chatEdgeBandFor('chat')).toBe(EDGE_BAND_HEAVY)
  })
})

describe('CHAT_DROP_POLICY', () => {
  it('wires the matrix and the band function straight through, unmodified', () => {
    expect(CHAT_DROP_POLICY.allowedModes).toBe(chatAllowedModes)
    expect(CHAT_DROP_POLICY.edgeBandFor).toBe(chatEdgeBandFor)
  })
})

describe('CHAT_ROW_SPEC', () => {
  it('names the attributes this tree owns — its identity on the page', () => {
    // Pinned exactly: these are what the sidebar's own tree must never
    // publish, and what a chat row must never stop publishing.
    expect(CHAT_ROW_SPEC).toEqual({
      kinds: { chat: 'data-chat-drop', chatFolder: 'data-chat-folder-drop' },
      parentAttr: 'data-chat-parent',
      strings: { path: 'data-chat-path' },
      flags: { expanded: 'data-chat-expanded', hasChildren: 'data-chat-children' },
    })
  })

  it('publishes no zone attribute of its own — removal is the sidebar’s pane', () => {
    // A row of this tree carries none of the pane's attribute and the pane
    // carries none of the rows', which is what lets one hit test read both.
    expect(Object.values(CHAT_ROW_SPEC.kinds)).not.toContain(PANE_DROP_ATTR)
  })
})

describe('chatRowProps / readChatRow', () => {
  const mount = (props: Record<string, string | undefined>): HTMLElement => {
    const el = document.createElement('div')
    for (const [k, v] of Object.entries(props)) if (v !== undefined) el.setAttribute(k, v)
    document.body.appendChild(el)
    return el
  }

  afterEach(() => {
    document.body.innerHTML = ''
  })

  it('round-trips every field a chat row carries', () => {
    const r: ChatDropRow = {
      kind: 'chat',
      id: 'c1',
      parentId: 'f1',
      expanded: true,
      hasChildren: true,
      path: '/f1/',
    }
    expect(readChatRow(mount(chatRowProps(r)))).toEqual(r)
  })

  it('round-trips a folder row through its own attribute', () => {
    const r: ChatDropRow = { kind: 'chatFolder', id: 'f1', parentId: '' }
    expect(readChatRow(mount(chatRowProps(r)))).toMatchObject({ kind: 'chatFolder', id: 'f1' })
  })

  it('publishes expanded/hasChildren by presence, not by "true"/"false"', () => {
    const props = chatRowProps(row('c1'))
    expect(props['data-chat-expanded']).toBeUndefined()
    expect(props['data-chat-children']).toBeUndefined()
    expect(props['data-chat-path']).toBeUndefined()
  })

  it('is not a chat row when it carries none of these attributes', () => {
    expect(readChatRow(document.createElement('div'))).toBeNull()
  })

  it('finds the live element for a subject, scoped to a root when given one', () => {
    const scope = document.createElement('div')
    document.body.appendChild(scope)
    const inside = mount(chatRowProps(row('c1')))
    scope.appendChild(inside)
    const outside = mount(chatRowProps(row('c1')))

    expect(chatRowElementFor({ kind: 'chat', id: 'c1' }, scope)).toBe(inside)
    // Unscoped (default root = document) finds whichever comes first in the document.
    expect(chatRowElementFor({ kind: 'chat', id: 'c1' })).toBe(inside)
    expect(outside).not.toBeNull()
  })

  it('finds nothing for an id this tree never published', () => {
    expect(chatRowElementFor({ kind: 'chat', id: 'ghost' })).toBeNull()
  })
})

describe('findChatDrop', () => {
  const ROW_TOP = 100
  const ROW_HEIGHT = 40
  const at = (ratio: number) => ROW_TOP + ROW_HEIGHT * ratio

  function mountRow(r: ChatDropRow, top = ROW_TOP): HTMLElement {
    const el = document.createElement('div')
    for (const [k, v] of Object.entries(chatRowProps(r))) if (v !== undefined) el.setAttribute(k, v)
    el.getBoundingClientRect = () =>
      ({ top, bottom: top + ROW_HEIGHT, height: ROW_HEIGHT, left: 0, width: 200 }) as DOMRect
    document.body.appendChild(el)
    return el
  }

  function mountPane(): HTMLElement {
    const el = document.createElement('div')
    el.setAttribute(PANE_DROP_ATTR, '')
    document.body.appendChild(el)
    return el
  }

  function stackAt(...els: Element[]): void {
    document.elementsFromPoint = vi.fn(() => els) as typeof document.elementsFromPoint
  }

  afterEach(() => {
    document.body.innerHTML = ''
    vi.restoreAllMocks()
  })

  it('resolves before/into/after using this tree’s own band for the row kind', () => {
    stackAt(mountRow(row('folder1', { kind: 'chatFolder' })))
    const dragged = [subject('dragged')]

    // A folder uses the narrow (0.2) container band.
    expect(findChatDrop(0, at(0.1), dragged)).toMatchObject({ row: { mode: 'before' } })
    expect(findChatDrop(0, at(0.5), dragged)).toMatchObject({ row: { mode: 'into' } })
    expect(findChatDrop(0, at(0.9), dragged)).toMatchObject({ row: { mode: 'after' } })
  })

  it('uses the wider band for a chat row than for a folder row', () => {
    stackAt(mountRow(row('chat1')))
    // 0.25 is inside a chat's 0.3 band (reorders) but outside a folder's 0.2
    // band (would nest) — this is what makes the wide/narrow split visible.
    expect(findChatDrop(0, at(0.25), [subject('dragged')])).toMatchObject({
      row: { mode: 'before' },
    })
  })

  it('refuses a drop onto the dragged row’s own descendant, via its published path', () => {
    stackAt(mountRow(row('kid', { path: '/parent/' })))
    expect(findChatDrop(0, at(0.5), [subject('parent')])).toBeNull()
  })

  it('hands back the measured rect alongside the resolved row', () => {
    stackAt(mountRow(row('chat1')))
    expect(findChatDrop(0, at(0.5), [subject('dragged')])).toMatchObject({
      kind: 'row',
      rect: { top: ROW_TOP, bottom: ROW_TOP + ROW_HEIGHT, left: 0, width: 200 },
    })
  })

  it('the editor pane is hit before any row behind it, for any cargo', () => {
    stackAt(mountPane(), mountRow(row('chat1')))
    // The zone's own `hit` never refuses — every row this tree carries may
    // leave, so a pointer over the pane never falls through to the rows behind
    // it. That is this tree's rule, not the shared hit test's.
    expect(
      findChatDrop(0, at(0.5), [subject('c1'), subject('f1', { kind: 'chatFolder' })]),
    ).toEqual({
      kind: 'pane',
    })
  })

  it('resolves nothing when the pointer is over no row and no zone', () => {
    stackAt(document.createElement('div'))
    expect(findChatDrop(0, at(0.5), [subject('dragged')])).toBeNull()
  })
})

describe('chatDropPlan', () => {
  const siblingsOf = (
    entries: Record<string, readonly string[]>,
  ): ReadonlyMap<string, readonly string[]> => new Map(Object.entries(entries))

  it('is a no-op when nothing is being dragged', () => {
    expect(chatDropPlan([], resolved({ id: 't1' }), siblingsOf({}))).toEqual({
      calls: [],
      writes: [],
    })
  })

  it('into appends at the end of the target’s existing children', () => {
    const plan = chatDropPlan(
      [subject('new')],
      resolved({ id: 'folder1', mode: 'into' }),
      siblingsOf({ folder1: ['a', 'b'] }),
    )
    expect(plan.calls).toEqual([{ id: 'new', parentId: 'folder1', order: 2 }])
    expect(plan.writes.map((w) => w.id)).toEqual(['a', 'b', 'new'])
  })

  it('into an empty (unlisted) container starts at index 0', () => {
    // No entry at all for 'empty' — not even an empty array — the `?? []`
    // fallback, exercised by a container nobody has ever put a child under.
    const plan = chatDropPlan(
      [subject('new')],
      resolved({ id: 'empty', mode: 'into' }),
      siblingsOf({}),
    )
    expect(plan.calls).toEqual([{ id: 'new', parentId: 'empty', order: 0 }])
  })

  it('before/after an ordinary row reorders among the real siblings, by the anchor’s index', () => {
    const siblings = siblingsOf({ '': ['a', 'b', 'c'] })
    const before = chatDropPlan([subject('c')], resolved({ id: 'a', mode: 'before' }), siblings)
    expect(before.writes.map((w) => w.id)).toEqual(['c', 'a', 'b'])

    const after = chatDropPlan([subject('a')], resolved({ id: 'b', mode: 'after' }), siblings)
    expect(after.writes.map((w) => w.id)).toEqual(['b', 'a', 'c'])
  })

  it('after an EXPANDED row that has children is a re-parent into its first-child slot', () => {
    // The gap under an expanded parent is drawn as its first child's slot,
    // not as "after the whole subtree" — so this must land INSIDE, at index 0,
    // among the target's own children, not among its siblings.
    const plan = chatDropPlan(
      [subject('new')],
      resolved({ id: 'folder1', mode: 'after', expanded: true, hasChildren: true }),
      siblingsOf({ '': ['folder1', 'other'], folder1: ['existingKid'] }),
    )
    expect(plan.calls).toEqual([{ id: 'new', parentId: 'folder1', order: 0 }])
    expect(plan.writes.map((w) => w.id)).toEqual(['new', 'existingKid'])
  })

  it('after a COLLAPSED row with children reorders among its siblings instead — the gap is its own', () => {
    const plan = chatDropPlan(
      [subject('new')],
      resolved({ id: 'folder1', mode: 'after', expanded: false, hasChildren: true }),
      siblingsOf({ '': ['folder1', 'other'] }),
    )
    expect(plan.calls).toEqual([{ id: 'new', parentId: '', order: 1 }])
  })

  it('after an expanded row with NO children reorders among its siblings — nothing to be first child of', () => {
    const plan = chatDropPlan(
      [subject('new')],
      resolved({ id: 'folder1', mode: 'after', expanded: true, hasChildren: false }),
      siblingsOf({ '': ['folder1', 'other'] }),
    )
    expect(plan.calls).toEqual([{ id: 'new', parentId: '', order: 1 }])
  })

  it('before an expanded row with children still names its own slot, not the first-child one', () => {
    // resolvesToFirstChild only ever applies to 'after' — 'before' never
    // reinterprets itself as "inside".
    const plan = chatDropPlan(
      [subject('new')],
      resolved({ id: 'folder1', mode: 'before', expanded: true, hasChildren: true }),
      siblingsOf({ '': ['folder1', 'other'], folder1: ['existingKid'] }),
    )
    expect(plan.calls).toEqual([{ id: 'new', parentId: '', order: 0 }])
  })

  it('falls back to the tree root when the target has no parentId of its own', () => {
    const plan = chatDropPlan(
      [subject('new')],
      resolved({ id: 'a', mode: 'before', parentId: undefined }),
      siblingsOf({ '': ['a'] }),
    )
    expect(plan.calls).toEqual([{ id: 'new', parentId: '', order: 0 }])
  })

  it('a stale anchor — absent from the level — lands the drop at the end rather than swallowing it', () => {
    // The row the pointer was over moved out from under it before the drop
    // committed. Landing at the end beats losing the drag.
    const plan = chatDropPlan(
      [subject('new')],
      resolved({ id: 'gone', mode: 'before', parentId: '' }),
      siblingsOf({ '': ['a', 'b'] }),
    )
    expect(plan.calls).toEqual([{ id: 'new', parentId: '', order: 2 }])
  })

  it('a drop that changes nothing sends neither calls nor writes', () => {
    // 'b' dropped 'after' 'a', when 'b' is already right after 'a': lifting
    // 'b' out and reinserting it after 'a' reads back identical to what was
    // already there — the commonest accident in a list.
    const plan = chatDropPlan(
      [subject('b')],
      resolved({ id: 'a', mode: 'after', parentId: '' }),
      siblingsOf({ '': ['a', 'b'] }),
    )
    expect(plan).toEqual({ calls: [], writes: [] })
  })

  it('moving several rows at once keeps them in the order given, in sequential slots', () => {
    const plan = chatDropPlan(
      [subject('b'), subject('c')],
      resolved({ id: 'a', mode: 'before', parentId: '' }),
      siblingsOf({ '': ['a', 'b', 'c'] }),
    )
    expect(plan.calls).toEqual([
      { id: 'b', parentId: '', order: 0 },
      { id: 'c', parentId: '', order: 1 },
    ] satisfies ChatMove[])
    // The whole destination level, moved rows and displaced siblings alike.
    expect(plan.writes.map((w) => w.id)).toEqual(['b', 'c', 'a'])
    expect(plan.writes.map((w) => w.order)).toEqual([0, 1, 2])
  })

  it('lifts the moving rows out of the count before resolving the anchor’s index', () => {
    // Dragging 'a' to land after 'b': naive indexOf('b') in the ORIGINAL list
    // is 1, but with 'a' lifted out first the real slot after 'b' is index 1
    // of the remaining ['b','c'] — same number here by coincidence, so this
    // also pins the not-off-by-one case explicitly via the written order.
    const plan = chatDropPlan(
      [subject('a')],
      resolved({ id: 'b', mode: 'after', parentId: '' }),
      siblingsOf({ '': ['a', 'b', 'c'] }),
    )
    expect(plan.writes.map((w) => w.id)).toEqual(['b', 'a', 'c'])
  })
})
