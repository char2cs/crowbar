/**
 * What a removal decides before anything is hidden.
 *
 * The rule under test throughout is that a hold is not a delete: everything
 * here computes what to HIDE, and every one of those hidings is undone by
 * dropping an id. The three kinds hide different amounts because the daemon
 * removes different amounts — a workspace cascades, a folder reparents its
 * children, and a repo takes every worktree under it.
 */
import { describe, it, expect } from 'vitest'
import { applyPendingRemovals, planRemoval } from '@/components/layout/removal-plan'
import type { Repo } from '@/lib/store/sidebar'
import type { DragSubject } from '@/components/layout/drop-rules'

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  workspaces: [
    { id: 'root', branch: 'develop', status: 'locked', age: '' },
    { id: 'a', branch: 'alpha', status: 'new', age: '' },
    { id: 'kid', branch: 'alpha/one', parentId: 'a', status: 'new', age: '' },
    { id: 'grandkid', branch: 'alpha/two', parentId: 'kid', status: 'new', age: '' },
    { id: 'b', branch: 'beta', status: 'new', age: '' },
  ],
  folders: [{ id: 'f1', repoId: 'r1', name: 'spikes', order: 0 }],
  ...over,
})

const WS = (id: string): DragSubject => ({ kind: 'workspace', id, repoId: 'r1' })

describe('what a removal takes with it', () => {
  it('takes a workspace and its whole subtree — the delete cascades', () => {
    const [draft] = planRemoval([WS('a')], [repo()])

    expect(draft.kind).toBe('workspace')
    expect(draft.label).toBe('alpha')
    expect([...draft.hiddenIds].sort()).toEqual(['a', 'grandkid', 'kid'])
    expect(draft.extra).toBe(2)
  })

  // A folder holds no worktree, and the daemon reparents its children to the
  // folder's own parent. Hiding them would promise a deletion that is not
  // going to happen.
  it('takes a folder alone, and says so with no count', () => {
    const [draft] = planRemoval([{ kind: 'folder', id: 'f1', repoId: 'r1' }], [repo()])

    expect(draft.kind).toBe('folder')
    expect(draft.hiddenIds).toEqual(['f1'])
    expect(draft.extra).toBe(0)
  })

  it('takes a repo and counts the worktrees that go with it', () => {
    const [draft] = planRemoval([{ kind: 'repo', id: 'r1' }], [repo()])

    expect(draft.kind).toBe('repo')
    expect(draft.hiddenIds).toEqual(['r1'])
    expect(draft.extra).toBe(5)
  })

  it('refuses a protected branch — the daemon would refuse the delete', () => {
    expect(planRemoval([WS('root')], [repo()])).toEqual([])
  })

  it('refuses a project outright', () => {
    expect(planRemoval([{ kind: 'project', id: 'p1' }], [repo()])).toEqual([])
  })

  // Two rows of one subtree are one disappearance, so they are one tray row.
  it('drops a row that is already inside another row it is taking', () => {
    const drafts = planRemoval([WS('a'), WS('kid')], [repo()])

    expect(drafts.map((d) => d.id)).toEqual(['a'])
  })

  it('resolves where to go BEFORE the row is hidden', () => {
    const [draft] = planRemoval([WS('kid')], [repo()])

    expect(draft.fallbackWsId).toBe('a')
  })
})

describe('the sidebar as it reads with rows held', () => {
  it('hands back the same repos when nothing is held', () => {
    const repos = [repo()]

    expect(applyPendingRemovals(repos, new Set())).toBe(repos)
  })

  it('takes the held workspaces out', () => {
    const out = applyPendingRemovals([repo()], new Set(['a', 'kid', 'grandkid']))

    expect(out[0].workspaces.map((w) => w.id)).toEqual(['root', 'b'])
  })

  it('takes a held repo out whole', () => {
    expect(applyPendingRemovals([repo()], new Set(['r1']))).toEqual([])
  })

  // The commit reparents; the preview has to show the same thing, or the tray
  // is showing one outcome and delivering another.
  it("moves a held folder's children up to the folder's own parent", () => {
    const withFolders = repo({
      folders: [
        { id: 'outer', repoId: 'r1', name: 'outer', order: 0 },
        { id: 'inner', repoId: 'r1', parentId: 'outer', name: 'inner', order: 0 },
      ],
      workspaces: [{ id: 'a', branch: 'alpha', folderId: 'inner', status: 'new', age: '' }],
    })

    const out = applyPendingRemovals([withFolders], new Set(['inner']))

    expect(out[0].folders?.map((f) => f.id)).toEqual(['outer'])
    expect(out[0].workspaces[0].folderId).toBe('outer')
  })

  it('walks past a held ancestor to the outermost folder still on screen', () => {
    const withFolders = repo({
      folders: [
        { id: 'outer', repoId: 'r1', name: 'outer', order: 0 },
        { id: 'inner', repoId: 'r1', parentId: 'outer', name: 'inner', order: 0 },
      ],
      workspaces: [{ id: 'a', branch: 'alpha', folderId: 'inner', status: 'new', age: '' }],
    })

    const out = applyPendingRemovals([withFolders], new Set(['inner', 'outer']))

    expect(out[0].folders).toEqual([])
    expect(out[0].workspaces[0].folderId).toBe('')
  })
})
