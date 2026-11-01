import { describe, expect, it, vi } from 'vitest'
import type { Operation } from '@/lib/store/operations'
import { type RemovalEntry } from '@/features/workspace/tree/tree-ui-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { indexNodes, type TreeNode } from '@/features/workspace/tree/tree-index'
import type { RowViews } from '@/features/workspace/tree/views'
import { deriveRows, type TreeInput } from '@/features/workspace/tree/rows'
import {
  HOME_ROW_ID,
  makeChat,
  makeFolder,
  makeRepo,
  makeWorkspace,
  makeOwnedWorkspace,
  repoRows,
} from '@/__tests__/__fixtures__/tree-rows'

// deriveRows over a hand-built index and views: no stores, no placement fallbacks.
type ChatView = NonNullable<ReturnType<RowViews['chat']>>
type WorkspaceView = NonNullable<ReturnType<RowViews['workspace']>>
type RepoView = NonNullable<ReturnType<RowViews['repo']>>

const node = (id: string, kind: TreeNode['kind'], parentId = '', order = 0): TreeNode => ({
  id,
  kind,
  parentId,
  order,
  ...(kind === 'folder' ? { name: id } : {}),
})
const wsView = (rowId: string, label: string): WorkspaceView => ({
  rowId,
  label,
  labelProvisional: false,
  working: false,
  icon: { locked: false, isPlaceholder: false },
})
const repoView = (rowId: string | null): RepoView => ({
  rowId,
  workspaceId: 'home-ws',
  label: 'crowbar',
  labelProvisional: false,
  working: false,
  locked: false,
})
const chatView = (label: string): ChatView => ({ label, labelProvisional: false })

function pure(
  nodes: TreeNode[],
  views: {
    chats?: Record<string, ChatView>
    workspaces?: Record<string, WorkspaceView>
    repos?: Record<string, RepoView>
  },
  over: Partial<TreeInput> = {},
): SidebarRow[] {
  const rowViews: RowViews = {
    chat: (id) => views.chats?.[id],
    workspace: (id) => views.workspaces?.[id],
    repo: (id) => views.repos?.[id],
  }
  return deriveRows({
    projectId: 'p1',
    homeWorkspaceId: 'project-home-ws',
    index: indexNodes(nodes),
    views: rowViews,
    operations: [],
    isPresent: () => false,
    ui: { removals: [], draft: null },
    ...over,
  })
}

