/**
 * The tree-agnostic half of drag and drop.
 *
 * Every assertion here is written in a vocabulary the sidebar does not use —
 * 'note' and 'group' rows on a board that does not exist — because the one
 * property worth pinning is that this core needs no particular tree to work. A
 * test phrased in workspaces and repos would pass just as happily against a
 * core that had the sidebar's matrix quietly baked back into it, which is the
 * regression this file exists to catch.
 */
import { describe, expect, it } from 'vitest'
import {
  ALL_MODES,
  EDGE_BAND_CONTAINER,
  EDGE_BAND_HEAVY,
  INTO_MODES,
  NO_MODES,
  REORDER_MODES,
  anyModeAllowed,
  dragSubjectsFor,
  dropModeAt,
  resolvesToFirstChild,
  sameDrop,
  type AllowedModes,
  type DropMode,
  type DropTargetBase,
} from '@/components/tree-dnd/drop-core'

const target = (over: Partial<DropTargetBase> = {}): DropTargetBase => ({
  kind: 'note',
  id: 'n1',
  parentId: '',
  ...over,
})

const modes = (over: Partial<AllowedModes> = {}): AllowedModes => ({ ...ALL_MODES, ...over })

describe('the four answers a matrix gives', () => {
  it('spells each refusal and permission exactly once', () => {
    expect(NO_MODES).toEqual({ before: false, after: false, into: false })
    expect(REORDER_MODES).toEqual({ before: true, after: true, into: false })
    expect(INTO_MODES).toEqual({ before: false, after: false, into: true })
    expect(ALL_MODES).toEqual({ before: true, after: true, into: true })
  })

  it('offers two bands, so two trees cannot drift into different geometry', () => {
    expect(EDGE_BAND_CONTAINER).toBe(0.2)
    expect(EDGE_BAND_HEAVY).toBe(0.3)
    expect(EDGE_BAND_CONTAINER).toBeLessThan(EDGE_BAND_HEAVY)
  })
})

describe('resolvesToFirstChild', () => {
  it('is true under an expanded row that has children', () => {
    expect(resolvesToFirstChild(target({ expanded: true, hasChildren: true }), 'after')).toBe(true)
  })

  it('is false when the row is collapsed — the gap is then its own', () => {
    expect(resolvesToFirstChild(target({ expanded: false, hasChildren: true }), 'after')).toBe(
      false,
    )
  })

  it('is false when the row has nothing to be the first child of', () => {
    expect(resolvesToFirstChild(target({ expanded: true, hasChildren: false }), 'after')).toBe(
      false,
    )
  })

  it('is false for before and into, which name their own slot', () => {
    const t = target({ expanded: true, hasChildren: true })
    expect(resolvesToFirstChild(t, 'before')).toBe(false)
    expect(resolvesToFirstChild(t, 'into')).toBe(false)
  })
})

describe('anyModeAllowed', () => {
  it('is true when the matrix left anything at all on the table', () => {
    expect(anyModeAllowed({ before: true, after: false, into: false })).toBe(true)
    expect(anyModeAllowed({ before: false, after: true, into: false })).toBe(true)
    expect(anyModeAllowed({ before: false, after: false, into: true })).toBe(true)
  })

  it('is false only for a total refusal', () => {
    expect(anyModeAllowed(NO_MODES)).toBe(false)
  })
})

