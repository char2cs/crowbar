import { describe, it, expect } from 'vitest'
import {
  changeKeyToAnchor,
  anchorToChangeKey,
} from '@/features/git/lib/diff-change-key'
import type { HunkData } from 'react-diff-view'
import type { ChangeData } from 'react-diff-view'

// ── Fixtures ────────────────────────────────────────────────────────────────────

const insertChange: ChangeData = {
  type: 'insert',
  content: 'inserted line',
  lineNumber: 5,
  isInsert: true,
}

const deleteChange: ChangeData = {
  type: 'delete',
  content: 'deleted line',
  lineNumber: 3,
  isDelete: true,
}

const normalChange: ChangeData = {
  type: 'normal',
  content: 'context line',
  oldLineNumber: 2,
  newLineNumber: 4,
  isNormal: true,
}

function makeHunks(...changes: ChangeData[]): HunkData[] {
  return [
    {
      content: '@@ -1,5 +1,5 @@',
      oldStart: 1,
      newStart: 1,
      oldLines: 5,
      newLines: 5,
      changes,
    },
  ]
}

// ── changeKeyToAnchor ───────────────────────────────────────────────────────────

describe('changeKeyToAnchor', () => {
  it('insert → side new, line = lineNumber', () => {
    expect(changeKeyToAnchor(insertChange)).toEqual({ side: 'new', line: 5 })
  })

  it('delete → side old, line = lineNumber', () => {
    expect(changeKeyToAnchor(deleteChange)).toEqual({ side: 'old', line: 3 })
  })

  it('normal → side old (canonical), line = oldLineNumber', () => {
    expect(changeKeyToAnchor(normalChange)).toEqual({ side: 'old', line: 2 })
  })
})

// ── anchorToChangeKey ───────────────────────────────────────────────────────────

describe('anchorToChangeKey', () => {
  it('finds I5 for insert change at side new, line 5', () => {
    const hunks = makeHunks(insertChange)
    expect(anchorToChangeKey(hunks, { side: 'new', line: 5 })).toBe('I5')
  })

  it('finds D3 for delete change at side old, line 3', () => {
    const hunks = makeHunks(deleteChange)
    expect(anchorToChangeKey(hunks, { side: 'old', line: 3 })).toBe('D3')
  })

  it('finds N2 for normal change at side old (oldLineNumber)', () => {
    const hunks = makeHunks(normalChange)
    expect(anchorToChangeKey(hunks, { side: 'old', line: 2 })).toBe('N2')
  })

  it('finds N2 for normal change at side new (newLineNumber=4, but key is N{oldLineNumber}=N2)', () => {
    const hunks = makeHunks(normalChange)
    // Side 'new', line=4 matches normal change where newLineNumber=4
    // The change key is getChangeKey(normalChange) = 'N2' (keyed by oldLineNumber)
    expect(anchorToChangeKey(hunks, { side: 'new', line: 4 })).toBe('N2')
  })

  it('returns null when line is not in hunks', () => {
    const hunks = makeHunks(insertChange)
    expect(anchorToChangeKey(hunks, { side: 'new', line: 99 })).toBeNull()
  })

  it('returns null for empty hunks', () => {
    expect(anchorToChangeKey([], { side: 'old', line: 1 })).toBeNull()
  })

  it('does not match insert on side old', () => {
    const hunks = makeHunks(insertChange)
    expect(anchorToChangeKey(hunks, { side: 'old', line: 5 })).toBeNull()
  })

  it('does not match delete on side new', () => {
    const hunks = makeHunks(deleteChange)
    expect(anchorToChangeKey(hunks, { side: 'new', line: 3 })).toBeNull()
  })
})

// ── Round-trip ──────────────────────────────────────────────────────────────────

describe('round-trip: changeKeyToAnchor → anchorToChangeKey', () => {
  it('insert: anchor from changeKeyToAnchor can be re-found by anchorToChangeKey', () => {
    const hunks = makeHunks(insertChange)
    const anchor = changeKeyToAnchor(insertChange)
    const key = anchorToChangeKey(hunks, anchor)
    expect(key).toBe('I5')
  })

  it('delete: anchor from changeKeyToAnchor can be re-found by anchorToChangeKey', () => {
    const hunks = makeHunks(deleteChange)
    const anchor = changeKeyToAnchor(deleteChange)
    const key = anchorToChangeKey(hunks, anchor)
    expect(key).toBe('D3')
  })

  it('normal: anchor from changeKeyToAnchor can be re-found by anchorToChangeKey', () => {
    const hunks = makeHunks(normalChange)
    const anchor = changeKeyToAnchor(normalChange)
    const key = anchorToChangeKey(hunks, anchor)
    expect(key).toBe('N2')
  })
})
