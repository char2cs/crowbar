/**
 * The registry: a tree publishes its own row attributes and gets the same hit
 * test everyone else uses.
 *
 * Two invented trees are wired up here — 'notes' and 'cards' — and neither is
 * the sidebar. That is the whole point: the sidebar's own hit test is covered
 * against its real attributes in `sidebar/hooks/use-sidebar-drag.test.ts`, so
 * what is left to prove is that a tree the core has never heard of gets
 * identical behaviour by handing in a table, and that two such trees on one
 * page cannot see each other's rows.
 *
 * jsdom has neither `elementsFromPoint` nor real layout, so both are stubbed.
 * The geometry IS the input, so stating it explicitly is what makes the band
 * arithmetic assertable at all.
 */
import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  ALL_MODES,
  EDGE_BAND_CONTAINER,
  EDGE_BAND_HEAVY,
  NO_MODES,
  REORDER_MODES,
  type AllowedModes,
  type DragSubjectBase,
  type DropPolicy,
  type DropTargetBase,
} from '@/components/tree-dnd/drop-core'
import {
  createDropHitTest,
  createDropRowDom,
  type DropRowSpec,
  type DropZone,
} from '@/components/tree-dnd/drop-dom'

/** A tree with two kinds, a scope, a label and the two structural flags. */
interface NoteRow extends DropTargetBase {
  kind: 'note' | 'group'
  boardId?: string
  label?: string
}

const NOTE_SPEC: DropRowSpec = {
  kinds: { note: 'data-note-drop', group: 'data-note-group-drop' },
  parentAttr: 'data-note-parent',
  strings: { boardId: 'data-note-board', label: 'data-note-label' },
  flags: { expanded: 'data-note-expanded', hasChildren: 'data-note-children' },
}

/** A second tree, so the mutual-invisibility claim has something to be tested against. */
const CARD_SPEC: DropRowSpec = { kinds: { card: 'data-card-drop' }, parentAttr: 'data-card-parent' }

const notes = createDropRowDom<NoteRow>(NOTE_SPEC)
const cards = createDropRowDom<DropTargetBase>(CARD_SPEC)

/** Everything may go anywhere, except onto a row that is being dragged. */
const notePolicy: DropPolicy<DragSubjectBase, NoteRow> = {
  allowedModes: (subjects, target): AllowedModes =>
    subjects.length > 0 && !subjects.some((s) => s.id === target.id) ? ALL_MODES : NO_MODES,
  edgeBandFor: (kind) => (kind === 'group' ? EDGE_BAND_CONTAINER : EDGE_BAND_HEAVY),
}

const ROW_TOP = 100
const ROW_HEIGHT = 40

/** Mount an element carrying exactly what one of these trees publishes. */
function mount(props: Record<string, string | undefined>, top = ROW_TOP): HTMLElement {
  const el = document.createElement('div')
  for (const [key, value] of Object.entries(props)) {
    if (value !== undefined) el.setAttribute(key, value)
  }
  el.getBoundingClientRect = () =>
    ({ top, bottom: top + ROW_HEIGHT, height: ROW_HEIGHT, left: 0, width: 200 }) as DOMRect
  document.body.appendChild(el)
  return el
}

/** Point the stubbed hit test at a stack of elements, topmost first. */
function stackAt(...els: Element[]): void {
  document.elementsFromPoint = vi.fn(() => els) as typeof document.elementsFromPoint
}

/** A y coordinate at `ratio` down the row. */
const at = (ratio: number) => ROW_TOP + ROW_HEIGHT * ratio

