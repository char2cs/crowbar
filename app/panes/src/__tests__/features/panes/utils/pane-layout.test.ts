import { describe, it, expect } from 'vitest'
import type { LayoutLeaf, LayoutSplit } from '@/features/panes/types/pane'
import { createSplit } from '@/__tests__/__fixtures__/pane-layout'
import {
  createLeaf,
  splitLayout,
  closeLayout,
  findLeaf,
  findSplit,
  getAllLeafIds,
  updateSplitSizes,
  getAdjacentLeafId,
} from '@/features/panes/utils/pane-layout'

describe('createLeaf', () => {
  it('creates a leaf with given id', () => {
    expect(createLeaf('abc')).toEqual({ type: 'pane', id: 'abc' })
  })
})

describe('a new split', () => {
  it('starts with 50/50 sizes', () => {
    const result = splitLayout(createLeaf('a'), 'a', 'horizontal')
    expect(result?.layout.type).toBe('split')
    expect((result?.layout as LayoutSplit).sizes).toEqual([50, 50])
  })
  it('clamps sizes below MIN_PANE_SIZE', () => {
    const s = createSplit('horizontal', createLeaf('a'), createLeaf('b'))
    const resized = updateSplitSizes(s, s.id, [2, 98]) as LayoutSplit
    expect(resized.sizes[0]).toBeGreaterThanOrEqual(10)
  })
})

describe('findLeaf', () => {
  it('finds leaf at root', () => {
    const a = createLeaf('a')
    expect(findLeaf(a, 'a')).toBe(a)
  })
  it('finds nested leaf', () => {
    const a = createLeaf('a')
    const b = createLeaf('b')
    expect(findLeaf(createSplit('horizontal', a, b), 'b')?.id).toBe('b')
  })
  it('returns null for unknown id', () => {
    expect(findLeaf(createLeaf('a'), 'z')).toBeNull()
  })
})

describe('getAllLeafIds', () => {
  it('returns all leaf ids in order', () => {
    const tree = createSplit(
      'horizontal',
      createLeaf('a'),
      createSplit('horizontal', createLeaf('b'), createLeaf('c')),
    )
    expect(getAllLeafIds(tree)).toEqual(['a', 'b', 'c'])
  })
})

describe('splitLayout', () => {
  it('splits and returns new pane id after', () => {
    const result = splitLayout(createLeaf('root'), 'root', 'horizontal', 'after')
    expect(result).not.toBeNull()
    expect(getAllLeafIds(result!.layout)).toHaveLength(2)
    expect(getAllLeafIds(result!.layout)[0]).toBe('root')
  })
  it('places new pane before when specified', () => {
    const result = splitLayout(createLeaf('root'), 'root', 'horizontal', 'before')!
    expect(getAllLeafIds(result.layout)[1]).toBe('root')
  })
  it('returns null for unknown id', () => {
    expect(splitLayout(createLeaf('a'), 'z', 'horizontal')).toBeNull()
  })
})

describe('closeLayout', () => {
  it('returns null when closing only leaf', () => {
    expect(closeLayout(createLeaf('a'), 'a')).toBeNull()
  })
  it('returns sibling when closing one side', () => {
    const result = closeLayout(createSplit('horizontal', createLeaf('a'), createLeaf('b')), 'a')
    expect((result as LayoutLeaf | null)?.id).toBe('b')
  })
  it('collapses nested split', () => {
    const inner = createSplit('horizontal', createLeaf('b'), createLeaf('c'))
    const outer = createSplit('horizontal', createLeaf('a'), inner)
    expect(getAllLeafIds(closeLayout(outer, 'b')!)).toEqual(['a', 'c'])
  })
})

describe('updateSplitSizes — chained same-direction splits', () => {
  it('resizing the outer split of a 3-leaf chain leaves the inner split untouched', () => {
    const inner = createSplit('horizontal', createLeaf('a'), createLeaf('b'), [40, 60])
    const outer = createSplit('horizontal', inner, createLeaf('c'), [70, 30])

    const result = updateSplitSizes(outer, outer.id, [55, 45])

    const outerAfter = findSplit(result, outer.id)!
    expect(outerAfter.sizes[0]).toBeCloseTo(55)
    expect(outerAfter.sizes[1]).toBeCloseTo(45)

    const innerAfter = findSplit(result, inner.id)!
    expect(innerAfter.sizes[0]).toBeCloseTo(40)
    expect(innerAfter.sizes[1]).toBeCloseTo(60)
  })

  it('resizing the inner split of a 3-leaf chain leaves the outer split untouched', () => {
    const inner = createSplit('horizontal', createLeaf('a'), createLeaf('b'), [40, 60])
    const outer = createSplit('horizontal', inner, createLeaf('c'), [70, 30])

    const result = updateSplitSizes(outer, inner.id, [20, 80])

    const outerAfter = findSplit(result, outer.id)!
    expect(outerAfter.sizes[0]).toBeCloseTo(70)
    expect(outerAfter.sizes[1]).toBeCloseTo(30)

    const innerAfter = findSplit(result, inner.id)!
    expect(innerAfter.sizes[0]).toBeCloseTo(20)
    expect(innerAfter.sizes[1]).toBeCloseTo(80)
  })
})

describe('getAdjacentLeafId', () => {
  it('returns right sibling', () => {
    const split = createSplit('horizontal', createLeaf('a'), createLeaf('b'))
    expect(getAdjacentLeafId(split, 'a', 'right')).toBe('b')
  })
  it('returns left sibling', () => {
    const split = createSplit('horizontal', createLeaf('a'), createLeaf('b'))
    expect(getAdjacentLeafId(split, 'b', 'left')).toBe('a')
  })
  it('returns null at boundary', () => {
    const split = createSplit('horizontal', createLeaf('a'), createLeaf('b'))
    expect(getAdjacentLeafId(split, 'a', 'left')).toBeNull()
  })
})
