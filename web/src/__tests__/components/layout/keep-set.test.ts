/**
 * Contract pins for the keep set.
 *
 * The bug this exists to prevent: treating the multiselection and the keep set
 * as one thing. If they are one set, then either clearing the selection wipes
 * the rows you deliberately kept, or the kept rows keep the selection styling
 * and you can no longer tell which workspace you are actually in. Both were
 * built and both were wrong.
 *
 * So the pins are mostly about what must NOT change the keep set: clearing the
 * selection, opening another workspace, navigating anywhere at all. Only a
 * collapse fills it, and only cmd-clicking a row off or the parent's fold-away
 * control empties it.
 */
import { describe, expect, test } from 'vitest'
import {
  indexFromParents,
  snapshotOnCollapse,
  releaseOnExpand,
  keptRootsUnder,
  isHoldingRows,
  foldAway,
} from '@/components/layout/keep-set'

//  develop
//    ├─ Reviewing (folder)
//    │    ├─ plate
//    │    └─ clip
//    └─ mine (folder)
//         └─ sidebar
//              └─ folders
const index = indexFromParents(
  new Map<string, string | undefined>([
    ['develop', undefined],
    ['reviewing', 'develop'],
    ['plate', 'reviewing'],
    ['clip', 'reviewing'],
    ['mine', 'develop'],
    ['sidebar', 'mine'],
    ['folders', 'sidebar'],
  ]),
)

const set = (...ids: string[]) => new Set(ids)

describe('snapshotOnCollapse — the keep set is filled at collapse, from both sources', () => {
  test('takes the multiselection under this parent', () => {
    expect(snapshotOnCollapse('reviewing', index, set('plate', 'clip'), null).sort()).toEqual([
      'clip',
      'plate',
    ])
  })

  test('takes the open workspace even when nothing is selected', () => {
    expect(snapshotOnCollapse('mine', index, set(), 'sidebar')).toEqual(['sidebar'])
  })

  test('ignores selection outside this parent', () => {
    expect(snapshotOnCollapse('reviewing', index, set('sidebar'), null)).toEqual([])
  })

  test('reaches any depth, not just direct children', () => {
    expect(snapshotOnCollapse('develop', index, set('folders'), null)).toEqual(['folders'])
  })
})

describe('the keep set survives everything except an explicit dismissal', () => {
  test('clearing the multiselection does not touch it', () => {
    const kept = set(...snapshotOnCollapse('reviewing', index, set('plate', 'clip'), null))
    // The user plain-clicks somewhere: selection is wiped.
    const selectionAfter = set()
    expect(selectionAfter.size).toBe(0)
    // The keep set is a different object and is untouched.
    expect([...kept].sort()).toEqual(['clip', 'plate'])
  })

  test('opening an unrelated workspace does not touch it', () => {
    const kept = set(...snapshotOnCollapse('mine', index, set(), 'sidebar'))
    // activeId moves elsewhere entirely; nothing recomputes the keep set.
    expect(keptRootsUnder('mine', index, kept)).toEqual(['sidebar'])
  })

  test('the parent’s fold-away control is what empties it', () => {
    const kept = set('plate', 'clip')
    expect([...foldAway('reviewing', index, kept)]).toEqual([])
  })

  test('folding one parent leaves another parent’s kept rows alone', () => {
    const kept = set('plate', 'sidebar')
    expect([...foldAway('reviewing', index, kept)]).toEqual(['sidebar'])
  })

  test('expanding releases exactly this parent’s rows', () => {
    expect(releaseOnExpand('reviewing', index).sort()).toEqual(['clip', 'plate'])
  })
})

describe('keptRootsUnder — a kept row brings its subtree, so only outermost render', () => {
  test('returns the kept rows directly', () => {
    expect(keptRootsUnder('reviewing', index, set('plate')).sort()).toEqual(['plate'])
  })

  test('drops a kept row that sits under another kept row', () => {
    // Both sidebar and its child are kept; only sidebar should be hoisted,
    // because rendering it brings `folders` along underneath it.
    expect(keptRootsUnder('develop', index, set('sidebar', 'folders'))).toEqual(['sidebar'])
  })

  test('keeps two kept rows that are siblings', () => {
    expect(keptRootsUnder('reviewing', index, set('plate', 'clip')).sort()).toEqual([
      'clip',
      'plate',
    ])
  })

  test('returns nothing when the keep set is empty', () => {
    expect(keptRootsUnder('develop', index, set())).toEqual([])
  })
})

describe('isHoldingRows — the parent shows the three-dot state', () => {
  test('true when it holds something', () => {
    expect(isHoldingRows('reviewing', index, set('plate'))).toBe(true)
  })

  test('true for a deep descendant, not just a direct child', () => {
    expect(isHoldingRows('develop', index, set('folders'))).toBe(true)
  })

  test('false when it holds nothing', () => {
    expect(isHoldingRows('reviewing', index, set('sidebar'))).toBe(false)
  })
})

describe('indexFromParents — a corrupt chain degrades, never hangs', () => {
  test('a cycle terminates instead of looping forever', () => {
    const cyclic = indexFromParents(
      new Map<string, string | undefined>([
        ['a', 'b'],
        ['b', 'a'],
      ]),
    )
    // Both calls must return, which is the whole assertion — a naive walk hangs.
    expect(cyclic.descendantsOf('a')).toContain('b')
    expect(cyclic.isAncestorOf('a', 'b')).toBe(true)
  })

  test('a self-parented node does not recurse', () => {
    const selfish = indexFromParents(new Map<string, string | undefined>([['x', 'x']]))
    expect(selfish.isAncestorOf('x', 'x')).toBe(true)
    expect(selfish.descendantsOf('x')).toEqual([])
  })

  test('an unknown id has no descendants and no ancestors', () => {
    expect(index.descendantsOf('nope')).toEqual([])
    expect(index.isAncestorOf('develop', 'nope')).toBe(false)
  })
})
