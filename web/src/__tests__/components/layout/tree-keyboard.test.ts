/**
 * What each key means to the tree, decided over rows rather than over the DOM.
 *
 * Kept pure for the same reason the drop matrix is: every rule — and every
 * refusal — is exercised here without focus, layout or a real keyboard, so the
 * hook next door only has to prove that it wires this up.
 */
import { describe, it, expect } from 'vitest'
import { isTypeaheadKey, resolveTreeKey } from '@/components/layout/tree-keyboard'
import type { DropRow } from '@/components/layout/drop-target-dom'

/** A project, a repo, and a subtree — the shape the sidebar actually draws. */
const ROWS: DropRow[] = [
  {
    kind: 'project',
    id: 'p1',
    parentId: '',
    label: 'crowbar-project',
    expanded: true,
    hasChildren: true,
  },
  {
    kind: 'repo',
    id: 'r1',
    repoId: 'r1',
    parentId: 'p1',
    label: 'crowbar',
    expanded: true,
    hasChildren: true,
  },
  {
    kind: 'workspace',
    id: 'a',
    repoId: 'r1',
    parentId: '',
    label: 'alpha',
    expanded: true,
    hasChildren: true,
  },
  {
    kind: 'workspace',
    id: 'kid',
    repoId: 'r1',
    parentId: 'a',
    label: 'apricot',
    hasChildren: false,
  },
  {
    kind: 'folder',
    id: 'f1',
    repoId: 'r1',
    parentId: '',
    label: 'spikes',
    expanded: false,
    hasChildren: true,
  },
  { kind: 'workspace', id: 'b', repoId: 'r1', parentId: '', label: 'beta', hasChildren: false },
]

const key = (
  k: string,
  focusedId: string | null,
  over: Partial<Parameters<typeof resolveTreeKey>[0]> = {},
) =>
  resolveTreeKey({
    key: k,
    modified: false,
    rows: ROWS,
    focusedId,
    prefix: '',
    extendingPrefix: false,
    ...over,
  })

describe('moving through the rows', () => {
  it('walks down and up in the order they are drawn', () => {
    expect(key('ArrowDown', 'a')).toEqual({ kind: 'move', id: 'kid' })
    expect(key('ArrowUp', 'a')).toEqual({ kind: 'move', id: 'r1' })
  })

  it('stops at both ends rather than wrapping', () => {
    expect(key('ArrowUp', 'p1')).toEqual({ kind: 'move', id: 'p1' })
    expect(key('ArrowDown', 'b')).toEqual({ kind: 'move', id: 'b' })
  })

  it('jumps to the ends', () => {
    expect(key('Home', 'a')).toEqual({ kind: 'move', id: 'p1' })
    expect(key('End', 'a')).toEqual({ kind: 'move', id: 'b' })
  })

  it('enters at the top when nothing has focus yet', () => {
    expect(key('ArrowDown', null)).toEqual({ kind: 'move', id: 'p1' })
  })
})

describe('Left and Right', () => {
  it('opens a closed row, and steps into an open one', () => {
    expect(key('ArrowRight', 'f1')).toEqual({ kind: 'expand', id: 'f1' })
    expect(key('ArrowRight', 'a')).toEqual({ kind: 'move', id: 'kid' })
  })

  it('closes an open row, and steps out of a closed one', () => {
    expect(key('ArrowLeft', 'a')).toEqual({ kind: 'collapse', id: 'a' })
    expect(key('ArrowLeft', 'kid')).toEqual({ kind: 'move', id: 'a' })
  })

  // A row at the repo's root publishes an empty container; what it is really
  // under is the repo header, which is a row in this same tree.
  it('steps out of a root row onto its repo, and out of the repo onto its project', () => {
    expect(key('ArrowLeft', 'b')).toEqual({ kind: 'move', id: 'r1' })
    expect(key('ArrowLeft', 'r1')).toEqual({ kind: 'collapse', id: 'r1' })
  })

  it('does nothing at the top of the tree', () => {
    expect(key('ArrowRight', 'b')).toBeNull()
  })
})

describe('type to jump', () => {
  it('lands on the next row whose label starts with the letter', () => {
    expect(key('a', 'p1', { prefix: 'a' })).toEqual({ kind: 'move', id: 'a' })
  })

  // A repeated letter walks the rows that share it rather than sticking.
  it('advances past the row it is already on', () => {
    expect(key('a', 'a', { prefix: 'a' })).toEqual({ kind: 'move', id: 'kid' })
  })

  it('narrows on the row it is on while a word is still being typed', () => {
    expect(key('l', 'a', { prefix: 'al', extendingPrefix: true })).toEqual({
      kind: 'move',
      id: 'a',
    })
  })

  it('wraps rather than giving up at the bottom', () => {
    expect(key('c', 'b', { prefix: 'c' })).toEqual({ kind: 'move', id: 'p1' })
  })

  it('claims nothing when no label matches', () => {
    expect(key('z', 'a', { prefix: 'z' })).toBeNull()
  })

  it('is not a chord, and not the space bar', () => {
    expect(isTypeaheadKey('a', false)).toBe(true)
    expect(isTypeaheadKey('a', true)).toBe(false)
    expect(isTypeaheadKey(' ', false)).toBe(false)
    expect(isTypeaheadKey('ArrowDown', false)).toBe(false)
  })
})

describe('the rest of the contract', () => {
  it('activates on Enter', () => {
    expect(key('Enter', 'a')).toEqual({ kind: 'activate', id: 'a' })
  })

  it('clears the multiselection on Escape', () => {
    expect(key('Escape', 'a')).toEqual({ kind: 'clear-selection' })
  })

  it('removes on Delete and on Backspace', () => {
    expect(key('Delete', 'a')).toEqual({ kind: 'remove' })
    expect(key('Backspace', 'a')).toEqual({ kind: 'remove' })
  })

  it('claims nothing it does not own', () => {
    expect(key('Tab', 'a')).toBeNull()
    expect(key('ArrowDown', 'a', { modified: true })).toBeNull()
    expect(key('ArrowDown', 'a', { rows: [] })).toBeNull()
  })
})