describe('deriveRows', () => {
  it('draws a repo row with its home checkout owning chat as the row id', () => {
    const rows = pure([node('r1', 'repo'), node('w1', 'workspace', 'r1')], {
      repos: { r1: repoView('home-owner') },
      workspaces: { w1: wsView('w1-owner', 'feature') },
    })
    expect(rows.map((r) => [r.id, r.kind, r.parentId, r.workspaceId])).toEqual([
      ['home-owner', 'branch', null, 'home-ws'],
      ['w1-owner', 'branch', 'home-owner', 'w1'],
    ])
  })

  it('draws a workspace row by its owning chat id, else by the workspace id', () => {
    const rows = pure([node('w1', 'workspace'), node('w2', 'workspace')], {
      workspaces: { w1: wsView('c1', 'owned'), w2: wsView('w2', 'chatless') },
    })
    expect(rows.map((r) => r.id)).toEqual(['c1', 'w2'])
  })

  it('draws children in index order, depth-first', () => {
    const rows = pure(
      [
        node('f1', 'folder'),
        node('c1', 'chat', 'f1'),
        node('c2', 'chat', 'c1'),
        node('c3', 'chat'),
      ],
      { chats: { c1: chatView('one'), c2: chatView('two'), c3: chatView('three') } },
    )
    expect(rows.map((r) => [r.id, r.parentId])).toEqual([
      ['f1', null],
      ['c1', 'f1'],
      ['c2', 'c1'],
      ['c3', null],
    ])
  })

  it('draws project-home chats and folders at the root between repo rows by order, not forkable', () => {
    const rows = pure(
      [node('c1', 'chat', '', 0), node('r1', 'repo', '', 1), node('f1', 'folder', '', 2)],
      {
        chats: { c1: chatView('home chat') },
        repos: { r1: repoView('home-owner') },
      },
    )
    expect(rows.map((r) => [r.id, r.order])).toEqual([
      ['c1', 0],
      ['home-owner', 1],
      ['f1', 2],
    ])
    const byId = new Map(rows.map((r) => [r.id, r]))
    expect(byId.get('c1')).toMatchObject({ canFork: false, workspaceId: 'project-home-ws' })
    expect(byId.get('f1')).toMatchObject({ ownsWorktree: false, workspaceId: 'project-home-ws' })
  })

  it('never draws a node it has no view for, nor its subtree, and reports it once', () => {
    const warn = vi.spyOn(console, 'warn').mockImplementation(() => {})
    const kinds = ['workspace', 'chat', 'repo'] as const
    const nodes = kinds.flatMap((k) => [
      node(`ghost-${k}`, k),
      node(`kid-${k}`, 'chat', `ghost-${k}`),
    ])
    const views = { chats: Object.fromEntries(kinds.map((k) => [`kid-${k}`, chatView('kid')])) }
    expect(pure(nodes, views)).toEqual([])
    pure(nodes, views)
    expect(warn).toHaveBeenCalledTimes(3)
    warn.mockRestore()
  })

  it('marks a row being removed, without hiding it', () => {
    const entry = { kind: 'chat', id: 'c1', entryId: 'e1', deadlineAt: 5, extra: 2 } as const
    const rows = pure(
      [node('c1', 'chat')],
      { chats: { c1: chatView('going') } },
      {
        ui: { removals: [entry as unknown as RemovalEntry], draft: null },
      },
    )
    expect(rows[0]?.removal).toEqual({ entryId: 'e1', deadlineAt: 5, extra: 2 })
  })

  it('draws an in-flight create from its operation item until its node exists', () => {
    const op: Operation = {
      id: 'op-1',
      kind: 'chat.create',
      subject: { kind: 'repo', id: 'r1' },
      projectId: 'p1',
      label: 'New chat',
      state: 'running',
      items: [{ ref: { kind: 'chat', id: 'c-new' }, label: 'New chat', state: 'running' }],
      dismissed: false,
      version: 1,
      createdAt: '2026-10-29T10:00:00Z',
      updatedAt: '2026-10-29T10:00:00Z',
    }
    const nodes = [node('r1', 'repo')]
    const views = { repos: { r1: repoView('home-owner') } }
    const creating = pure(nodes, views, { operations: [op] })
    expect(creating.at(-1)).toMatchObject({ id: 'c-new', parentId: 'home-owner', working: true })
    // The daemon appends a create at the end of its level.
    expect(creating.at(-1)?.order).toBe(Number.MAX_SAFE_INTEGER)
    const arrived = pure(nodes, views, { operations: [op], isPresent: (ref) => ref.id === 'c-new' })
    expect(arrived.map((r) => r.id)).toEqual(['home-owner'])
  })

  it("draws the naming input at the draft's parent and slot, in its own project only", () => {
    const draft = {
      kind: 'chat',
      projectId: 'p1',
      workspaceId: 'w1',
      parentId: 'c1',
      order: 3,
    } as const
    const ui = { removals: [], draft: { ...draft, ownsWorktree: false } }
    expect(pure([], {}, { ui })).toMatchObject([{ id: 'create-draft', parentId: 'c1', order: 3 }])
    expect(pure([], {}, { ui, projectId: 'p2' })).toEqual([])
  })
})

