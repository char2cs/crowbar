/**
 * What a removal decides before anything is hidden.
 *
 * The rule under test throughout is that a hold is not a delete: everything
 * here computes what to HIDE, and every one of those hidings is undone by
 * dropping an id. The three kinds hide different amounts because the daemon
 * removes different amounts — a workspace cascades, a folder reparents its
 * children, and a repo takes every worktree under it.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'

const { getHomeWorkspaceId } = vi.hoisted(() => ({ getHomeWorkspaceId: vi.fn() }))
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({ getHomeWorkspaceId }))

import {
  applyPendingRemovals,
  planRemoval,
  type DragSubject,
} from '@/components/layout/removal-plan'
import type { Repo } from '@/lib/store/sidebar'
import { useHomeTreeStore } from '@/lib/store/home-tree'

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

beforeEach(() => {
  getHomeWorkspaceId.mockReset()
  useHomeTreeStore.setState({ trees: {} })
})

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

// Regression: a project-home chat/folder rides no repo at all, so the repo
// lookup every OTHER chat/folder draft needs found nothing for one and
// `handleTrash` refused it outright (reported live as "Can't delete X
// yet"). `draftFor` now resolves a home row FIRST, via the same
// `resolveHomeRowScope` `handleOpen`/`handleCreate` already check before
// anything repo-scoped.
describe('a project-home chat or folder', () => {
  it('drafts a chat removal scoped to the home workspace, taking its threads with it', () => {
    getHomeWorkspaceId.mockReturnValue('home-ws-1')
    useHomeTreeStore.setState({
      trees: {
        p1: {
          chats: [
            { id: 'c1', repoId: '', title: 'Parent', order: 0 },
            { id: 'c2', repoId: '', title: 'Thread', order: 0, parentId: 'c1' },
          ],
          folders: [],
        },
      },
    })

    const [draft] = planRemoval([{ kind: 'chat', id: 'c1' }], [repo()])

    expect(draft.kind).toBe('chat')
    expect(draft.projectId).toBe('p1')
    expect(draft.repoId).toBe('')
    expect(draft.wsId).toBe('home-ws-1')
    expect([...draft.hiddenIds].sort()).toEqual(['c1', 'c2'])
    expect(draft.extra).toBe(1)
  })

  it('drafts a folder removal scoped to no repo at all, alone', () => {
    getHomeWorkspaceId.mockReturnValue('home-ws-1')
    useHomeTreeStore.setState({
      trees: { p1: { chats: [], folders: [{ id: 'f1', repoId: '', name: 'Notes', order: 0 }] } },
    })

    const [draft] = planRemoval([{ kind: 'folder', id: 'f1' }], [repo()])

    expect(draft.kind).toBe('folder')
    expect(draft.projectId).toBe('p1')
    expect(draft.repoId).toBe('')
    expect(draft.hiddenIds).toEqual(['f1'])
    expect(draft.extra).toBe(0)
  })

  it('is never confused by a repo whose OWN folders array also claims the same id', () => {
    getHomeWorkspaceId.mockReturnValue('home-ws-1')
    useHomeTreeStore.setState({
      trees: { p1: { chats: [], folders: [{ id: 'home-folder-1', repoId: '', name: 'x', order: 0 }] } },
    })
    const bled = repo({ folders: [{ id: 'home-folder-1', repoId: 'r1', name: 'x', order: 0 }] })

    const [draft] = planRemoval([{ kind: 'folder', id: 'home-folder-1' }], [bled])

    // '' (home), never 'r1' — the repo-scoped branch below would have
    // resolved this against the bled-into repo instead.
    expect(draft.repoId).toBe('')
  })
})
