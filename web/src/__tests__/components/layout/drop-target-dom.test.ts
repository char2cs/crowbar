/**
 * The hit test: what a pointer position resolves to, and what it refuses.
 *
 * `drop-rules.ts` owns the matrix and is covered on its own; this covers the
 * layer that feeds it — the attributes a row publishes, the ratio the pointer
 * sits at inside the row, and the two answers that must never be confused: "no
 * row here" and "this row says no".
 *
 * jsdom has neither `elementsFromPoint` nor real layout, so both are stubbed.
 * That is not a compromise: the geometry IS the input, so stating it explicitly
 * is what makes the band arithmetic assertable at all.
 */
import { describe, it, expect, afterEach, vi } from 'vitest'
import {
  dragSubjectsFor,
  dropRowProps,
  findDrop,
  readDropRow,
  readSelectedSubjects,
  rowElementFor,
  sameDrop,
  type DropRow,
} from '@/components/layout/drop-target-dom'
import type { DragSubject } from '@/components/layout/drop-rules'

const ROW_TOP = 100
const ROW_HEIGHT = 36

/** Build a row element carrying exactly what a real row publishes. */
function row(props: DropRow, top = ROW_TOP): HTMLElement {
  const el = document.createElement('div')
  for (const [key, value] of Object.entries(dropRowProps(props))) {
    if (value !== undefined) el.setAttribute(key, value)
  }
  el.getBoundingClientRect = () =>
    ({
      top,
      bottom: top + ROW_HEIGHT,
      height: ROW_HEIGHT,
      left: 0,
      right: 200,
      width: 200,
    }) as DOMRect
  document.body.appendChild(el)
  return el
}

/** Point the stubbed hit test at a stack of elements, deepest first. */
function stackAt(...els: Element[]): void {
  document.elementsFromPoint = vi.fn(() => els) as typeof document.elementsFromPoint
}

/** A y coordinate at `ratio` down the row. */
const at = (ratio: number) => ROW_TOP + ROW_HEIGHT * ratio

const WS = (over: Partial<DragSubject> = {}): DragSubject => ({
  kind: 'workspace',
  id: 'ws-drag',
  repoId: 'r1',
  parentId: '',
  ...over,
})

afterEach(() => {
  document.body.innerHTML = ''
  vi.restoreAllMocks()
})

describe('what a row publishes', () => {
  it('round-trips through the DOM', () => {
    const el = row({
      kind: 'workspace',
      id: 'ws1',
      repoId: 'r1',
      parentId: 'ws0',
      locked: true,
      expanded: true,
      hasChildren: true,
    })

    expect(readDropRow(el)).toEqual({
      kind: 'workspace',
      id: 'ws1',
      repoId: 'r1',
      parentId: 'ws0',
      locked: true,
      expanded: true,
      hasChildren: true,
    })
  })

  it('publishes booleans by presence, so an ordinary row carries none of them', () => {
    const props = dropRowProps({ kind: 'folder', id: 'f1', repoId: 'r1' })

    expect(props['data-folder-drop']).toBe('f1')
    expect(props['data-drop-expanded']).toBeUndefined()
    expect(props['data-drop-children']).toBeUndefined()
    expect(props['data-drop-locked']).toBeUndefined()
  })

  it('is not a row when it carries no kind attribute', () => {
    const el = document.createElement('div')
    expect(readDropRow(el)).toBeNull()
  })

  it('finds a row element by its subject', () => {
    const el = row({ kind: 'folder', id: 'f1', repoId: 'r1' })
    expect(rowElementFor({ kind: 'folder', id: 'f1' })).toBe(el)
    expect(rowElementFor({ kind: 'folder', id: 'nope' })).toBeNull()
  })
})

