import { describe, expect, it } from 'vitest'
import {
  distributeSplit,
  flattenForRender,
  getAdjacentLeafId,
  normalizeLayout,
  resizeFlattenedLayout,
  splitLayout,
  createLeaf,
  createSplit,
  findSplit,
} from '@/features/panes/utils/pane-layout'
import type { LayoutNode, LayoutSplit } from '@/features/panes/types/pane'

function leaf(id: string) {
  return createLeaf(id)
}

describe('splitLayout', () => {
  it('places the new pane after the current pane by default', () => {
    const root = leaf('root')
    const result = splitLayout(root, 'root', 'horizontal')

    expect(result).not.toBeNull()
    if (!result) return
    expect(result.layout.type).toBe('split')
    const split = result.layout as LayoutSplit
    expect(split.first.id).toBe('root')
    expect(split.second.id).toBe(result.newPaneId)
  })

  it('places the new pane before the current pane when requested', () => {
    const root = leaf('root')
    const result = splitLayout(root, 'root', 'horizontal', 'before')

    expect(result).not.toBeNull()
    if (!result) return
    expect(result.layout.type).toBe('split')
    const split = result.layout as LayoutSplit
    expect(split.first.id).toBe(result.newPaneId)
    expect(split.second.id).toBe('root')
  })
})

describe('normalizeLayout', () => {
  it('repairs unsafe split sizes', () => {
    const root: LayoutNode = createSplit('horizontal', leaf('left'), leaf('right'), [0, 0])

    const normalized = normalizeLayout(root)

    expect(normalized.type).toBe('split')
    if (normalized.type !== 'split') return
    expect(normalized.sizes[0] + normalized.sizes[1]).toBe(100)
  })
})

describe('flattened layout', () => {
  it('flattens same-direction nested splits into one resize row', () => {
    const row: LayoutNode = createSplit(
      'horizontal',
      leaf('left'),
      createSplit('horizontal', leaf('middle'), leaf('right'), [50, 50]),
      [50, 50],
    )

    expect(row.type).toBe('split')
    if (row.type !== 'split') return
    const entries = flattenForRender(row)
    expect(entries.map((e) => e.node.id)).toEqual(['left', 'middle', 'right'])
    expect(entries.map((e) => e.size)).toEqual([50, 25, 25])
  })

  it('resizes adjacent flattened entries and writes nested split sizes once', () => {
    const nestedSplit = createSplit('horizontal', leaf('middle'), leaf('right'), [50, 50])
    const rowSplit = createSplit('horizontal', leaf('left'), nestedSplit, [50, 50])

    const resized = resizeFlattenedLayout(rowSplit, rowSplit.id, 0, [50, 50])

    expect(resized.type).toBe('split')
    if (resized.type !== 'split') return
    const nested = findSplit(resized, nestedSplit.id)
    expect(nested).not.toBeNull()
    expect(resized.sizes[0] + resized.sizes[1]).toBeCloseTo(100)
    if (nested) expect(nested.sizes[0] + nested.sizes[1]).toBeCloseTo(100)
  })

  it('distributes entries evenly across nested same-direction splits', () => {
    const nestedSplit = createSplit('horizontal', leaf('middle'), leaf('right'), [75, 25])
    const rowSplit = createSplit('horizontal', leaf('left'), nestedSplit, [80, 20])

    const distributed = distributeSplit(rowSplit, rowSplit.id)

    expect(distributed.type).toBe('split')
    if (distributed.type !== 'split') return
    expect(distributed.sizes).toEqual([50, 50])
  })
})

describe('getAdjacentLeafId', () => {
  it('finds panes by geometric direction instead of tree order', () => {
    const rightColumn = createSplit('vertical', leaf('top-right'), leaf('bottom-right'), [50, 50])
    const root = createSplit('horizontal', leaf('left'), rightColumn, [50, 50])

    expect(getAdjacentLeafId(root, 'top-right', 'left')).toBe('left')
    expect(getAdjacentLeafId(root, 'bottom-right', 'left')).toBe('left')
    expect(getAdjacentLeafId(root, 'left', 'right')).toBe('top-right')
    expect(getAdjacentLeafId(root, 'top-right', 'down')).toBe('bottom-right')
    expect(getAdjacentLeafId(root, 'bottom-right', 'up')).toBe('top-right')
  })
})