// Design spec §3.1-3.2: a worktree chat and a bubble chat are first-class rows beside branches and
// folders, placed by one `parentId` walk that folders are transparent to.
describe('chat rows', () => {
  it('a chat becomes a chat-kind row that owns no worktree', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [makeChat({ id: 'c-1', title: 'Fix the parser', workspaceId: 'ws-home' })],
    })
    const row = repoRows(repo).find((r) => r.id === 'c-1')
    expect(row?.kind).toBe('chat')
    expect(row?.label).toBe('Fix the parser')
    expect(row?.ownsWorktree).toBe(false)
    expect(row?.workspaceId).toBe('ws-home')
  })

  // A bubble's row falls back to its nearest ancestor workspace, or a click has nothing to open.
  it('a bubble with no workspace of its own falls back to its nearest ancestor workspace', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeWorkspace({ id: 'ws-1', branch: 'feature/x' })],
      chats: [makeChat({ id: 'c-1', title: 'Nested bubble', parentId: 'ws-1' })],
    })
    const row = repoRows(repo).find((r) => r.id === 'c-1')
    expect(row?.kind).toBe('chat')
    expect(row?.ownsWorktree).toBe(false)
    expect(row?.workspaceId).toBe('ws-1')
  })

  it('a bubble at the repo root falls back to the repo home workspace', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [makeChat({ id: 'c-1', title: 'Root bubble' })],
    })
    const row = repoRows(repo).find((r) => r.id === 'c-1')
    expect(row?.workspaceId).toBe('ws-home')
  })

  it('a chat at the repo root hangs off the repo-home row', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [makeChat({ id: 'c-1', title: 'Root chat' })],
    })
    expect(repoRows(repo).find((r) => r.id === 'c-1')?.parentId).toBe(HOME_ROW_ID)
  })

  it('a chat whose workspace is the repo home hangs off the repo-home row', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [makeChat({ id: 'c-1', title: 'Home chat', workspaceId: 'ws-home' })],
    })
    expect(repoRows(repo).find((r) => r.id === 'c-1')?.parentId).toBe(HOME_ROW_ID)
  })

  it('a chat parented to a folder nests under that folder', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      folders: [makeFolder({ id: 'f-1', name: 'Bugs' })],
      chats: [makeChat({ id: 'c-1', title: 'Filed chat', parentId: 'f-1' })],
    })
    expect(repoRows(repo).find((r) => r.id === 'c-1')?.parentId).toBe('f-1')
  })

  it('a chat parented to a workspace nests under that workspace', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeWorkspace({ id: 'ws-1', branch: 'feature/x' })],
      chats: [makeChat({ id: 'c-1', title: 'Branch chat', parentId: 'ws-1' })],
    })
    expect(repoRows(repo).find((r) => r.id === 'c-1')?.parentId).toBe('ws-1')
  })

  it('a chat with no parentId nests under the workspace it owns', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeWorkspace({ id: 'ws-1', branch: 'feature/x' })],
      chats: [makeChat({ id: 'c-1', title: 'Owned chat', workspaceId: 'ws-1' })],
    })
    expect(repoRows(repo).find((r) => r.id === 'c-1')?.parentId).toBe('ws-1')
  })

  it('a chat parented to another chat nests under it — a thread', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [
        makeChat({ id: 'c-1', title: 'Parent' }),
        makeChat({ id: 'c-2', title: 'Thread', parentId: 'c-1' }),
      ],
    })
    const rows = repoRows(repo)
    expect(rows.find((r) => r.id === 'c-2')?.parentId).toBe('c-1')
    expect(rows.find((r) => r.id === 'c-1')?.parentId).toBe(HOME_ROW_ID)
  })

  it('a chat nested N chats deep still resolves to its own parent', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeWorkspace({ id: 'ws-1', branch: 'feature/x' })],
      folders: [makeFolder({ id: 'f-1', name: 'Bugs', parentId: 'ws-1' })],
      chats: [
        makeChat({ id: 'c-1', title: 'One', parentId: 'f-1' }),
        makeChat({ id: 'c-2', title: 'Two', parentId: 'c-1' }),
        makeChat({ id: 'c-3', title: 'Three', parentId: 'c-2' }),
        makeChat({ id: 'c-4', title: 'Four', parentId: 'c-3' }),
      ],
    })
    const parentOf = new Map(repoRows(repo).map((r) => [r.id, r.parentId]))
    expect(parentOf.get('c-1')).toBe('f-1')
    expect(parentOf.get('c-2')).toBe('c-1')
    expect(parentOf.get('c-3')).toBe('c-2')
    expect(parentOf.get('c-4')).toBe('c-3')
    // …and the chain is genuinely anchored to the repo, not floating.
    expect(parentOf.get('f-1')).toBe('ws-1')
    expect(parentOf.get('ws-1')).toBe(HOME_ROW_ID)
  })

  it('a folder inside a chat holds that chat’s threads', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      folders: [makeFolder({ id: 'f-1', name: 'Spikes', parentId: 'c-1' })],
      chats: [
        makeChat({ id: 'c-1', title: 'Parent' }),
        makeChat({ id: 'c-2', title: 'Filed thread', parentId: 'f-1' }),
      ],
    })
    const parentOf = new Map(repoRows(repo).map((r) => [r.id, r.parentId]))
    expect(parentOf.get('f-1')).toBe('c-1')
    expect(parentOf.get('c-2')).toBe('f-1')
  })

  it('chats, folders and branches interleave on their SHARED order at one level', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeWorkspace({ id: 'ws-1', branch: 'feature/x', order: 1 })],
      folders: [makeFolder({ id: 'f-1', name: 'Bugs', order: 2 })],
      chats: [makeChat({ id: 'c-1', title: 'First', order: 0 })],
    })
    const rows = repoRows(repo).filter((r) => r.parentId === HOME_ROW_ID)
    expect([...rows].sort((a, b) => a.order - b.order).map((r) => r.id)).toEqual([
      'c-1',
      'ws-1',
      'f-1',
    ])
  })

  it('a chat whose parent is unknown root-anchors rather than vanishing', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [makeChat({ id: 'c-1', title: 'Orphan', parentId: 'gone' })],
    })
    expect(repoRows(repo).find((r) => r.id === 'c-1')?.parentId).toBe(HOME_ROW_ID)
  })

  it('a chat cycle degrades to rows rather than hanging the render', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [
        makeChat({ id: 'c-1', title: 'A', parentId: 'c-2' }),
        makeChat({ id: 'c-2', title: 'B', parentId: 'c-1' }),
      ],
    })
    const rows = repoRows(repo)
    expect(rows.filter((r) => r.kind === 'chat')).toHaveLength(2)
  })

  it('a repo with no chats yet is byte-identical to one built without the field', () => {
    const over = {
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeWorkspace({ id: 'ws-1', branch: 'feature/x' })],
      folders: [makeFolder({ id: 'f-1', name: 'Bugs' })],
    }
    expect(repoRows(makeRepo({ ...over, chats: [] }))).toEqual(repoRows(makeRepo(over)))
  })

  // The bug class tasks 21/22/26/34 each found a version of, in this same area.
  describe('cross-repo / cross-workspace isolation', () => {
    it('never renders a chat that belongs to another repo', () => {
      const repo = makeRepo({
        id: 'r1',
        defaultWorkspaceId: 'ws-home',
        chats: [
          makeChat({ id: 'mine', title: 'Mine', repoId: 'r1' }),
          makeChat({ id: 'theirs', title: 'Theirs', repoId: 'r2' }),
        ],
      })
      const rows = repoRows(repo)
      expect(rows.find((r) => r.id === 'mine')).toBeDefined()
      expect(rows.find((r) => r.id === 'theirs')).toBeUndefined()
    })

    it('never nests one repo’s chat under another repo’s row', () => {
      // `theirs` names r1's own folder as its parent — the shape a mis-scoped
      // list would produce. It must not be drawn under it; it must not be drawn.
      const repo = makeRepo({
        id: 'r1',
        defaultWorkspaceId: 'ws-home',
        folders: [makeFolder({ id: 'f-1', name: 'Bugs' })],
        chats: [makeChat({ id: 'theirs', title: 'Theirs', repoId: 'r2', parentId: 'f-1' })],
      })
      expect(repoRows(repo).some((r) => r.id === 'theirs')).toBe(false)
    })

    it('a chat naming a workspace of another repo root-anchors in its own', () => {
      // Spec §9.2: a bubble moved across repos keeps reading ancestors that live in the repo it
      // left, so a repo's chats are not a closed set.
      const repo = makeRepo({
        id: 'r1',
        defaultWorkspaceId: 'ws-home',
        chats: [makeChat({ id: 'c-1', title: 'Moved', workspaceId: 'ws-in-r2' })],
      })
      const row = repoRows(repo).find((r) => r.id === 'c-1')
      expect(row?.parentId).toBe(HOME_ROW_ID)
      expect(row?.workspaceId).toBe('ws-in-r2')
    })
  })
})