describe('the bands', () => {
  it('splits a workspace row 30/40/30', () => {
    const target = row({ kind: 'workspace', id: 'ws-t', repoId: 'r1' })
    stackAt(target)

    expect(findDrop(0, at(0.1), [WS()])).toMatchObject({ row: { mode: 'before' } })
    expect(findDrop(0, at(0.5), [WS()])).toMatchObject({ row: { mode: 'into' } })
    expect(findDrop(0, at(0.9), [WS()])).toMatchObject({ row: { mode: 'after' } })
  })

  it('splits a folder row 20/60/20 — nesting into one is the cheap, common move', () => {
    const target = row({ kind: 'folder', id: 'f1', repoId: 'r1' })
    stackAt(target)

    expect(findDrop(0, at(0.1), [WS()])).toMatchObject({ row: { mode: 'before' } })
    expect(findDrop(0, at(0.25), [WS()])).toMatchObject({ row: { mode: 'into' } })
    expect(findDrop(0, at(0.9), [WS()])).toMatchObject({ row: { mode: 'after' } })
  })

  it('splits a row that cannot be nested into straight down the middle', () => {
    const target = row({ kind: 'project', id: 'p2' })
    stackAt(target)
    const project: DragSubject = { kind: 'project', id: 'p1' }

    expect(findDrop(0, at(0.45), [project])).toMatchObject({ row: { mode: 'before' } })
    expect(findDrop(0, at(0.55), [project])).toMatchObject({ row: { mode: 'after' } })
  })

  it("reads through the row's inner elements to the row itself", () => {
    const target = row({ kind: 'workspace', id: 'ws-t', repoId: 'r1' })
    const label = document.createElement('span')
    target.appendChild(label)
    stackAt(label, target)

    expect(findDrop(0, at(0.5), [WS()])).toMatchObject({ row: { id: 'ws-t', mode: 'into' } })
  })

  it('resolves nothing when the pointer is over no row at all', () => {
    stackAt(document.createElement('div'))
    expect(findDrop(0, 0, [WS()])).toBeNull()
  })
})

describe('the gap under an expanded parent is its first child slot', () => {
  it('reports `after` on an expanded parent, which the plan reads as first child', () => {
    const target = row({
      kind: 'workspace',
      id: 'ws-parent',
      repoId: 'r1',
      expanded: true,
      hasChildren: true,
    })
    stackAt(target)

    expect(findDrop(0, at(0.9), [WS()])).toMatchObject({
      row: { id: 'ws-parent', mode: 'after', expanded: true, hasChildren: true },
    })
  })

  it('refuses that same gap to a locked row, because landing there would nest it', () => {
    const target = row({
      kind: 'workspace',
      id: 'ws-parent',
      repoId: 'r1',
      parentId: '',
      expanded: true,
      hasChildren: true,
    })
    stackAt(target)

    expect(findDrop(0, at(0.9), [WS({ locked: true })])).toBeNull()
    // Its own half of the row still reorders it.
    expect(findDrop(0, at(0.2), [WS({ locked: true })])).toMatchObject({ row: { mode: 'before' } })
  })
})

describe('every refusal', () => {
  it('refuses a drop on the row being dragged', () => {
    const target = row({ kind: 'workspace', id: 'ws-drag', repoId: 'r1' })
    stackAt(target)

    expect(findDrop(0, at(0.5), [WS()])).toBeNull()
  })

  it('refuses a cross-repo drop', () => {
    const target = row({ kind: 'workspace', id: 'ws-t', repoId: 'r2' })
    stackAt(target)

    expect(findDrop(0, at(0.5), [WS({ repoId: 'r1' })])).toBeNull()
  })

  it('refuses a mixed-kind selection', () => {
    const target = row({ kind: 'workspace', id: 'ws-t', repoId: 'r1' })
    stackAt(target)

    expect(findDrop(0, at(0.5), [WS(), { kind: 'repo', id: 'r1' }])).toBeNull()
  })

  it('never offers a locked row the nest affordance, anywhere in the row', () => {
    const target = row({ kind: 'workspace', id: 'ws-t', repoId: 'r1', parentId: '' })
    stackAt(target)

    for (const ratio of [0.35, 0.5, 0.65]) {
      expect(findDrop(0, at(ratio), [WS({ locked: true })])?.kind === 'row').toBe(true)
      expect(findDrop(0, at(ratio), [WS({ locked: true })])).not.toMatchObject({
        row: { mode: 'into' },
      })
    }
  })

  it('refuses a locked row any parent but its own', () => {
    const target = row({ kind: 'workspace', id: 'ws-t', repoId: 'r1', parentId: 'ws-other' })
    stackAt(target)

    expect(findDrop(0, at(0.2), [WS({ locked: true, parentId: 'ws-mine' })])).toBeNull()
  })

  it('refuses a locked row the repo root, which is also a re-parent', () => {
    const target = row({ kind: 'repo', id: 'r1', repoId: 'r1' })
    stackAt(target)

    expect(findDrop(0, at(0.5), [WS({ locked: true })])).toBeNull()
    expect(findDrop(0, at(0.5), [WS()])).toMatchObject({ row: { mode: 'into' } })
  })

  it('refuses a workspace dropped on a project row', () => {
    const target = row({ kind: 'project', id: 'p1' })
    stackAt(target)

    expect(findDrop(0, at(0.5), [WS()])).toBeNull()
  })

  it('refuses a project dropped on anything but another project', () => {
    const target = row({ kind: 'repo', id: 'r1', repoId: 'r1' })
    stackAt(target)

    expect(findDrop(0, at(0.5), [{ kind: 'project', id: 'p1' }])).toBeNull()
  })

  it('refuses a repo dropped onto a workspace', () => {
    const target = row({ kind: 'workspace', id: 'ws-t', repoId: 'r1' })
    stackAt(target)

    expect(findDrop(0, at(0.5), [{ kind: 'repo', id: 'r2' }])).toBeNull()
  })

  // A refused row is the answer, not a reason to keep looking: drawing an
  // indicator on whatever sits behind it would be the indicator lying.
  it('does not fall through to a row behind a refusing one', () => {
    const refusing = row({ kind: 'workspace', id: 'ws-t', repoId: 'r2' })
    const behind = row({ kind: 'repo', id: 'r1', repoId: 'r1' })
    stackAt(refusing, behind)

    expect(findDrop(0, at(0.5), [WS({ repoId: 'r1' })])).toBeNull()
  })

  it('refuses a zero-height row rather than dividing by it', () => {
    const target = row({ kind: 'workspace', id: 'ws-t', repoId: 'r1' })
    target.getBoundingClientRect = () => ({ top: 0, bottom: 0, height: 0 }) as DOMRect
    stackAt(target)

    expect(findDrop(0, 0, [WS()])).toBeNull()
  })
})

