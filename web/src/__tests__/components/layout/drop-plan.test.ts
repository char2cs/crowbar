/**
 * What a drop MEANS: which requests it fires and what it paints before they
 * land.
 *
 * The two things worth pinning are the ones a drag cannot show you: that
 * `order` is the index the daemon expects (the destination list with the moved
 * row lifted out), and that a move renumbers the siblings it displaced — a
 * level that renumbers a frame late makes the dropped row visibly jump twice.
 */
import { describe, it, expect } from 'vitest'
import { planDrop, type PlacementSnapshot } from '@/components/layout/drop-plan'
import { capturePlacement, type Repo } from '@/lib/store/sidebar'
import type { ResolvedDrop } from '@/components/layout/drop-target-dom'
import type { DragSubject } from '@/components/layout/drop-rules'

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  workspaces: [
    { id: 'a', branch: 'a', age: '', order: 0 },
    { id: 'b', branch: 'b', age: '', order: 1 },
    { id: 'c', branch: 'c', age: '', order: 2 },
  ],
  ...over,
})

const snap = (over: Partial<PlacementSnapshot> = {}): PlacementSnapshot => ({
  repos: [repo()],
  projectIds: ['p1'],
  ...over,
})

const WS = (id: string, over: Partial<DragSubject> = {}): DragSubject => ({
  kind: 'workspace',
  id,
  repoId: 'r1',
  parentId: '',
  ...over,
})

const target = (
  over: Partial<ResolvedDrop> & { id: string; mode: ResolvedDrop['mode'] },
): ResolvedDrop => ({
  kind: 'workspace',
  repoId: 'r1',
  parentId: '',
  ...over,
})

describe('reordering within a level', () => {
  it('sends the index the destination list holds with the moved row lifted out', () => {
    // a b c, moving c before b → rest is [a, b], so c lands at 1.
    const plan = planDrop([WS('c')], target({ id: 'b', mode: 'before' }), snap())

    expect(plan?.calls).toEqual([
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'c', folderId: '', order: 1 },
    ])
  })

  it('lands after the target when the pointer is in its lower band', () => {
    const plan = planDrop([WS('a')], target({ id: 'b', mode: 'after' }), snap())

    expect(plan?.calls).toEqual([
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'a', folderId: '', order: 1 },
    ])
  })

  it('renumbers every sibling the move displaced, not just the row that moved', () => {
    const plan = planDrop([WS('c')], target({ id: 'a', mode: 'before' }), snap())

    expect(plan?.writes.workspaces).toEqual([
      { id: 'c', folderId: '', order: 0 },
      { id: 'a', order: 1 },
      { id: 'b', order: 2 },
    ])
  })
})

describe('the gap under an expanded parent', () => {
  const nested = repo({
    workspaces: [
      { id: 'a', branch: 'a', age: '', order: 0 },
      { id: 'kid', branch: 'kid', parentId: 'a', age: '', order: 0 },
      { id: 'b', branch: 'b', age: '', order: 1 },
    ],
  })

  it("makes the row that parent's FIRST CHILD, which is where the line was drawn", () => {
    const plan = planDrop(
      [WS('b')],
      target({ id: 'a', mode: 'after', expanded: true, hasChildren: true }),
      snap({ repos: [nested] }),
    )

    expect(plan?.calls).toEqual([
      { kind: 'reparent', projectId: 'p1', repoId: 'r1', id: 'b', parentId: 'a' },
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'b', order: 0 },
    ])
  })

  // The indicator promised a slot, not just a depth. The re-parent is 202 + an
  // async rebase, so the slot follows as its own call rather than riding along.
  it('sends the slot it promised as a second call, not as an append', () => {
    const twoKids = repo({
      workspaces: [
        { id: 'a', branch: 'a', age: '', order: 0 },
        { id: 'kid1', branch: 'kid1', parentId: 'a', age: '', order: 0 },
        { id: 'kid2', branch: 'kid2', parentId: 'a', age: '', order: 1 },
        { id: 'b', branch: 'b', age: '', order: 1 },
      ],
    })
    const plan = planDrop(
      [WS('b')],
      target({ id: 'kid2', mode: 'before', parentId: 'a' }),
      snap({ repos: [twoKids] }),
    )

    expect(plan?.calls).toEqual([
      { kind: 'reparent', projectId: 'p1', repoId: 'r1', id: 'b', parentId: 'a' },
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'b', order: 1 },
    ])
  })

  it('is an ordinary "after" once the parent is collapsed', () => {
    const plan = planDrop(
      [WS('c')],
      target({ id: 'a', mode: 'after', expanded: false, hasChildren: true }),
      snap(),
    )

    expect(plan?.calls).toEqual([
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'c', folderId: '', order: 1 },
    ])
  })
})