afterEach(() => {
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

describe('what a tree publishes about its rows', () => {
  it('round-trips every field the table names', () => {
    const row: NoteRow = {
      kind: 'note',
      id: 'n1',
      parentId: 'g1',
      boardId: 'b1',
      label: 'Shopping',
      expanded: true,
      hasChildren: true,
    }

    expect(notes.read(mount(notes.props(row)))).toEqual(row)
  })

  it('publishes flags by presence, so an ordinary row carries none of them', () => {
    const props = notes.props({ kind: 'group', id: 'g1', boardId: 'b1' })

    expect(props['data-note-group-drop']).toBe('g1')
    expect(props['data-note-expanded']).toBeUndefined()
    expect(props['data-note-children']).toBeUndefined()
    expect(props['data-note-label']).toBeUndefined()
  })

  it('always publishes a container, because a row with none is at the root', () => {
    expect(notes.props({ kind: 'note', id: 'n1' })['data-note-parent']).toBe('')
    expect(notes.read(mount(notes.props({ kind: 'note', id: 'n1' })))).toMatchObject({
      parentId: '',
      expanded: false,
      hasChildren: false,
    })
  })

  it('needs neither strings nor flags — a table may be kinds and a parent alone', () => {
    const el = mount(cards.props({ kind: 'card', id: 'c1', parentId: 'c0' }))

    expect(cards.read(el)).toEqual({ kind: 'card', id: 'c1', parentId: 'c0' })
  })

  // A row assembled by hand — or one whose container attribute React dropped
  // because the value was undefined — still reads back as a rooted row rather
  // than as a row with no container at all, which every caller would then have
  // to remember to ground for itself.
  it('grounds a row that carries an id but no container', () => {
    const el = document.createElement('div')
    el.setAttribute('data-note-drop', 'n1')

    expect(notes.read(el)).toMatchObject({ id: 'n1', parentId: '' })
  })

  it('reads the right kind when a table names several', () => {
    expect(notes.read(mount(notes.props({ kind: 'group', id: 'g1' })))).toMatchObject({
      kind: 'group',
      id: 'g1',
    })
  })

  it('is not a row when it carries none of this tree’s attributes', () => {
    expect(notes.read(document.createElement('div'))).toBeNull()
  })

  // The isolation property the registry exists for: two trees can share a page
  // because neither can even see the other's rows, let alone decide about them.
  it('is invisible to the other tree, in both directions', () => {
    const note = mount(notes.props({ kind: 'note', id: 'n1' }))
    const card = mount(cards.props({ kind: 'card', id: 'c1' }))

    expect(cards.read(note)).toBeNull()
    expect(notes.read(card)).toBeNull()
  })
})

describe('finding rows in the document', () => {
  it('reads every row matching a selector, in document order', () => {
    for (const id of ['n1', 'n2']) {
      mount(notes.props({ kind: 'note', id })).setAttribute('role', 'treeitem')
    }

    expect(notes.readAll('[role="treeitem"]').map((r) => r.id)).toEqual(['n1', 'n2'])
  })

  it('skips matches that are not rows of this tree', () => {
    const stray = document.createElement('div')
    stray.setAttribute('role', 'treeitem')
    document.body.appendChild(stray)
    mount(notes.props({ kind: 'note', id: 'n1' })).setAttribute('role', 'treeitem')

    expect(notes.readAll('[role="treeitem"]').map((r) => r.id)).toEqual(['n1'])
  })

  it('scopes to a root when given one', () => {
    const scope = document.createElement('div')
    document.body.appendChild(scope)
    scope.appendChild(mount(notes.props({ kind: 'note', id: 'inside' })))
    mount(notes.props({ kind: 'note', id: 'outside' }))

    expect(notes.readAll('[data-note-drop]', scope).map((r) => r.id)).toEqual(['inside'])
    expect(notes.readAll('[data-note-drop]').map((r) => r.id)).toEqual(['inside', 'outside'])
  })

  it('finds the live element behind a subject', () => {
    const el = mount(notes.props({ kind: 'group', id: 'g1' }))

    expect(notes.elementFor({ kind: 'group', id: 'g1' })).toBe(el)
    expect(notes.elementFor({ kind: 'group', id: 'nope' })).toBeNull()
  })

  it('scopes elementFor to a root when given one', () => {
    const scope = document.createElement('div')
    document.body.appendChild(scope)
    const inside = mount(notes.props({ kind: 'note', id: 'n1' }))
    scope.appendChild(inside)

    expect(notes.elementFor({ kind: 'note', id: 'n1' }, scope)).toBe(inside)
    expect(notes.elementFor({ kind: 'note', id: 'n1' }, document.createElement('div'))).toBeNull()
  })

  it('finds nothing for a kind this tree does not publish', () => {
    mount(notes.props({ kind: 'note', id: 'n1' }))

    expect(cards.elementFor({ kind: 'note', id: 'n1' })).toBeNull()
  })
})

describe('the hit test, bound to a table and a policy', () => {
  const findNote = createDropHitTest(notes, notePolicy)
  const dragged: DragSubjectBase = { kind: 'note', id: 'dragged' }

  it('resolves the pointer against the band the policy gave for that kind', () => {
    stackAt(mount(notes.props({ kind: 'note', id: 'n1' })))

    expect(findNote(0, at(0.1), [dragged])).toMatchObject({ row: { id: 'n1', mode: 'before' } })
    expect(findNote(0, at(0.5), [dragged])).toMatchObject({ row: { mode: 'into' } })
    expect(findNote(0, at(0.9), [dragged])).toMatchObject({ row: { mode: 'after' } })
  })

  it('uses the narrower band on the kind the policy calls a container', () => {
    stackAt(mount(notes.props({ kind: 'group', id: 'g1' })))

    // 0.25 reorders on a note row and nests on a group row.
    expect(findNote(0, at(0.25), [dragged])).toMatchObject({ row: { mode: 'into' } })
  })

  it('hands back the rect it measured, so nothing measures the row twice', () => {
    stackAt(mount(notes.props({ kind: 'note', id: 'n1' })))

    expect(findNote(0, at(0.5), [dragged])).toMatchObject({
      kind: 'row',
      rect: { top: ROW_TOP, bottom: ROW_TOP + ROW_HEIGHT, left: 0, width: 200 },
    })
  })

  it("reads through a row's inner elements to the row itself", () => {
    const row = mount(notes.props({ kind: 'note', id: 'n1' }))
    const label = document.createElement('span')
    row.appendChild(label)
    stackAt(label, row)

    expect(findNote(0, at(0.5), [dragged])).toMatchObject({ row: { id: 'n1' } })
  })

  it('resolves nothing when the pointer is over no row at all', () => {
    stackAt(document.createElement('div'))

    expect(findNote(0, at(0.5), [dragged])).toBeNull()
  })

  // A refused row is the answer, not a reason to keep looking: drawing an
  // indicator on whatever sits behind it would be the indicator lying.
  it('stops at a refusing row rather than falling through to the one behind', () => {
    const refusing = mount(notes.props({ kind: 'note', id: 'dragged' }))
    const behind = mount(notes.props({ kind: 'note', id: 'n2' }))
    stackAt(refusing, behind)

    expect(findNote(0, at(0.5), [dragged])).toBeNull()
  })

  it('refuses a zero-height row rather than dividing by it', () => {
    const row = mount(notes.props({ kind: 'note', id: 'n1' }))
    row.getBoundingClientRect = () => ({ top: 0, bottom: 0, height: 0 }) as DOMRect
    stackAt(row)

    expect(findNote(0, 0, [dragged])).toBeNull()
  })

  it('draws nothing when the position lands on a mode the matrix refused', () => {
    const beforeOnly = createDropHitTest(notes, {
      allowedModes: () => REORDER_MODES,
      edgeBandFor: () => EDGE_BAND_HEAVY,
    })
    const noAfter = createDropHitTest(notes, {
      allowedModes: (): AllowedModes => ({ before: true, after: false, into: false }),
      edgeBandFor: () => EDGE_BAND_HEAVY,
    })
    stackAt(mount(notes.props({ kind: 'note', id: 'n1' })))

    expect(beforeOnly(0, at(0.9), [dragged])).toMatchObject({ row: { mode: 'after' } })
    expect(noAfter(0, at(0.9), [dragged])).toBeNull()
  })
})

describe('a whole-region drop zone', () => {
  const TRASH_ATTR = 'data-note-trash'
  const findWithTrash = createDropHitTest(notes, notePolicy, {
    attr: TRASH_ATTR,
    // Only a leaf may be thrown away; a group is refused, which is a stand-in
    // for any rule a tree wants to attach to its own region.
    hit: (subjects) =>
      subjects.every((s) => s.kind === 'note') ? ({ kind: 'trash' } as const) : null,
  })
  const dragged: DragSubjectBase = { kind: 'note', id: 'dragged' }

  const trash = () => {
    const el = document.createElement('div')
    el.setAttribute(TRASH_ATTR, '')
    return el
  }

  it('is hit before any row under it', () => {
    stackAt(trash(), mount(notes.props({ kind: 'note', id: 'n1' })))

    expect(findWithTrash(0, at(0.5), [dragged])).toEqual({ kind: 'trash' })
  })

  it('refusing the zone ends the hit test — it does not fall through to the rows', () => {
    stackAt(trash(), mount(notes.props({ kind: 'note', id: 'n1' })))

    expect(findWithTrash(0, at(0.5), [{ kind: 'group', id: 'g9' }])).toBeNull()
  })

  it('leaves rows alone when the pointer is nowhere near the zone', () => {
    stackAt(mount(notes.props({ kind: 'note', id: 'n1' })))

    expect(findWithTrash(0, at(0.5), [dragged])).toMatchObject({ kind: 'row' })
  })
})

// Addendum §2's own blocker: the pane zone (use-sidebar-drag.ts's own
// `paneZone`) already occupied `createDropHitTest`'s one `zone` slot before
// a second whole-region target (the file explorer card's trash surface)
// needed one too.
describe('several whole-region drop zones at once', () => {
  const dragged: DragSubjectBase = { kind: 'note', id: 'dragged' }
  const ZONE_A_ATTR = 'data-zone-a'
  const ZONE_B_ATTR = 'data-zone-b'
  type EitherZoneHit = { kind: 'a' } | { kind: 'b' }
  const zoneA: DropZone<DragSubjectBase, EitherZoneHit> = {
    attr: ZONE_A_ATTR,
    hit: () => ({ kind: 'a' }),
  }
  const zoneB: DropZone<DragSubjectBase, EitherZoneHit> = {
    attr: ZONE_B_ATTR,
    hit: () => ({ kind: 'b' }),
  }
  const findEither = createDropHitTest(notes, notePolicy, [zoneA, zoneB])

  const makeZone = (attr: string) => {
    const el = document.createElement('div')
    el.setAttribute(attr, '')
    return el
  }

  it('checked in the order given — the first zone in the array wins when both are present', () => {
    stackAt(makeZone(ZONE_A_ATTR), makeZone(ZONE_B_ATTR))

    expect(findEither(0, at(0.5), [dragged])).toEqual({ kind: 'a' })
  })

  it('a later zone in the array still resolves when the earlier one is not the topmost element', () => {
    stackAt(makeZone(ZONE_B_ATTR))

    expect(findEither(0, at(0.5), [dragged])).toEqual({ kind: 'b' })
  })

  it('per-element compositing order still governs — a row painted above both zones wins', () => {
    stackAt(mount(notes.props({ kind: 'note', id: 'n1' })), makeZone(ZONE_A_ATTR))

    expect(findEither(0, at(0.5), [dragged])).toMatchObject({ kind: 'row' })
  })

  it('a bare single zone (not an array) keeps working exactly as before — every existing caller', () => {
    const findSingle = createDropHitTest(notes, notePolicy, zoneA)
    stackAt(makeZone(ZONE_A_ATTR))

    expect(findSingle(0, at(0.5), [dragged])).toEqual({ kind: 'a' })
  })

  it('no zone at all is still a plain row-only hit test', () => {
    const findNone = createDropHitTest(notes, notePolicy)
    stackAt(mount(notes.props({ kind: 'note', id: 'n1' })))

    expect(findNone(0, at(0.5), [dragged])).toMatchObject({ kind: 'row' })
  })
})