describe('the editor pane, as the removal target', () => {
  const pane = () => {
    const el = document.createElement('div')
    el.setAttribute('data-pane-drop', '')
    stackAt(el)
    return el
  }

  it('is hit before any row', () => {
    pane()

    expect(findDrop(0, 0, [WS()])).toEqual({ kind: 'pane' })
  })

  it('takes repos and folders too — a whole repo can leave this way', () => {
    pane()

    expect(findDrop(0, 0, [{ kind: 'repo', id: 'r1' }])).toEqual({ kind: 'pane' })
    expect(findDrop(0, 0, [{ kind: 'folder', id: 'f1', repoId: 'r1' }])).toEqual({ kind: 'pane' })
  })

  // A project IS removable this way now — it goes to the tray like a repo, with
  // a confirmation naming everything inside it. It was excluded when the sidebar
  // had no way to delete one by any gesture; leaving it excluded afterwards was
  // worse than either answer, because the pane's veil is drawn from what a
  // removal would PLAN — so it offered to remove a project and then refused on
  // release.
  it('takes a project too', () => {
    pane()

    expect(findDrop(0, 0, [{ kind: 'project', id: 'p1' }])).toEqual({ kind: 'pane' })
  })

  it('offers nothing for a protected branch', () => {
    pane()

    expect(findDrop(0, 0, [WS({ locked: true })])).toBeNull()
  })
})

describe('the rows a drag carries', () => {
  it('carries the selection when the grabbed row is part of it', () => {
    const selection = [WS({ id: 'a' }), WS({ id: 'b' })]

    expect(dragSubjectsFor(WS({ id: 'b' }), selection)).toEqual(selection)
  })

  it('carries the grabbed row alone when it is outside the selection', () => {
    const grabbed = WS({ id: 'c' })

    expect(dragSubjectsFor(grabbed, [WS({ id: 'a' }), WS({ id: 'b' })])).toEqual([grabbed])
  })

  it('is one element even with nothing selected', () => {
    const grabbed = WS({ id: 'c' })

    expect(dragSubjectsFor(grabbed, [])).toEqual([grabbed])
  })

  it('reads the selection off the tree, from what assistive tech reads', () => {
    const a = row({ kind: 'workspace', id: 'a', repoId: 'r1' })
    a.setAttribute('aria-selected', 'true')
    row({ kind: 'workspace', id: 'b', repoId: 'r1' })

    expect(readSelectedSubjects().map((s) => s.id)).toEqual(['a'])
  })
})

describe('sameDrop', () => {
  const base = { kind: 'workspace' as const, id: 'a', mode: 'before' as const }

  it('is true only when the drop would draw and commit the same thing', () => {
    expect(sameDrop(base, { ...base })).toBe(true)
    expect(sameDrop(base, { ...base, mode: 'after' })).toBe(false)
    expect(sameDrop(base, { ...base, id: 'b' })).toBe(false)
    expect(sameDrop(null, null)).toBe(true)
    expect(sameDrop(base, null)).toBe(false)
  })
})
