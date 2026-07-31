/**
 * Contract pins for the drop matrix.
 *
 * Every permitted move here matters, but the refusals matter more: a drag that
 * silently does the wrong thing corrupts either fork lineage or a protected
 * branch's worktree. The rules under test:
 *
 *   - The four movable classes do not mix.
 *   - Workspaces and folders move only within their own repo.
 *   - A protected branch reorders among its own siblings and does nothing else.
 *   - "After" an expanded parent means FIRST CHILD, because that is the gap the
 *     indicator is drawn in — so it counts as a re-parent and anything that may
 *     not re-parent may not do it.
 */
import { describe, expect, test } from 'vitest'
import {
  allowedModes,
  resolveDropMode,
  resolvesToFirstChild,
  edgeBandFor,
  type DragSubject,
  type DropTarget,
} from '@/components/layout/drop-rules'

const sub = (over: Partial<DragSubject> = {}): DragSubject => ({
  kind: 'workspace',
  id: 's1',
  repoId: 'r1',
  parentId: 'p1',
  ...over,
})

const tgt = (over: Partial<DropTarget> = {}): DropTarget => ({
  kind: 'workspace',
  id: 't1',
  repoId: 'r1',
  parentId: 'p1',
  ...over,
})

describe('allowedModes — the matrix', () => {
  test('a workspace onto a workspace in the same repo can do anything', () => {
    expect(allowedModes([sub()], tgt())).toEqual({ before: true, after: true, into: true })
  })

  test('a workspace onto a folder in the same repo can do anything', () => {
    expect(allowedModes([sub()], tgt({ kind: 'folder' }))).toEqual({
      before: true,
      after: true,
      into: true,
    })
  })

  test('a workspace onto its repo header can only be appended at the root', () => {
    expect(allowedModes([sub()], tgt({ kind: 'repo' }))).toEqual({
      before: false,
      after: false,
      into: true,
    })
  })

  test('a repo onto a repo reorders, never nests', () => {
    expect(allowedModes([sub({ kind: 'repo' })], tgt({ kind: 'repo' }))).toEqual({
      before: true,
      after: true,
      into: false,
    })
  })

  test('a repo onto a project moves it into that project', () => {
    expect(allowedModes([sub({ kind: 'repo' })], tgt({ kind: 'project' }))).toEqual({
      before: false,
      after: false,
      into: true,
    })
  })

  test('a project onto a project reorders', () => {
    expect(allowedModes([sub({ kind: 'project' })], tgt({ kind: 'project' }))).toEqual({
      before: true,
      after: true,
      into: false,
    })
  })
})

describe('allowedModes — refusals', () => {
  test('a workspace may not cross repos', () => {
    expect(allowedModes([sub({ repoId: 'r1' })], tgt({ repoId: 'r2' }))).toEqual({
      before: false,
      after: false,
      into: false,
    })
  })

  test('a workspace may not be dropped on a project', () => {
    expect(allowedModes([sub()], tgt({ kind: 'project' })).into).toBe(false)
  })

  test('a project may not be dropped on a repo', () => {
    expect(allowedModes([sub({ kind: 'project' })], tgt({ kind: 'repo' })).before).toBe(false)
  })

  test('a repo may not be dropped on a workspace', () => {
    expect(allowedModes([sub({ kind: 'repo' })], tgt({ kind: 'workspace' })).into).toBe(false)
  })

  test('a row may not be dropped on itself', () => {
    expect(allowedModes([sub({ id: 'same' })], tgt({ id: 'same' })).into).toBe(false)
  })

  test('a mixed selection is refused rather than guessed at', () => {
    const mixed = [sub({ kind: 'workspace' }), sub({ kind: 'repo', id: 's2' })]
    expect(allowedModes(mixed, tgt())).toEqual({ before: false, after: false, into: false })
  })

  test('an empty drag does nothing', () => {
    expect(allowedModes([], tgt()).into).toBe(false)
  })
})