describe('nesting', () => {
  it('re-parents a fork when a workspace lands inside a workspace', () => {
    const plan = planDrop([WS('c')], target({ id: 'a', mode: 'into' }), snap())

    expect(plan?.calls).toEqual([
      { kind: 'reparent', projectId: 'p1', repoId: 'r1', id: 'c', parentId: 'a' },
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'c', order: 0 },
    ])
  })

  it('files a row when a workspace lands inside a FOLDER — a different call entirely', () => {
    const withFolder = repo({
      folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }],
    })
    const plan = planDrop(
      [WS('c')],
      target({ kind: 'folder', id: 'f1', mode: 'into' }),
      snap({ repos: [withFolder] }),
    )

    expect(plan?.calls).toEqual([
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'c', folderId: 'f1', order: 0 },
    ])
  })

  it('files a fork child into a folder under the same fork parent without rebasing it', () => {
    const withFolder = repo({
      folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', parentId: 'a', order: 0 }],
      workspaces: [
        { id: 'a', branch: 'a', age: '', order: 0 },
        { id: 'c', branch: 'c', parentId: 'a', age: '', order: 1 },
      ],
    })
    const plan = planDrop(
      [WS('c', { parentId: 'a' })],
      target({ kind: 'folder', id: 'f1', parentId: 'a', mode: 'into' }),
      snap({ repos: [withFolder] }),
    )

    expect(plan?.calls).toEqual([
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'c', folderId: 'f1', order: 0 },
    ])
  })

  it('unfiles a fork child when it lands directly beside its fork siblings', () => {
    const withFolder = repo({
      folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', parentId: 'a', order: 0 }],
      workspaces: [
        { id: 'a', branch: 'a', age: '', order: 0 },
        { id: 'b', branch: 'b', parentId: 'a', age: '', order: 1 },
        { id: 'c', branch: 'c', parentId: 'a', folderId: 'f1', age: '', order: 0 },
      ],
    })
    const plan = planDrop(
      [WS('c', { parentId: 'f1' })],
      target({ id: 'b', parentId: 'a', mode: 'after' }),
      snap({ repos: [withFolder] }),
    )

    expect(plan?.calls).toEqual([
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'c', folderId: '', order: 2 },
    ])
  })

  it('reparents first when a folder belongs to a different workspace', () => {
    const withFolder = repo({
      folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', parentId: 'b', order: 0 }],
      workspaces: [
        { id: 'a', branch: 'a', age: '', order: 0 },
        { id: 'b', branch: 'b', age: '', order: 1 },
        { id: 'c', branch: 'c', parentId: 'a', age: '', order: 0 },
      ],
    })
    const plan = planDrop(
      [WS('c', { parentId: 'a' })],
      target({ kind: 'folder', id: 'f1', parentId: 'b', mode: 'into' }),
      snap({ repos: [withFolder] }),
    )

    expect(plan?.calls).toEqual([
      { kind: 'reparent', projectId: 'p1', repoId: 'r1', id: 'c', parentId: 'b' },
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'c', folderId: 'f1', order: 0 },
    ])
  })

  it('moves a folder by its own endpoint, never by a fork re-parent', () => {
    const withFolder = repo({
      folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }],
    })
    const plan = planDrop(
      [{ kind: 'folder', id: 'f1', repoId: 'r1', parentId: '' }],
      target({ id: 'a', mode: 'into' }),
      snap({ repos: [withFolder] }),
    )

    expect(plan?.calls).toEqual([
      { kind: 'folder', projectId: 'p1', repoId: 'r1', id: 'f1', parentId: 'a', order: 0 },
    ])
  })

  it('clears the folder when a row lands on the repo header', () => {
    const filed = repo({
      folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }],
      workspaces: [{ id: 'a', branch: 'a', folderId: 'f1', age: '', order: 0 }],
    })
    const plan = planDrop(
      [WS('a')],
      target({ kind: 'repo', id: 'r1', mode: 'into' }),
      snap({ repos: [filed] }),
    )

    expect(plan?.calls).toEqual([
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'a', folderId: '', order: 1 },
    ])
  })

  it('lifts a forked child back to the root by re-parenting it onto repo home', () => {
    const forked = repo({
      workspaces: [
        { id: 'a', branch: 'a', age: '', order: 0 },
        { id: 'kid', branch: 'kid', parentId: 'a', age: '', order: 0 },
      ],
    })
    const plan = planDrop(
      [WS('kid', { parentId: 'a' })],
      target({ kind: 'repo', id: 'r1', mode: 'into' }),
      snap({ repos: [forked] }),
    )

    expect(plan?.calls).toEqual([
      { kind: 'reparent', projectId: 'p1', repoId: 'r1', id: 'kid', parentId: 'w-default' },
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'kid', order: 1 },
    ])
  })
})

