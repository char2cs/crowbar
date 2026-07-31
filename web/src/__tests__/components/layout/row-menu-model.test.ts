/**
 * What the row context menu offers.
 *
 * The menu is where the bulk actions live now, so its whole vocabulary is two
 * items — group into a folder, remove N — and the interesting part is which
 * rows get which, and which get none at all.
 */
import { describe, it, expect } from 'vitest'
import { rowMenuFor } from '@/components/layout/row-menu-model'
import type { DragSubject } from '@/components/layout/drop-rules'

const WS = (id: string, over: Partial<DragSubject> = {}): DragSubject => ({
  kind: 'workspace',
  id,
  repoId: 'r1',
  ...over,
})

const labels = (subjects: DragSubject[]) => rowMenuFor(subjects).map((e) => e.label)

describe('one row', () => {
  it('offers both actions on a workspace', () => {
    expect(labels([WS('a')])).toEqual(['Group into a folder', 'Remove workspace'])
  })

  it('names a folder deletion for what it is', () => {
    expect(labels([{ kind: 'folder', id: 'f1', repoId: 'r1' }])).toEqual([
      'Group into a folder',
      'Delete folder',
    ])
  })

  // A repo is its own level; there is no folder for one to go into.
  it('offers a repo removal alone', () => {
    expect(labels([{ kind: 'repo', id: 'r1' }])).toEqual(['Remove repository'])
  })

  it('offers a protected branch the folder, but never the removal', () => {
    expect(labels([WS('root', { locked: true })])).toEqual(['Group into a folder'])
  })

  it('offers a project nothing, so no menu opens at all', () => {
    expect(rowMenuFor([{ kind: 'project', id: 'p1' }])).toEqual([])
    expect(rowMenuFor([])).toEqual([])
  })
})

describe('several rows', () => {
  it('counts them', () => {
    expect(labels([WS('a'), WS('b')])).toEqual(['Group 2 into a folder', 'Remove 2 workspaces'])
  })

  it('says rows when they are not all workspaces', () => {
    expect(labels([WS('a'), { kind: 'folder', id: 'f1', repoId: 'r1' }])[1]).toBe('Remove 2 rows')
  })

  it('withholds the removal if any of them is protected', () => {
    expect(labels([WS('a'), WS('root', { locked: true })])).toEqual(['Group 2 into a folder'])
  })
})
