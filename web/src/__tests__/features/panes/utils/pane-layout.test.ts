import { describe, it, expect } from 'vitest'
import {
  createLeaf,
  createSplit,
  splitLayout,
  closeLayout,
  findLeaf,
  findSplit,
  getAllLeafIds,
  flattenForRender,
  resizeFlattenedLayout,
  getAdjacentLeafId,
} from '@/features/panes/utils/pane-layout'

describe('createLeaf', () => {
  it('creates a leaf with given id', () => {
    expect(createLeaf('abc')).toEqual({ type: 'pane', id: 'abc' })
  })
})

describe('createSplit', () => {
  it('creates split with 50/50 default sizes', () => {
    const s = createSplit('horizontal', createLeaf('a'), createLeaf('b'))
    expect(s.type).toBe('split')
    expect(s.sizes[0] + s.sizes[1]).toBeCloseTo(100)
  })
  it('clamps sizes below MIN_PANE_SIZE', () => {
    const s = createSplit('horizontal', createLeaf('a'), createLeaf('b'), [2, 98])
    expect(s.sizes[0]).toBeGreaterThanOrEqual(10)
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
    expect((result as any)?.id).toBe('b')
  })
  it('collapses nested split', () => {
    const inner = createSplit('horizontal', createLeaf('b'), createLeaf('c'))
    const outer = createSplit('horizontal', createLeaf('a'), inner)
    expect(getAllLeafIds(closeLayout(outer, 'b')!)).toEqual(['a', 'c'])
  })
})

describe('flattenForRender', () => {
  it('flattens same-direction nested splits', () => {
    const inner = createSplit('horizontal', createLeaf('b'), createLeaf('c'), [50, 50])
    const outer = createSplit('horizontal', createLeaf('a'), inner, [50, 50])
    const entries = flattenForRender(outer)
    expect(entries).toHaveLength(3)
    expect(entries.map((e) => (e.node as any).id)).toEqual(['a', 'b', 'c'])
  })
  it('does not flatten cross-direction splits', () => {
    const inner = createSplit('vertical', createLeaf('b'), createLeaf('c'))
    const outer = createSplit('horizontal', createLeaf('a'), inner)
    const entries = flattenForRender(outer)
    expect(entries).toHaveLength(2)
    expect(entries[1].node.type).toBe('split')
  })
})

describe('resizeFlattenedLayout', () => {
  it('updates sizes on resize', () => {
    const a = createLeaf('a')
    const b = createLeaf('b')
    const split = createSplit('horizontal', a, b, [50, 50])
    const result = resizeFlattenedLayout(split, split.id, 0, [70, 30])
    const updated = findSplit(result, split.id)!
    expect(updated.sizes[0]).toBeGreaterThan(60)
    expect(updated.sizes[0] + updated.sizes[1]).toBeCloseTo(100)
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
