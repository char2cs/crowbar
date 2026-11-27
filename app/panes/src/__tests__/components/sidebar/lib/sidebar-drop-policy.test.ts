import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SIDEBAR_DROP_POLICY } from '@/components/sidebar/lib/sidebar-drop-policy'
import {
  ALL_MODES,
  EDGE_BAND_CONTAINER,
  EDGE_BAND_HEAVY,
  NO_MODES,
  REORDER_MODES,
} from '@/components/tree-dnd/drop-core'
import { resetChatStore, setChatWorking } from '@/__tests__/__fixtures__/agent-chat'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { resetOperations } from '@/__tests__/__fixtures__/operations'
import { runOperation, type OperationReply } from '@/core/entities/run-operation'
import { resetEntities, seedHomeTree, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'
import { readSelect, selectRepos } from '@/core/entities/select'

const { getHomeWorkspaceId } = vi.hoisted(() => ({ getHomeWorkspaceId: vi.fn() }))
vi.mock('@/features/workspace/lib/home-workspace-resolver', () => ({
  getHomeWorkspaceId,
  getHomeOwningChatId: () => null,
}))

function recordOwnerOfWs1(chatId: string): void {
  seedRepos(
    readSelect(selectRepos).map((r) =>
      r.id === 'repo-1'
        ? {
            ...r,
            workspaces: r.workspaces.map((w) =>
              w.id === 'ws-1' ? { ...w, owningChatId: chatId } : w,
            ),
          }
        : r,
    ),
  )
}

function makeRow(over: Partial<SidebarRow> & { id: string }): SidebarRow {
  return {
    kind: 'branch',
    parentId: null,
    order: 0,
    label: over.id,
    ownsWorktree: true,
    workspaceId: over.id,
    working: false,
    hasView: false,
    ...over,
  }
}

describe('SIDEBAR_DROP_POLICY', () => {
  beforeEach(() => {
    resetChatStore()
    resetOperations()
    vi.clearAllMocks()
    resetEntities()
    seedRepos([
      {
        id: 'repo-1',
        projectId: 'proj-1',
        name: 'repo-1',
        avatarLabel: 'A',
        avatarColor: 'bg-indigo-700',
        defaultWorkspaceId: 'home-1',
        workspaces: [
          { id: 'ws-1', branch: 'feature-a', age: '' },
          { id: 'ws-locked', branch: 'develop', age: '', status: 'locked' },
        ],
        folders: [{ id: 'folder-1', repoId: 'repo-1', name: 'Bugs', order: 0 }],
      },
      {
        id: 'repo-2',
        projectId: 'proj-1',
        name: 'repo-2',
        avatarLabel: 'B',
        avatarColor: 'bg-indigo-700',
        defaultWorkspaceId: 'home-2',
        workspaces: [{ id: 'ws-2', branch: 'feature-b', age: '' }],
      },
      {
        id: 'repo-3',
        projectId: 'proj-2',
        name: 'repo-3',
        avatarLabel: 'C',
        avatarColor: 'bg-indigo-700',
        defaultWorkspaceId: 'home-3',
        workspaces: [],
      },
    ])
  })

  it('a working row allows no drop mode', () => {
    const subject = makeRow({ id: 'ws-1', working: true })
    const target = makeRow({ id: 'home-1' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(NO_MODES)
  })

  it('a workspace whose move is still running allows no second drop', async () => {
    const subject = makeRow({ id: 'ws-1' })
    const target = makeRow({ id: 'home-1' })
    let land!: (reply: OperationReply) => void
    const start = { kind: 'workspace.reparent', projectId: 'proj-1', label: 'Move' }
    runOperation({ ...start, subject: { kind: 'workspace', id: 'ws-1' } }, () => {
      return new Promise<OperationReply>((resolve) => (land = resolve))
    })
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(NO_MODES)

    land({ items: [] })
    await vi.waitFor(() =>
      expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).not.toEqual(NO_MODES),
    )
  })

  it('cross-project drag is refused', () => {
    const subject = makeRow({ id: 'ws-1' })
    const target = makeRow({ id: 'home-3' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(NO_MODES)
  })

  it('cross-repo is legal only for a row owning no worktree', () => {
    const subjectNoWorktree = makeRow({ id: 'ws-1', ownsWorktree: false })
    const subjectWithWorktree = makeRow({ id: 'ws-1', ownsWorktree: true })
    const target = makeRow({ id: 'ws-2' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([subjectNoWorktree], target)).not.toEqual(NO_MODES)
    expect(SIDEBAR_DROP_POLICY.allowedModes([subjectWithWorktree], target)).toEqual(NO_MODES)
  })

  it('a same-repo drag is allowed in full', () => {
    recordOwnerOfWs1('chat-ws-1')
    const subject = makeRow({ id: 'chat-ws-1', workspaceId: 'ws-1' })
    const target = makeRow({ id: 'home-1' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(ALL_MODES)
  })

  it('refuses to drop a row onto itself', () => {
    const row = makeRow({ id: 'ws-1' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([row], row)).toEqual(NO_MODES)
  })

  it('allows a repo folder to reorder/file relative to a plain CHAT in the same repo', () => {
    seedRepos(
      readSelect(selectRepos).map((r) =>
        r.id === 'repo-1'
          ? { ...r, chats: [{ id: 'chat-1', repoId: 'repo-1', title: 'testing', order: 1 }] }
          : r,
      ),
    )
    const folder = makeRow({ id: 'folder-1', kind: 'folder', workspaceId: null })
    const chat = makeRow({ id: 'chat-1', kind: 'chat', workspaceId: null })
    expect(SIDEBAR_DROP_POLICY.allowedModes([folder], chat)).toEqual(ALL_MODES)
  })

  describe('a folder may not cross into a different branch’s own context', () => {
    beforeEach(() => {
      seedRepos(
        readSelect(selectRepos).map((r) =>
          r.id === 'repo-1'
            ? {
                ...r,
                folders: [
                  ...(r.folders ?? []),
                  {
                    id: 'folder-in-ws-1',
                    repoId: 'repo-1',
                    name: 'Notes',
                    parentId: 'ws-1',
                    order: 0,
                  },
                  {
                    id: 'folder-in-locked',
                    repoId: 'repo-1',
                    name: 'Fixes',
                    parentId: 'ws-locked',
                    order: 0,
                  },
                ],
              }
            : r,
        ),
      )
    })

    it('refuses a folder filed under one branch dropped onto a folder filed under a DIFFERENT branch', () => {
      const subject = makeRow({
        id: 'folder-in-ws-1',
        kind: 'folder',
        workspaceId: null,
        parentId: 'ws-1',
      })
      const target = makeRow({
        id: 'folder-in-locked',
        kind: 'folder',
        workspaceId: null,
        parentId: 'ws-locked',
      })
      expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(NO_MODES)
    })

    it('refuses nesting a branch-anchored folder INTO a different branch directly', () => {
      const subject = makeRow({
        id: 'folder-in-ws-1',
        kind: 'folder',
        workspaceId: null,
        parentId: 'ws-1',
      })
      const target = makeRow({ id: 'ws-locked' })
      expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(NO_MODES)
    })

    it('allows reordering a folder among siblings that share its own branch context', () => {
      seedRepos(
        readSelect(selectRepos).map((r) =>
          r.id === 'repo-1'
            ? {
                ...r,
                folders: [
                  ...(r.folders ?? []),
                  {
                    id: 'folder-2-in-ws-1',
                    repoId: 'repo-1',
                    name: 'More',
                    parentId: 'ws-1',
                    order: 1,
                  },
                ],
              }
            : r,
        ),
      )
      const subject = makeRow({
        id: 'folder-in-ws-1',
        kind: 'folder',
        workspaceId: null,
        parentId: 'ws-1',
      })
      const target = makeRow({
        id: 'folder-2-in-ws-1',
        kind: 'folder',
        workspaceId: null,
        parentId: 'ws-1',
      })
      expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(ALL_MODES)
    })
  })

  it('refuses a mixed-kind selection', () => {
    const subjects = [
      makeRow({ id: 'ws-1', kind: 'branch' }),
      makeRow({ id: 'folder-1', kind: 'folder', workspaceId: null }),
    ]
    const target = makeRow({ id: 'home-1' })
    expect(SIDEBAR_DROP_POLICY.allowedModes(subjects, target)).toEqual(NO_MODES)
  })

  it('refuses when a subject cannot be resolved against the live store', () => {
    const subject = makeRow({ id: 'ghost-row' })
    const target = makeRow({ id: 'ws-2' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(NO_MODES)
  })

  it('refuses when the target cannot be resolved against the live store', () => {
    const subject = makeRow({ id: 'ws-1' })
    const target = makeRow({ id: 'ghost-target' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(NO_MODES)
  })

  it('an empty selection has nothing to drop', () => {
    const target = makeRow({ id: 'ws-2' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([], target)).toEqual(NO_MODES)
  })

  it('a locked (protected-branch) row reorders among its own siblings but cannot re-parent', () => {
    const subject = makeRow({ id: 'ws-locked', workspaceId: 'ws-locked', parentId: 'home-1' })
    const sibling = makeRow({ id: 'ws-1', workspaceId: 'ws-1', parentId: 'home-1' })
    seedRepos(
      readSelect(selectRepos).map((r) =>
        r.id === 'repo-1'
          ? {
              ...r,
              folders: [
                ...(r.folders ?? []),
                {
                  id: 'folder-nested',
                  repoId: 'repo-1',
                  name: 'Nested',
                  parentId: 'ws-1',
                  order: 0,
                },
              ],
            }
          : r,
      ),
    )
    const otherLineage = makeRow({
      id: 'folder-nested',
      kind: 'folder',
      workspaceId: null,
      parentId: 'ws-1',
    })

    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], sibling)).toEqual(REORDER_MODES)
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], otherLineage)).toEqual(NO_MODES)
  })

  it('a locked row may be filed into ANY same-anchor folder, not just its current neighbour', () => {
    const subject = makeRow({ id: 'ws-locked', workspaceId: 'ws-locked', parentId: 'home-1' })
    const sameAnchorFolder = makeRow({
      id: 'folder-1',
      kind: 'folder',
      workspaceId: null,
      parentId: null,
    })

    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], sameAnchorFolder)).toEqual(ALL_MODES)
  })

  it('a locked row may still be filed INTO a folder sitting right beside it — no lineage change', () => {
    const subject = makeRow({ id: 'ws-locked', workspaceId: 'ws-locked', parentId: 'home-1' })
    const siblingFolder = makeRow({
      id: 'folder-1',
      kind: 'folder',
      workspaceId: null,
      parentId: 'home-1',
    })

    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], siblingFolder)).toEqual(ALL_MODES)
  })

  it('a locked row never gets "after" on an already-expanded target (that slot re-parents)', () => {
    const subject = makeRow({ id: 'ws-locked', workspaceId: 'ws-locked', parentId: 'home-1' })
    const sibling = makeRow({
      id: 'ws-1',
      workspaceId: 'ws-1',
      parentId: 'home-1',
    }) as SidebarRow & { expanded?: boolean; hasChildren?: boolean }
    const expandedSibling = { ...sibling, expanded: true, hasChildren: true }

    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], expandedSibling)).toEqual({
      before: true,
      after: false,
      into: false,
    })
  })

  it('a fork with no owning chat recorded reorders among its siblings but cannot re-parent', () => {
    const subject = makeRow({ id: 'ws-1', workspaceId: 'ws-1', parentId: 'home-1' })
    const sibling = makeRow({ id: 'ws-locked', workspaceId: 'ws-locked', parentId: 'home-1' })
    seedRepos(
      readSelect(selectRepos).map((r) =>
        r.id === 'repo-1'
          ? {
              ...r,
              folders: [
                ...(r.folders ?? []),
                {
                  id: 'folder-nested',
                  repoId: 'repo-1',
                  name: 'Nested',
                  parentId: 'ws-locked',
                  order: 0,
                },
              ],
            }
          : r,
      ),
    )
    const otherLineage = makeRow({
      id: 'folder-nested',
      kind: 'folder',
      workspaceId: null,
      parentId: 'ws-locked',
    })

    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], sibling)).toEqual(REORDER_MODES)
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], otherLineage)).toEqual(NO_MODES)
  })

  it('a fork with no owning chat recorded may be filed into ANY same-anchor folder', () => {
    const subject = makeRow({ id: 'ws-1', workspaceId: 'ws-1', parentId: 'home-1' })
    const sameAnchorFolder = makeRow({
      id: 'folder-1',
      kind: 'folder',
      workspaceId: null,
      parentId: null,
    })

    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], sameAnchorFolder)).toEqual(ALL_MODES)
  })

  it('a fork with no owning chat recorded may still be filed INTO a folder sitting right beside it', () => {
    const subject = makeRow({ id: 'ws-1', workspaceId: 'ws-1', parentId: 'home-1' })
    const siblingFolder = makeRow({
      id: 'folder-1',
      kind: 'folder',
      workspaceId: null,
      parentId: 'home-1',
    })

    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], siblingFolder)).toEqual(ALL_MODES)
  })

  it('a fork whose owning chat IS recorded may still re-parent', () => {
    recordOwnerOfWs1('chat-ws-1')
    const subject = makeRow({ id: 'chat-ws-1', workspaceId: 'ws-1', parentId: 'home-1' })
    const target = makeRow({ id: 'ws-locked', workspaceId: 'ws-locked', parentId: 'home-1' })

    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(ALL_MODES)
  })

  it('a folder row resolves through the folders array, not workspaceId', () => {
    const subject = makeRow({ id: 'folder-1', kind: 'folder', workspaceId: null })
    const target = makeRow({ id: 'ws-1' })
    expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(REORDER_MODES)
  })

  it('gives a folder target the container band and every other kind the heavy one', () => {
    expect(SIDEBAR_DROP_POLICY.edgeBandFor('folder')).toBe(EDGE_BAND_CONTAINER)
    expect(SIDEBAR_DROP_POLICY.edgeBandFor('branch')).toBe(EDGE_BAND_HEAVY)
    expect(SIDEBAR_DROP_POLICY.edgeBandFor('chat')).toBe(EDGE_BAND_HEAVY)
    expect(SIDEBAR_DROP_POLICY.edgeBandFor('workflow')).toBe(EDGE_BAND_HEAVY)
  })

  describe('chat rows are workspace-scoped, not repo-scoped', () => {
    const chatRow = (id: string, over: Partial<SidebarRow> = {}) =>
      makeRow({ id, kind: 'chat', ownsWorktree: false, workspaceId: 'ws-1', ...over })

    it('allows a chat drop onto another chat, though neither is in any repo', () => {
      expect(SIDEBAR_DROP_POLICY.allowedModes([chatRow('chat-a')], chatRow('chat-b'))).toEqual(
        ALL_MODES,
      )
    })

    it('allows a chat drop into another workspace — the placement moves it there', () => {
      const subject = chatRow('chat-a', { workspaceId: 'ws-1' })
      const target = chatRow('chat-b', { workspaceId: 'ws-2' })
      expect(SIDEBAR_DROP_POLICY.allowedModes([subject], target)).toEqual(ALL_MODES)
    })

    const ws1Branch = () => makeRow({ id: 'ws-1', parentId: 'home-1' })
    const leavesOnly = { before: true, after: true, into: false }

    it('lets a WORKING chat only out of its own workspace', () => {
      const subject = chatRow('chat-a', { working: true })
      expect(SIDEBAR_DROP_POLICY.allowedModes([subject], ws1Branch())).toEqual(leavesOnly)
    })

    it('reads a tree chat row as working off its entity, despite its row saying false', () => {
      setChatWorking('chat-a', true)
      const subject = chatRow('chat-a', { working: false })
      expect(SIDEBAR_DROP_POLICY.allowedModes([subject], ws1Branch())).toEqual(leavesOnly)
    })

    it('allows a tree chat row whose live turn state says idle', () => {
      setChatWorking('chat-a', false)
      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [chatRow('chat-a', { working: false })],
          chatRow('chat-b'),
        ),
      ).toEqual(ALL_MODES)
    })

    it('allows a chat the ChatStore does not know about', () => {
      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [chatRow('chat-unknown', { working: false })],
          chatRow('chat-b'),
        ),
      ).toEqual(ALL_MODES)
    })

    it('still refuses dropping a chat onto itself', () => {
      expect(SIDEBAR_DROP_POLICY.allowedModes([chatRow('chat-a')], chatRow('chat-a'))).toEqual(
        NO_MODES,
      )
    })

    it('threads a chat into a branch row of another workspace', () => {
      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [chatRow('chat-a', { workspaceId: 'home-1' })],
          makeRow({ id: 'ws-1', parentId: 'home-1' }),
        ),
      ).toEqual(ALL_MODES)
    })

    describe('a working chat beside a branch row', () => {
      beforeEach(() => {
        getHomeWorkspaceId.mockImplementation((projectId: string) =>
          projectId === 'proj-1' ? 'home-ws-1' : null,
        )
      })

      const repoHeader = (id: string, repoId: string) =>
        makeRow({
          id,
          repoIcon: {
            repoId,
            projectId: 'proj-1',
            name: repoId,
            avatarLabel: 'R',
            avatarColor: 'bg-indigo-700',
          },
        })

      it('refuses a home chat beside a repo header — that level is its own project home', () => {
        expect(
          SIDEBAR_DROP_POLICY.allowedModes(
            [chatRow('home-chat', { workspaceId: 'home-ws-1', working: true })],
            repoHeader('home-1', 'repo-1'),
          ),
        ).toEqual({ before: false, after: false, into: true })
      })

      it('allows a repo chat beside a DIFFERENT repo’s locked branch', () => {
        expect(
          SIDEBAR_DROP_POLICY.allowedModes(
            [chatRow('chat-a', { workspaceId: 'home-2', working: true })],
            makeRow({ id: 'ws-locked', parentId: 'home-1' }),
          ),
        ).toEqual(ALL_MODES)
      })
    })

    it('allows a chat onto a folder row — filing it in is a real move now', () => {
      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [chatRow('chat-a')],
          makeRow({ id: 'folder-1', kind: 'folder', workspaceId: null }),
        ),
      ).toEqual(ALL_MODES)
    })

    it('refuses a tree row onto a chat — a branch is not one of a chat’s threads', () => {
      expect(
        SIDEBAR_DROP_POLICY.allowedModes([makeRow({ id: 'ws-1' })], chatRow('chat-a')),
      ).toEqual(NO_MODES)
    })
  })

  describe('project-home folders are project-scoped, not repo-scoped', () => {
    const homeFolderRow = (id: string, over: Partial<SidebarRow> = {}) =>
      makeRow({ id, kind: 'folder', workspaceId: null, ...over })

    it('allows reordering/filing a home folder relative to another row in the SAME project’s home tree', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      seedHomeTree('proj-1', {
        chats: [],
        folders: [
          { id: 'home-folder-1', repoId: '', name: 'Notes', order: 0 },
          { id: 'home-folder-2', repoId: '', name: 'Ideas', order: 1 },
        ],
      })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [homeFolderRow('home-folder-1')],
          homeFolderRow('home-folder-2'),
        ),
      ).toEqual(ALL_MODES)
    })

    it('refuses a home folder dropped onto a DIFFERENT project’s home folder', () => {
      getHomeWorkspaceId.mockImplementation((projectId: string) =>
        projectId === 'proj-2' ? 'home-ws-2' : 'home-ws-1',
      )
      seedHomeTree('proj-1', {
        chats: [],
        folders: [{ id: 'home-folder-1', repoId: '', name: 'A', order: 0 }],
      })
      seedHomeTree('proj-2', {
        chats: [],
        folders: [{ id: 'home-folder-2', repoId: '', name: 'B', order: 0 }],
      })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [homeFolderRow('home-folder-1')],
          homeFolderRow('home-folder-2'),
        ),
      ).toEqual(NO_MODES)
    })

    it('allows filing/reordering a home folder relative to a home CHAT in the SAME project', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      seedHomeTree('proj-1', {
        chats: [{ id: 'home-chat-1', repoId: '', title: 'testing', order: 0 }],
        folders: [{ id: 'home-folder-1', repoId: '', name: 'Notes', order: 1 }],
      })
      const homeChatRow = makeRow({ id: 'home-chat-1', kind: 'chat', workspaceId: null })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes([homeFolderRow('home-folder-1')], homeChatRow),
      ).toEqual(ALL_MODES)
    })

    it('refuses mixing a home folder with a repo folder in one drag', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      seedHomeTree('proj-1', {
        chats: [],
        folders: [{ id: 'home-folder-1', repoId: '', name: 'A', order: 0 }],
      })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [homeFolderRow('home-folder-1'), makeRow({ id: 'folder-1', kind: 'folder' })],
          homeFolderRow('home-folder-1'),
        ),
      ).toEqual(NO_MODES)
    })

    it('allows filing a home CHAT into a home folder in the SAME project', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      seedHomeTree('proj-1', {
        chats: [{ id: 'home-chat-1', repoId: '', title: 'testing', order: 0 }],
        folders: [{ id: 'home-folder-1', repoId: '', name: 'Notes', order: 0 }],
      })
      const homeChatRow = makeRow({ id: 'home-chat-1', kind: 'chat', workspaceId: null })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes([homeChatRow], homeFolderRow('home-folder-1')),
      ).toEqual(ALL_MODES)
    })

    describe('relative to a repo header row', () => {
      const repoHeader = (id: string, projectId: string, repoId: string) =>
        makeRow({
          id,
          kind: 'branch',
          repoIcon: { repoId, projectId, name: repoId, avatarLabel: 'A', avatarColor: 'c' },
        })

      beforeEach(() => {
        getHomeWorkspaceId.mockReturnValue('home-ws-1')
        seedHomeTree('proj-1', {
          chats: [],
          folders: [{ id: 'home-folder-1', repoId: '', name: 'Notes', order: 0 }],
        })
      })

      it('reorders (never nests) a home folder past a repo header in the SAME project', () => {
        expect(
          SIDEBAR_DROP_POLICY.allowedModes(
            [homeFolderRow('home-folder-1')],
            repoHeader('home-1', 'proj-1', 'repo-1'),
          ),
        ).toEqual(REORDER_MODES)
      })

      it('refuses a home folder dropped beside a DIFFERENT project’s repo header', () => {
        expect(
          SIDEBAR_DROP_POLICY.allowedModes(
            [homeFolderRow('home-folder-1')],
            repoHeader('home-3', 'proj-2', 'repo-3'),
          ),
        ).toEqual(NO_MODES)
      })

      it('refuses a home folder dropped beside an ORDINARY branch row inside a repo', () => {
        expect(
          SIDEBAR_DROP_POLICY.allowedModes(
            [homeFolderRow('home-folder-1')],
            makeRow({ id: 'ws-1', parentId: 'home-1' }),
          ),
        ).toEqual(NO_MODES)
      })
    })
  })

  describe('a repo header row is project-home-scoped, not repo-scoped', () => {
    const repoRow = (
      id: string,
      projectId: string,
      repoId: string,
      over: Partial<SidebarRow> = {},
    ) =>
      makeRow({
        id,
        kind: 'branch',
        repoIcon: {
          repoId,
          projectId,
          name: repoId,
          avatarLabel: 'A',
          avatarColor: 'bg-indigo-700',
        },
        ...over,
      })

    it('reorders (never nests) relative to another repo header in the SAME project', () => {
      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [repoRow('home-1', 'proj-1', 'repo-1')],
          repoRow('home-2', 'proj-1', 'repo-2'),
        ),
      ).toEqual(REORDER_MODES)
    })

    it('refuses a repo header dropped onto a DIFFERENT project’s repo header', () => {
      expect(
        SIDEBAR_DROP_POLICY.allowedModes(
          [repoRow('home-1', 'proj-1', 'repo-1')],
          repoRow('home-3', 'proj-2', 'repo-3'),
        ),
      ).toEqual(NO_MODES)
    })

    it('refuses a repo header dropped onto an ORDINARY branch row (not a repo header)', () => {
      const ordinaryBranch = makeRow({ id: 'ws-1', kind: 'branch', workspaceId: 'ws-1' })
      expect(
        SIDEBAR_DROP_POLICY.allowedModes([repoRow('home-1', 'proj-1', 'repo-1')], ordinaryBranch),
      ).toEqual(NO_MODES)
    })

    it('reorders (never nests) relative to a home CHAT in the same project', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      seedHomeTree('proj-1', {
        chats: [{ id: 'home-chat-1', repoId: '', title: 'testing', order: 0 }],
        folders: [],
      })
      const homeChatRow = makeRow({ id: 'home-chat-1', kind: 'chat', workspaceId: null })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes([repoRow('home-1', 'proj-1', 'repo-1')], homeChatRow),
      ).toEqual(REORDER_MODES)
    })

    it('reorders OR nests into a home FOLDER in the same project', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      seedHomeTree('proj-1', {
        chats: [],
        folders: [{ id: 'home-folder-1', repoId: '', name: 'Notes', order: 0 }],
      })
      const homeFolderRow = makeRow({ id: 'home-folder-1', kind: 'folder', workspaceId: null })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes([repoRow('home-1', 'proj-1', 'repo-1')], homeFolderRow),
      ).toEqual(ALL_MODES)
    })

    it('refuses to reorder past a home folder that is itself nested inside a CHAT — but still allows nesting INTO it', () => {
      getHomeWorkspaceId.mockReturnValue('home-ws-1')
      seedHomeTree('proj-1', {
        chats: [{ id: 'home-chat-1', repoId: '', title: 'testing', order: 0 }],
        folders: [
          { id: 'home-folder-1', repoId: '', name: 'Notes', parentId: 'home-chat-1', order: 0 },
        ],
      })
      const nestedFolderRow = makeRow({
        id: 'home-folder-1',
        kind: 'folder',
        workspaceId: null,
        parentId: 'home-chat-1',
      })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes([repoRow('home-1', 'proj-1', 'repo-1')], nestedFolderRow),
      ).toEqual({ before: false, after: false, into: true })
    })

    it('refuses a repo header dropped onto a DIFFERENT project’s home folder', () => {
      getHomeWorkspaceId.mockImplementation((projectId: string) =>
        projectId === 'proj-2' ? 'home-ws-2' : 'home-ws-1',
      )
      seedHomeTree('proj-2', {
        chats: [],
        folders: [{ id: 'home-folder-2', repoId: '', name: 'Ideas', order: 0 }],
      })
      const homeFolderRow = makeRow({ id: 'home-folder-2', kind: 'folder', workspaceId: null })

      expect(
        SIDEBAR_DROP_POLICY.allowedModes([repoRow('home-1', 'proj-1', 'repo-1')], homeFolderRow),
      ).toEqual(NO_MODES)
    })
  })

  describe('a target in the Recents band (spec §8.1)', () => {
    const recentsRow = (id: string, over: Partial<SidebarRow> = {}) =>
      ({
        ...makeRow({ id, kind: 'chat', ownsWorktree: false, workspaceId: null, ...over }),
        inRecents: true,
      }) as SidebarRow & { inRecents: true }
    const ownerRow = (id: string, workspaceId: string) =>
      recentsRow(id, { kind: 'branch', ownsWorktree: true, workspaceId })

    it('lets a chat entry reorder past a workspace-owning entry, and back', () => {
      const owner = ownerRow('chat-owner', 'ws-1')
      const plain = recentsRow('chat-plain', { workspaceId: 'ws-2' })

      expect(SIDEBAR_DROP_POLICY.allowedModes([plain], owner)).toEqual(ALL_MODES)
      expect(SIDEBAR_DROP_POLICY.allowedModes([owner], plain)).toEqual(ALL_MODES)
    })

    it('lets two workspace-owning entries from DIFFERENT repos reorder past each other', () => {
      const fromRepo1 = ownerRow('chat-a', 'ws-1')
      const fromRepo2 = ownerRow('chat-b', 'ws-2')

      expect(SIDEBAR_DROP_POLICY.allowedModes([fromRepo1], fromRepo2)).toEqual(ALL_MODES)
    })

    it('lets a split carrying both a chat and a workspace-owning entry move as one', () => {
      const owner = makeRow({ id: 'chat-owner', kind: 'branch', workspaceId: 'ws-1' })
      const plain = makeRow({ id: 'chat-plain', kind: 'chat', workspaceId: 'ws-1' })

      expect(SIDEBAR_DROP_POLICY.allowedModes([owner, plain], recentsRow('chat-other'))).toEqual(
        ALL_MODES,
      )
    })

    it('still refuses a folder — a Recents slot names a chat, and a folder names none', () => {
      const folder = makeRow({ id: 'folder-1', kind: 'folder', workspaceId: null })
      expect(SIDEBAR_DROP_POLICY.allowedModes([folder], recentsRow('chat-plain'))).toEqual(NO_MODES)
    })

    it('still refuses a WORKING row, and a row dropped onto itself', () => {
      const working = recentsRow('chat-a', { working: true })
      expect(SIDEBAR_DROP_POLICY.allowedModes([working], recentsRow('chat-b'))).toEqual(NO_MODES)
      const row = recentsRow('chat-a')
      expect(SIDEBAR_DROP_POLICY.allowedModes([row], row)).toEqual(NO_MODES)
    })
  })
})
