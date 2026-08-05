/**
 * What the row context menu offers.
 *
 * The menu is where the bulk actions live, so its whole vocabulary is four items
 * — group into a folder, lock, unlock, remove N — and the interesting part is
 * which rows get which, and which get none at all.
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
  it('offers every action on a workspace', () => {
    expect(labels([WS('a')])).toEqual(['Group into a folder', 'Lock workspace', 'Remove workspace'])
  })

  it('names a folder deletion for what it is', () => {
    expect(labels([{ kind: 'folder', id: 'f1', repoId: 'r1' }])).toEqual([
      'Group into a folder',
      'Delete folder',
    ])
  })

  // A folder holds rows and has nothing to protect.
  it('offers a folder no lock', () => {
    expect(labels([{ kind: 'folder', id: 'f1', repoId: 'r1' }])).not.toContain('Lock workspace')
  })

  // A repo is its own level; there is no folder for one to go into, and its
  // header row IS the repo's own checkout, which must stay locked.
  it('offers a repo removal alone', () => {
    expect(labels([{ kind: 'repo', id: 'r1' }])).toEqual(['Remove repository'])
  })

  it('offers a locked branch the unlock, but never the removal', () => {
    // Unlock is the whole point of the override: a protected branch used to be
    // locked with no way to disagree. It still cannot be REMOVED while locked —
    // the daemon refuses that — so the removal stays withheld.
    expect(labels([WS('root', { locked: true })])).toEqual([
      'Group into a folder',
      'Unlock workspace',
    ])
  })

  it('offers a project its deletion', () => {
    // It used to offer nothing at all, so no menu opened on a project row and
    // there was no way to delete one.
    expect(labels([{ kind: 'project', id: 'p1' }])).toEqual(['Delete project'])
    expect(rowMenuFor([])).toEqual([])
  })
})

describe('several rows', () => {
  it('counts them', () => {
    expect(labels([WS('a'), WS('b')])).toEqual([
      'Group 2 into a folder',
      'Lock 2 workspaces',
      'Remove 2 workspaces',
    ])
  })

  it('says rows when they are not all workspaces', () => {
    expect(labels([WS('a'), { kind: 'folder', id: 'f1', repoId: 'r1' }])).toContain('Remove 2 rows')
  })

  it('offers BOTH verbs on a mixed selection', () => {
    // Each acts on the whole selection, which is the honest offer: either verb
    // genuinely applies to part of it.
    expect(labels([WS('a'), WS('root', { locked: true })])).toEqual([
      'Group 2 into a folder',
      'Lock 2 workspaces',
      'Unlock 2 workspaces',
    ])
  })

  it('withholds the removal if any of them is locked', () => {
    expect(labels([WS('a'), WS('root', { locked: true })])).not.toContain('Remove 2 workspaces')
  })

  it('opens nothing when a project is mixed in with rows from inside it', () => {
    // "Remove this project and also this branch within it" is not a coherent
    // action to offer.
    expect(rowMenuFor([{ kind: 'project', id: 'p1' }, WS('a')])).toEqual([])
  })
})