describe('allowedModes — protected branches', () => {
  const locked = sub({ locked: true, parentId: 'develop' })

  test('reorders among its own siblings', () => {
    expect(allowedModes([locked], tgt({ parentId: 'develop' }))).toEqual({
      before: true,
      after: true,
      into: false,
    })
  })

  test('never nests, even in the middle of a sibling row', () => {
    expect(allowedModes([locked], tgt({ parentId: 'develop' })).into).toBe(false)
  })

  test('refuses a target under a different parent, because that is a re-parent', () => {
    expect(allowedModes([locked], tgt({ parentId: 'elsewhere' }))).toEqual({
      before: false,
      after: false,
      into: false,
    })
  })

  test('refuses the repo root, because landing there is a re-parent', () => {
    expect(allowedModes([locked], tgt({ kind: 'repo' })).into).toBe(false)
  })

  test('refuses "after" an expanded sibling, because that means first child', () => {
    const target = tgt({ parentId: 'develop', expanded: true, hasChildren: true })
    expect(allowedModes([locked], target)).toEqual({
      before: true,
      after: false,
      into: false,
    })
  })

  test('allows "after" a collapsed sibling, which is a plain reorder', () => {
    const target = tgt({ parentId: 'develop', expanded: false, hasChildren: true })
    expect(allowedModes([locked], target).after).toBe(true)
  })

  test('one locked row in a multi-selection constrains the whole drag', () => {
    const subjects = [
      sub({ id: 'a', parentId: 'develop' }),
      sub({ id: 'b', locked: true, parentId: 'develop' }),
    ]
    expect(allowedModes(subjects, tgt({ parentId: 'develop' })).into).toBe(false)
  })
})

describe('resolvesToFirstChild', () => {
  test('after an expanded parent with children', () => {
    expect(resolvesToFirstChild(tgt({ expanded: true, hasChildren: true }), 'after')).toBe(true)
  })

  test('not when collapsed', () => {
    expect(resolvesToFirstChild(tgt({ expanded: false, hasChildren: true }), 'after')).toBe(false)
  })

  test('not when childless', () => {
    expect(resolvesToFirstChild(tgt({ expanded: true, hasChildren: false }), 'after')).toBe(false)
  })

  test('never for before or into', () => {
    const t = tgt({ expanded: true, hasChildren: true })
    expect(resolvesToFirstChild(t, 'before')).toBe(false)
    expect(resolvesToFirstChild(t, 'into')).toBe(false)
  })
})

describe('resolveDropMode — thresholds', () => {
  const any = { before: true, after: true, into: true }

  test('a folder reorders in its outer 20% and nests in the middle 60%', () => {
    const folder = tgt({ kind: 'folder' })
    expect(edgeBandFor('folder')).toBe(0.2)
    expect(resolveDropMode(0.1, folder, any)).toBe('before')
    expect(resolveDropMode(0.5, folder, any)).toBe('into')
    expect(resolveDropMode(0.9, folder, any)).toBe('after')
  })

  test('a workspace uses a wider 30% band, because nesting re-parents a fork', () => {
    const workspace = tgt({ kind: 'workspace' })
    expect(edgeBandFor('workspace')).toBe(0.3)
    expect(resolveDropMode(0.25, workspace, any)).toBe('before')
    expect(resolveDropMode(0.5, workspace, any)).toBe('into')
    expect(resolveDropMode(0.75, workspace, any)).toBe('after')
    // 0.25 would have been "into" on a folder row.
    expect(resolveDropMode(0.25, tgt({ kind: 'folder' }), any)).toBe('into')
  })

  test('a row that cannot be nested into splits 50/50', () => {
    const reorderOnly = { before: true, after: true, into: false }
    const repo = tgt({ kind: 'repo' })
    expect(resolveDropMode(0.45, repo, reorderOnly)).toBe('before')
    expect(resolveDropMode(0.55, repo, reorderOnly)).toBe('after')
  })

  test('a row that can only be nested into always nests', () => {
    const intoOnly = { before: false, after: false, into: true }
    expect(resolveDropMode(0.02, tgt({ kind: 'repo' }), intoOnly)).toBe('into')
    expect(resolveDropMode(0.98, tgt({ kind: 'repo' }), intoOnly)).toBe('into')
  })

  test('a refused mode yields null, so no indicator is drawn', () => {
    const none = { before: false, after: false, into: false }
    expect(resolveDropMode(0.5, tgt(), none)).toBeNull()
  })

  test('a locked drag over the bottom of an expanded sibling draws nothing', () => {
    const target = tgt({ parentId: 'develop', expanded: true, hasChildren: true })
    const allowed = allowedModes([sub({ locked: true, parentId: 'develop' })], target)
    expect(resolveDropMode(0.9, target, allowed)).toBeNull()
    // ...but its top edge still reorders.
    expect(resolveDropMode(0.1, target, allowed)).toBe('before')
  })
})