describe('several rows at once', () => {
  it('gives each subject the next index, so a sequential send lands them in order', () => {
    const plan = planDrop([WS('b'), WS('c')], target({ id: 'a', mode: 'before' }), snap())

    expect(plan?.calls).toEqual([
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'b', folderId: '', order: 0 },
      { kind: 'workspace', projectId: 'p1', repoId: 'r1', id: 'c', folderId: '', order: 1 },
    ])
    expect(plan?.writes.workspaces).toEqual([
      { id: 'b', folderId: '', order: 0 },
      { id: 'c', folderId: '', order: 1 },
      { id: 'a', order: 2 },
    ])
  })

  it('captures the whole move as one undo, so a refusal snaps it back as a unit', () => {
    const before = [repo()]
    const plan = planDrop([WS('b'), WS('c')], target({ id: 'a', mode: 'before' }), snap())!

    expect(capturePlacement(before, plan.writes).workspaces).toEqual([
      { id: 'a', order: 0 },
      { id: 'b', folderId: '', order: 1 },
      { id: 'c', folderId: '', order: 2 },
    ])
  })
})

describe('repos', () => {
  const two = snap({
    repos: [repo({ id: 'r1' }), repo({ id: 'r2' }), repo({ id: 'r3', projectId: 'p2' })],
    projectIds: ['p1', 'p2'],
  })

  it('reorders within a project', () => {
    const plan = planDrop(
      [{ kind: 'repo', id: 'r2', parentId: 'p1' }],
      { kind: 'repo', id: 'r1', repoId: 'r1', parentId: 'p1', mode: 'before' },
      two,
    )

    expect(plan?.calls).toEqual([
      { kind: 'repo', id: 'r2', fromProjectId: 'p1', projectId: 'p1', order: 0 },
    ])
    expect(plan?.writes.repos).toEqual([
      { id: 'r2', projectId: 'p1', order: 0 },
      { id: 'r1', projectId: 'p1', order: 1 },
    ])
  })

  it('moves between projects, addressed under the project it is in today', () => {
    const plan = planDrop(
      [{ kind: 'repo', id: 'r1', parentId: 'p1' }],
      { kind: 'project', id: 'p2', parentId: '', mode: 'into' },
      two,
    )

    expect(plan?.calls).toEqual([
      { kind: 'repo', id: 'r1', fromProjectId: 'p1', projectId: 'p2', order: 1 },
    ])
    expect(plan?.writes.repos).toEqual([
      { id: 'r3', projectId: 'p2', order: 0 },
      { id: 'r1', projectId: 'p2', order: 1 },
    ])
  })
})

describe('projects', () => {
  const three = snap({ projectIds: ['p1', 'p2', 'p3'] })

  it('reorders by index and paints the new list', () => {
    const plan = planDrop(
      [{ kind: 'project', id: 'p3' }],
      { kind: 'project', id: 'p1', parentId: '', mode: 'before' },
      three,
    )

    expect(plan?.calls).toEqual([{ kind: 'project', id: 'p3', order: 0 }])
    expect(plan?.projectOrder).toEqual(['p3', 'p1', 'p2'])
  })

  it('has nothing to plan for a project dropped inside another', () => {
    expect(
      planDrop(
        [{ kind: 'project', id: 'p3' }],
        { kind: 'project', id: 'p1', parentId: '', mode: 'into' },
        three,
      ),
    ).toBeNull()
  })
})

describe('nothing to do', () => {
  it('plans nothing without subjects', () => {
    expect(planDrop([], target({ id: 'a', mode: 'before' }), snap())).toBeNull()
  })

  it('plans nothing for a repo the tree does not hold', () => {
    expect(
      planDrop([WS('a')], target({ id: 'a', mode: 'before', repoId: 'gone' }), snap()),
    ).toBeNull()
  })
})