describe('dropModeAt', () => {
  it('splits a row that cannot be nested into straight down the middle', () => {
    expect(dropModeAt(0.49, REORDER_MODES, EDGE_BAND_HEAVY)).toBe('before')
    expect(dropModeAt(0.5, REORDER_MODES, EDGE_BAND_HEAVY)).toBe('after')
  })

  it('gives the whole row to a nest when nesting is the only thing on offer', () => {
    expect(dropModeAt(0.01, INTO_MODES, EDGE_BAND_HEAVY)).toBe('into')
    expect(dropModeAt(0.99, INTO_MODES, EDGE_BAND_HEAVY)).toBe('into')
  })

  it('reorders in the outer band and nests in the middle', () => {
    expect(dropModeAt(0.1, ALL_MODES, EDGE_BAND_CONTAINER)).toBe('before')
    expect(dropModeAt(0.5, ALL_MODES, EDGE_BAND_CONTAINER)).toBe('into')
    expect(dropModeAt(0.9, ALL_MODES, EDGE_BAND_CONTAINER)).toBe('after')
  })

  it('takes the band as a number, so a wider one moves the boundaries with it', () => {
    // 0.25 nests on a 20% band and reorders on a 30% one. That single ratio
    // behaving differently IS the knob a tree turns per row kind.
    expect(dropModeAt(0.25, ALL_MODES, EDGE_BAND_CONTAINER)).toBe('into')
    expect(dropModeAt(0.25, ALL_MODES, EDGE_BAND_HEAVY)).toBe('before')
    expect(dropModeAt(0.75, ALL_MODES, EDGE_BAND_HEAVY)).toBe('after')
  })

  it('treats the band edge itself as the nest, not the reorder', () => {
    expect(dropModeAt(EDGE_BAND_HEAVY, ALL_MODES, EDGE_BAND_HEAVY)).toBe('into')
    expect(dropModeAt(1 - EDGE_BAND_HEAVY, ALL_MODES, EDGE_BAND_HEAVY)).toBe('into')
  })

  it('returns null when the band lands on a mode the matrix refused', () => {
    // Half a row can be live while the other half is not: the position still
    // resolves, and the refusal is what turns it into "draw nothing".
    expect(dropModeAt(0.9, modes({ after: false }), EDGE_BAND_HEAVY)).toBeNull()
    expect(dropModeAt(0.1, modes({ before: false }), EDGE_BAND_HEAVY)).toBeNull()
    // Same on the 50/50 split a non-nestable row falls back to.
    expect(dropModeAt(0.1, modes({ into: false, before: false }), EDGE_BAND_HEAVY)).toBeNull()
    expect(dropModeAt(0.9, modes({ into: false, before: false }), EDGE_BAND_HEAVY)).toBe('after')
  })

  it('returns null for a total refusal, wherever the pointer is', () => {
    expect(dropModeAt(0.5, NO_MODES, EDGE_BAND_HEAVY)).toBeNull()
  })
})

describe('dragSubjectsFor', () => {
  const a = { kind: 'note', id: 'a' }
  const b = { kind: 'note', id: 'b' }
  const c = { kind: 'group', id: 'c' }

  it('carries the whole selection when the grabbed row is part of it', () => {
    expect(dragSubjectsFor(b, [a, b])).toEqual([a, b])
  })

  it('hands back a copy, so a drag cannot mutate the selection it read', () => {
    const selection = [a, b]
    expect(dragSubjectsFor(b, selection)).not.toBe(selection)
  })

  it('carries the grabbed row alone when it sits outside the selection', () => {
    expect(dragSubjectsFor(c, [a, b])).toEqual([c])
  })

  it('carries one row when nothing is selected', () => {
    expect(dragSubjectsFor(c, [])).toEqual([c])
  })
})

describe('sameDrop', () => {
  const base = { kind: 'note', id: 'a', mode: 'before' as DropMode }

  it('is true only when the drop would draw and commit the same thing', () => {
    expect(sameDrop(base, { ...base })).toBe(true)
    expect(sameDrop(base, { ...base, id: 'b' })).toBe(false)
    expect(sameDrop(base, { ...base, kind: 'group' })).toBe(false)
    expect(sameDrop(base, { ...base, mode: 'after' })).toBe(false)
  })

  it('treats two nothings as the same nothing', () => {
    expect(sameDrop(null, null)).toBe(true)
    expect(sameDrop(base, null)).toBe(false)
    expect(sameDrop(null, base)).toBe(false)
  })
})