// A branch row is id'd by its owning chat: the daemon resolves a create's parent as a chat row.
describe('a branch row is identified by its owning chat', () => {
  it('a locked branch row carries its owning branch-chat id, not the workspace id', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-locked', branch: 'develop', status: 'locked' },
      { id: 'branch-chat-1' },
    )
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const row = repoRows(repo).find((r) => r.workspaceId === 'ws-locked')
    expect(row?.id).toBe('branch-chat-1')
  })

  it('a thread under that branch hangs off the owning chat id, not the workspace id', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-locked', branch: 'develop', status: 'locked' },
      { id: 'branch-chat-1' },
    )
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat, makeChat({ id: 'c-1', title: 'Thread', parentId: 'branch-chat-1' })],
    })
    expect(repoRows(repo).find((r) => r.id === 'c-1')?.parentId).toBe('branch-chat-1')
  })

  it('the repo-home row carries its owning branch-chat id, not defaultWorkspaceId', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      defaultBranch: 'main',
    })
    const home = repoRows(repo).find((r) => r.parentId === null)
    expect(home?.id).toBe(HOME_ROW_ID)
    expect(home?.workspaceId).toBe('ws-home')
    expect(home?.branchName).toBe('main')
  })

  it('an owning branch chat is the branch row itself, never a second row beside it', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-locked', branch: 'develop', status: 'locked' },
      { id: 'branch-chat-1' },
    )
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const rows = repoRows(repo)
    expect(rows.map((r) => r.id)).toEqual([...new Set(rows.map((r) => r.id))])
    expect(rows.filter((r) => r.kind === 'chat')).toHaveLength(0)
  })
})

// The protected branches (develop, main, project home) never take the create-workspace path.
describe('protected branches stay chat-less branch rows', () => {
  it('the project-home row stays chat-less and branch-kind', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      defaultBranch: 'main',
      chats: [makeChat({ id: 'c-1', title: 'unrelated', workspaceId: 'ws-1' })],
    })
    const home = repoRows(repo).find((r) => r.id === HOME_ROW_ID)
    expect(home?.kind).toBe('branch')
    expect(home?.ownsWorktree).toBe(true)
  })

  it('a locked branch (develop) stays chat-less and branch-kind, even with other chats in the repo', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'develop', branch: 'develop', status: 'locked' },
      { id: 'develop-branch-row' },
    )
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat, makeChat({ id: 'c-1', title: 'unrelated', workspaceId: 'ws-home' })],
    })
    const rows = repoRows(repo)
    const develop = rows.find((r) => r.workspaceId === 'develop')
    expect(develop?.kind).toBe('branch')
    expect(develop?.ownsWorktree).toBe(true)
    // A locked branch keeps its branch-labelled row while folded like any owned workspace.
    expect(develop?.label).toBe('develop')
    expect(rows.some((r) => r.kind === 'chat' && r.workspaceId === 'develop')).toBe(false)
  })
})
