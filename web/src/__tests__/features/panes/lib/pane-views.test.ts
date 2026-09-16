import { describe, expect, it } from 'vitest'
import {
  panesInView,
  partitionLayoutByView,
  viewIdOf,
  viewIsShared,
} from '@/features/panes/lib/pane-views'
import {
  createLeaf,
  createSplit,
  getAllLeafIds,
  normalizeLayout,
} from '@/features/panes/utils/pane-layout'
import type { PaneGroup } from '@/features/panes/types/pane'

function pane(id: string, viewId?: string): PaneGroup {
  return {
    id,
    type: 'group',
    chatId: null,
    runnerId: null,
    editorTabIds: [],
    activeEditorTabId: null,
    editorOpen: false,
    ...(viewId !== undefined && { viewId }),
  }
}

function record(...panes: PaneGroup[]): Record<string, PaneGroup> {
  return Object.fromEntries(panes.map((p) => [p.id, p]))
}

describe('viewIdOf', () => {
  it('answers the tagged view when there is one', () => {
    expect(viewIdOf(pane('p1', 'view-a'))).toBe('view-a')
  })

  // No migration, just a correct reading of the old shape: every pane in a
  // layout persisted before views existed was independent.
  it('reads an UNTAGGED pane as a view of its own, named by its own id', () => {
    expect(viewIdOf(pane('p1'))).toBe('p1')
  })

  it('never lets two untagged panes look like one view', () => {
    expect(viewIdOf(pane('p1'))).not.toBe(viewIdOf(pane('p2')))
  })
})

describe('panesInView', () => {
  it('collects every pane sharing the view, the subject included', () => {
    const panes = record(pane('p1', 'v'), pane('p2', 'v'), pane('p3', 'other'))
    expect(panesInView(panes, 'p1').map((p) => p.id)).toEqual(['p1', 'p2'])
  })

  // The whole reason a dissolving view needs no special case anywhere.
  it('a pane nobody merged with is a view of exactly one', () => {
    const panes = record(pane('p1', 'v'), pane('p2', 'other'))
    expect(panesInView(panes, 'p1').map((p) => p.id)).toEqual(['p1'])
  })

  it('is empty for a pane that does not exist', () => {
    expect(panesInView(record(pane('p1')), 'gone')).toEqual([])
  })
})

describe('viewIsShared', () => {
  it('is true only while another pane carries the same view', () => {
    expect(viewIsShared(record(pane('p1', 'v'), pane('p2', 'v')), 'p1')).toBe(true)
  })

  it('is false for a lone member — a group of one is ungrouped', () => {
    expect(viewIsShared(record(pane('p1', 'v'), pane('p2', 'w')), 'p1')).toBe(false)
  })

  it('is false for untagged panes, which are never each other', () => {
    expect(viewIsShared(record(pane('p1'), pane('p2')), 'p1')).toBe(false)
  })
})

/**
 * `partitionLayoutByView` — the one function that has to deal with a tiling
 * tree holding more than one view, because that is the shape every layout
 * persisted before views owned their own trees is in. Splitting it correctly
 * is what stops the first reload after this feature ships from faithfully
 * restoring the side-by-side tiling the feature removes.
 */
describe('partitionLayoutByView', () => {
  it('gives every view in a mixed tree a tree of its own', () => {
    const panes = record(pane('a', 'v1'), pane('b', 'v2'), pane('c', 'v1'))
    const layout = createSplit(
      'horizontal',
      createLeaf('a'),
      createSplit('vertical', createLeaf('b'), createLeaf('c')),
    )

    const trees = partitionLayoutByView(layout, panes)

    expect(Object.keys(trees).sort()).toEqual(['v1', 'v2'])
    expect(getAllLeafIds(trees.v1).sort()).toEqual(['a', 'c'])
    expect(getAllLeafIds(trees.v2)).toEqual(['b'])
  })

  it('reads UNTAGGED panes as one view each — the pre-views shape, correctly', () => {
    const panes = record(pane('a'), pane('b'))
    const layout = createSplit('horizontal', createLeaf('a'), createLeaf('b'))

    const trees = partitionLayoutByView(layout, panes)

    expect(Object.keys(trees).sort()).toEqual(['a', 'b'])
    expect(getAllLeafIds(trees.a)).toEqual(['a'])
    expect(getAllLeafIds(trees.b)).toEqual(['b'])
  })

  it('a tree that is already one view comes back whole and unchanged', () => {
    const panes = record(pane('a', 'v'), pane('b', 'v'))
    const layout = createSplit('horizontal', createLeaf('a'), createLeaf('b'))

    const trees = partitionLayoutByView(layout, panes)

    expect(Object.keys(trees)).toEqual(['v'])
    expect(trees.v).toEqual(normalizeLayout(layout))
  })

  // Built by subtraction rather than by re-tiling, so a view that was three
  // nested splits deep keeps that arrangement instead of being flattened into
  // arbitrary halves.
  it('keeps a view internal nesting when its view-mates are subtracted out', () => {
    const panes = record(pane('a', 'v1'), pane('x', 'v2'), pane('b', 'v1'), pane('c', 'v1'))
    const layout = createSplit(
      'horizontal',
      createLeaf('x'),
      createSplit(
        'vertical',
        createLeaf('a'),
        createSplit('horizontal', createLeaf('b'), createLeaf('c')),
      ),
    )

    const trees = partitionLayoutByView(layout, panes)

    expect(getAllLeafIds(trees.v1)).toEqual(['a', 'b', 'c'])
    const v1 = trees.v1
    expect(v1.type).toBe('split')
    if (v1.type === 'split') expect(v1.direction).toBe('vertical')
  })

  it('answers nothing for a tree whose leaves no pane record knows', () => {
    // Every leaf reads as its own view via `viewIdOf`'s id fallback, so this
    // never silently collapses unknown panes into one shared arrangement.
    const trees = partitionLayoutByView(
      createSplit('horizontal', createLeaf('a'), createLeaf('b')),
      {},
    )
    expect(Object.keys(trees).sort()).toEqual(['a', 'b'])
  })
})
