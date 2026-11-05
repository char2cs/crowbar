import { describe, expect, it } from 'vitest'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import type { Chat, Folder, Repo, Workspace } from '@/core/entities/repo-types'
import { legacyTreeIndex } from '@/features/workspace/tree/legacy-tree-index'
import {
  HOME_ROW_ID,
  makeChat,
  makeOwnedWorkspace,
  makeRepo,
  makeWorkspace,
  repoRows,
} from '@/__tests__/__fixtures__/tree-rows'

// "Create workspace" mints the workspace and its first chat in one atomic call
// (`POST .../chats {ownWorktree: true}`).
describe('an atomically-created own-worktree chat', () => {
  it('the fresh workspace and its owning chat fold into ONE row, titled by the chat', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'workspace-abc123' },
      { id: 'c-1' },
    )
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const rows = repoRows(repo)

    // No separate `ws-1` row, and no separate `chat`-kind row for `c-1` — the
    // fold's whole point.
    expect(rows.find((r) => r.id === 'ws-1')).toBeUndefined()
    expect(rows.filter((r) => r.kind === 'chat')).toHaveLength(0)

    const row = rows.find((r) => r.id === 'c-1')
    expect(row?.kind).toBe('branch')
    expect(row?.workspaceId).toBe('ws-1')
    // Untitled until the agent (or the user) names it — same fallback a
    // bubble chat's label already uses.
    expect(row?.label).toBe(UNTITLED_CHAT_LABEL)
    expect(row?.branchName).toBe('workspace-abc123')

    // Just the home row (`makeTestRepo`'s own fixture) and this one fold.
    expect(rows.filter((r) => r.id !== HOME_ROW_ID)).toHaveLength(1)
  })
})

// Rule 6: a regular fork is one row, never a `branch` row for the workspace plus a separate `chat`
// row for the ordinary conversation that owns it.
describe('an ordinary fork folds into its owning chat', () => {
  it('an ordinary fork with no children renders as ONE branch-styled row', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'feature/x', added: 18000, deleted: 3 },
      { id: 'c-1', title: 'Fix the parser' },
    )
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const rows = repoRows(repo)

    // Just the home row (`makeTestRepo`'s own fixture) and this one fold.
    const nonHome = rows.filter((r) => r.id !== HOME_ROW_ID)
    expect(nonHome).toHaveLength(1)
    const row = nonHome[0]
    expect(row.id).toBe('c-1')
    expect(row.kind).toBe('branch')
    expect(row.workspaceId).toBe('ws-1')
    expect(row.label).toBe('Fix the parser')
    expect(row.branchName).toBe('feature/x')
    expect(row.added).toBe(18000)
    expect(row.deleted).toBe(3)
    expect(row.locked).toBe(false)
    expect(row.parentId).toBe(HOME_ROW_ID)
  })

  it('an ordinary fork with a genuine bubble thread child renders as two correctly-nested rows', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'feature/x' },
      { id: 'c-1', title: 'Fix the parser' },
    )
    const thread = makeChat({ id: 'c-2', title: 'A follow-up', parentId: 'c-1' })
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat, thread],
    })
    const rows = repoRows(repo)

    expect(rows.filter((r) => r.id !== HOME_ROW_ID)).toHaveLength(2)
    const parent = rows.find((r) => r.id === 'c-1')
    const child = rows.find((r) => r.id === 'c-2')
    expect(parent?.kind).toBe('branch')
    expect(child?.kind).toBe('chat')
    expect(child?.ownsWorktree).toBe(false)
    expect(child?.parentId).toBe('c-1')
  })

  it('an ordinary fork with a thread that forked its own workspace renders as two branch-styled rows', () => {
    const { workspace: parentWs, chat: parentChat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'feature/x', added: 10, deleted: 1 },
      { id: 'c-1', title: 'Fix the parser' },
    )
    // Rule 8's git-branch-icon case: a genuine child chat (real `parentId`) that ALSO owns a brand-
    // new workspace forked from its parent's branch.
    const { workspace: childWs, chat: childChat } = makeOwnedWorkspace(
      { id: 'ws-2', branch: 'feature/x/nested', parentId: 'ws-1', added: 4, deleted: 0 },
      { id: 'c-2', title: 'A nested spike' },
    )
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [parentWs, childWs],
      chats: [parentChat, childChat],
    })
    const rows = repoRows(repo)

    expect(rows.filter((r) => r.id !== HOME_ROW_ID)).toHaveLength(2)
    // Never a `chat`-kind row for either.
    expect(rows.filter((r) => r.kind === 'chat')).toHaveLength(0)

    const parent = rows.find((r) => r.id === 'c-1')
    expect(parent?.kind).toBe('branch')
    expect(parent?.branchName).toBe('feature/x')
    expect(parent?.added).toBe(10)

    const child = rows.find((r) => r.id === 'c-2')
    expect(child?.kind).toBe('branch')
    expect(child?.workspaceId).toBe('ws-2')
    expect(child?.label).toBe('A nested spike')
    expect(child?.branchName).toBe('feature/x/nested')
    expect(child?.added).toBe(4)
    // …correctly nested under its TRUE parent, not the repo root.
    expect(child?.parentId).toBe('c-1')
  })

  it('a workspace with no resolvable owner stays a chat-less branch row (genuinely chat-less state)', () => {
    // `owningChatId` absent, or naming a chat this repo has not seeded — both
    // mean "nothing to fold by yet" (`Workspace.owningChatId`'s own doc), and
    // `deriveRows` leaves such a workspace exactly as it always rendered.
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeWorkspace({ id: 'ws-1', branch: 'feature/x' })],
    })
    const rows = repoRows(repo)
    const nonHome = rows.filter((r) => r.id !== HOME_ROW_ID)
    expect(nonHome).toHaveLength(1)
    expect(nonHome[0]).toMatchObject({ id: 'ws-1', kind: 'branch', label: 'feature/x' })
  })
})

// `owningChatOf` (owning_chat.go) files a new child directly under its parent workspace's id, a row
// every workspace (locked, home or fork) has from creation.
describe('a fresh child parents directly onto the workspace id (Task 9)', () => {
  it('a chat parented onto a locked branch’s raw workspace id resolves under its folded row', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-locked', branch: 'develop', status: 'locked' },
      { id: 'branch-chat-1' },
    )
    const child = makeChat({ id: 'c-1', title: 'New-style child', parentId: 'ws-locked' })
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat, child],
    })
    expect(repoRows(repo).find((r) => r.id === 'c-1')?.parentId).toBe('branch-chat-1')
  })

  it('a chat parented onto the repo-home’s raw workspace id resolves under the home row', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [makeChat({ id: 'c-1', title: 'New-style home child', parentId: 'ws-home' })],
    })
    expect(repoRows(repo).find((r) => r.id === 'c-1')?.parentId).toBe(HOME_ROW_ID)
  })
})

// An imported repo whose repo.workspaces includes its home must not draw a second, unlabeled row.
describe('the repo home never doubles as a workspace row', () => {
  it('draws exactly ONE row when repo.workspaces wrongly contains the home workspace', () => {
    const repo = makeRepo({
      name: 'athas',
      defaultWorkspaceId: 'ws-home',
      // No branch yet — an import whose defaultBranch has not resolved, which
      // is what made the ghost unlabeled rather than merely duplicated.
      workspaces: [makeWorkspace({ id: 'ws-home', branch: '' })],
    })
    const rows = repoRows(repo)
    expect(rows).toHaveLength(1)
    expect(rows[0]?.id).toBe(HOME_ROW_ID)
    expect(rows[0]?.label).toBe('athas')
    expect(rows.some((r) => !r.label)).toBe(false)
  })

  it('draws exactly ONE row when that home workspace also names its owning chat', () => {
    // The other half of the same payload bug: with `owningChatId` present the
    // fold relabels the ghost to the home chat's id, producing a DUPLICATE id
    // rather than an empty label.
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeWorkspace({ id: 'ws-home', branch: 'main', owningChatId: HOME_ROW_ID })],
    })
    const rows = repoRows(repo)
    expect(rows.filter((r) => r.workspaceId === 'ws-home')).toHaveLength(1)
    expect(rows.filter((r) => r.id === HOME_ROW_ID)).toHaveLength(1)
  })

  it('still nests a real fork under the home ROW, not under the ghost it replaced', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [
        makeWorkspace({ id: 'ws-home', branch: 'main' }),
        makeWorkspace({ id: 'ws-1', branch: 'feature/x', parentId: 'ws-home' }),
      ],
    })
    const rows = repoRows(repo)
    expect(rows).toHaveLength(2)
    expect(rows.find((r) => r.id === 'ws-1')?.parentId).toBe(HOME_ROW_ID)
  })
})

// Today's placement rules turned into explicit containers: organisation and lineage stay
// independent, and an incompatible folder edge falls back to the real fork parent.
const ws = (id: string, over: Partial<Workspace> = {}): Workspace => ({
  id,
  branch: `branch/${id}`,
  age: 'now',
  ...over,
})
const folder = (id: string, over: Partial<Folder> = {}): Folder => ({
  id,
  repoId: 'r1',
  name: id,
  order: 0,
  ...over,
})
const chat = (id: string, over: Partial<Chat> = {}): Chat => ({
  id,
  repoId: 'r1',
  title: id,
  order: 0,
  ...over,
})

// One repo with no home checkout, so its members hang straight off the repo node.
function treeOf(workspaces: Workspace[], folders: Folder[] = [], chats: Chat[] = []) {
  const index = legacyTreeIndex(
    [{ id: 'r1', name: 'r', avatarLabel: 'R', avatarColor: '', workspaces, folders, chats }],
    null,
  )
  const kids = (id: string) => index.children(id).map((n) => n.id)
  return { index, kids, roots: kids('r1') }
}

describe('workspaces and folders', () => {
  it('a workspace with no parent and no folder sits at the repo root', () => {
    expect(treeOf([ws('a'), ws('b')]).roots).toEqual(['a', 'b'])
  })

  it('a folder with no parent sits at the repo root', () => {
    const { roots, index } = treeOf([], [folder('f1')])
    expect(roots).toEqual(['f1'])
    expect(index.node('f1')).toMatchObject({ kind: 'folder', name: 'f1', parentId: 'r1' })
  })

  it('a fork root with a folder renders inside that folder', () => {
    const { roots, kids } = treeOf([ws('root', { folderId: 'f1' })], [folder('f1')])
    expect(roots).toEqual(['f1'])
    expect(kids('f1')).toEqual(['root'])
  })

  it('a fork child can be organised inside a folder under its fork parent', () => {
    const { kids } = treeOf(
      [ws('parent'), ws('child', { parentId: 'parent', folderId: 'f1' })],
      [folder('f1', { parentId: 'parent' })],
    )
    expect(kids('parent')).toEqual(['f1'])
    expect(kids('f1')).toEqual(['child'])
  })

  it('an incompatible folder edge falls back to the real fork parent', () => {
    const { kids } = treeOf(
      [ws('parent'), ws('other'), ws('child', { parentId: 'parent', folderId: 'f1' })],
      [folder('f1', { parentId: 'other' })],
    )
    expect(kids('f1')).toEqual([])
    expect(kids('parent')).toEqual(['child'])
  })

  // The daemon files a create started on a folder row as a fork root (folderId, no parentId).
  it('a workspace created ON a folder row lands inside it, however deep the folder', () => {
    const { kids, roots } = treeOf(
      [ws('main'), ws('created', { folderId: 'backlog' })],
      [folder('backlog', { parentId: 'main' })],
    )
    expect(kids('backlog')).toEqual(['created'])
    expect(roots).not.toContain('created')
  })

  it('descendants follow their fork ancestor into a folder', () => {
    const { kids } = treeOf(
      [ws('root', { folderId: 'f1' }), ws('kid', { parentId: 'root' })],
      [folder('f1')],
    )
    expect(kids('f1')).toEqual(['root'])
    expect(kids('root')).toEqual(['kid'])
  })

  it('folders nest inside folders, and hang off a protected branch', () => {
    const { kids } = treeOf(
      [ws('develop', { status: 'locked' }), ws('feat', { parentId: 'develop', folderId: 'f1' })],
      [folder('f1', { parentId: 'develop' }), folder('inner', { parentId: 'f1' })],
    )
    expect(kids('develop')).toEqual(['f1'])
    expect(kids('f1')).toEqual(['inner', 'feat'])
  })

  it('an unresolvable folderId falls back to the repo root, not a dropped row', () => {
    expect(treeOf([ws('orphan', { folderId: 'gone' })]).roots).toEqual(['orphan'])
  })

  // The anchor walk steps THROUGH the chat: stopping there would drop the folder edge.
  it('a folder inside a chat still anchors a fork child to the right workspace', () => {
    const { kids } = treeOf(
      [ws('w1'), ws('kid', { parentId: 'w1', folderId: 'f1' })],
      [folder('f1', { parentId: 'c1' })],
      [chat('c1', { workspaceId: 'w1' })],
    )
    expect(kids('f1')).toEqual(['kid'])
  })
})

describe('chats', () => {
  it('a filed chat sits where it is filed, not on its workspace ground', () => {
    const { kids } = treeOf(
      [ws('w1')],
      [folder('f1')],
      [
        chat('filed', { workspaceId: 'w1', parentId: 'f1' }),
        chat('parent', { workspaceId: 'w1' }),
        chat('thread', { workspaceId: 'w1', parentId: 'parent' }),
      ],
    )
    expect(kids('f1')).toEqual(['filed'])
    expect(kids('w1')).toEqual(['parent'])
    expect(kids('parent')).toEqual(['thread'])
  })

  it('a chat filed nowhere nests under its workspace, else at the repo root', () => {
    const { kids, roots } = treeOf(
      [ws('w1', { order: 0 })],
      [],
      [chat('a', { workspaceId: 'w1' }), chat('b')],
    )
    expect(kids('w1')).toEqual(['a'])
    expect(roots).toEqual(['w1', 'b'])
  })
})

describe('sibling order', () => {
  it('un-ordered rows keep the order they arrived in', () => {
    expect(treeOf([ws('c'), ws('a'), ws('b')]).roots).toEqual(['c', 'a', 'b'])
  })

  it('order sorts siblings at every depth; rows without one sort last', () => {
    const { roots, kids } = treeOf([
      ws('none'),
      ws('root', { order: 0 }),
      ws('b', { parentId: 'root', order: 1 }),
      ws('a', { parentId: 'root', order: 0 }),
    ])
    expect(roots).toEqual(['root', 'none'])
    expect(kids('root')).toEqual(['a', 'b'])
  })

  it('chats, folders and workspaces share one order; on a tie folders, then branches, then chats', () => {
    expect(
      treeOf([ws('w', { order: 1 })], [folder('f', { order: 2 })], [chat('c', { order: 0 })]).roots,
    ).toEqual(['c', 'w', 'f'])
    expect(treeOf([ws('w', { order: 0 })], [folder('f')], [chat('c')]).roots).toEqual([
      'f',
      'w',
      'c',
    ])
  })

  // The daemon compares time.Time; Go's RFC3339Nano wire form does not string-sort by instant.
  it('a tied level breaks on the createdAt INSTANT, not the ISO string', () => {
    const { roots } = treeOf([
      ws('a', { order: 0, createdAt: '2026-11-01T01:30:00-07:00' }),
      ws('b', { order: 0, createdAt: '2026-11-01T01:15:00-08:00' }),
      ws('c', { order: 0, createdAt: '2026-09-17T20:36:46.53Z' }),
      ws('d', { order: 0, createdAt: '2026-09-17T20:36:46.5Z' }),
    ])
    expect(roots).toEqual(['d', 'c', 'a', 'b'])
  })
})

describe('corrupt input degrades, never hangs', () => {
  it('workspace, folder and self cycles re-root', () => {
    expect(treeOf([ws('a', { parentId: 'b' }), ws('b', { parentId: 'a' })]).roots).toEqual([
      'a',
      'b',
    ])
    expect(
      treeOf([], [folder('f1', { parentId: 'f2' }), folder('f2', { parentId: 'f1' })]).roots,
    ).toHaveLength(2)
    expect(treeOf([ws('self', { parentId: 'self' })]).roots).toEqual(['self'])
    expect(treeOf([], [], [chat('self', { parentId: 'self' })]).roots).toEqual(['self'])
  })

  // A loop through BOTH structures is what one placement authority exists for: every row stays.
  it('a chat -> folder -> chat and a chat -> folder -> workspace cycle draw every row', () => {
    const drawn = (t: ReturnType<typeof treeOf>) => {
      const ids: string[] = []
      const walk = (id: string) => t.index.children(id).forEach((n) => (ids.push(n.id), walk(n.id)))
      walk('r1')
      return new Set(ids)
    }
    const a = treeOf(
      [],
      [folder('f1', { parentId: 'c2' })],
      [chat('c1', { parentId: 'f1' }), chat('c2', { parentId: 'c1' })],
    )
    expect(drawn(a)).toEqual(new Set(['f1', 'c1', 'c2']))
    const b = treeOf(
      [ws('w1', { folderId: 'f1' })],
      [folder('f1', { parentId: 'c1' })],
      [chat('c1', { workspaceId: 'w1' })],
    )
    expect(drawn(b)).toEqual(new Set(['w1', 'f1', 'c1']))
  })
})

// A workspace's owning chat is `type: 'chat'`, so ownership must never be read off Chat.type.
describe('resolves the owning chat without ever reading Chat.type', () => {
  it('a locked branch folds by Workspace.owningChatId alone, even when no chat claims ownsWorktree', () => {
    const workspace = makeWorkspace({
      id: 'ws-locked',
      branch: 'develop',
      status: 'locked',
      owningChatId: 'branch-chat-1',
    })
    // Deliberately no `ownsWorktree` and no `type` on the candidate chat — the
    // direct `Workspace.owningChatId` field is the whole resolution.
    const chat = makeChat({ id: 'branch-chat-1', title: '', workspaceId: 'ws-locked' })
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const row = repoRows(repo).find((r) => r.workspaceId === 'ws-locked')
    expect(row?.id).toBe('branch-chat-1')
    expect(row?.kind).toBe('branch')
    expect(row?.locked).toBe(true)
  })

  it('the repo-home row folds by Repo.defaultOwningChatId directly, with no chats array at all', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      defaultBranch: 'main',
      defaultOwningChatId: 'home-owner',
      chats: [],
    })
    const home = repoRows(repo).find((r) => r.parentId === null)
    expect(home?.id).toBe('home-owner')
    expect(home?.kind).toBe('branch')
    expect(home?.branchName).toBe('main')
  })

  it('the repo-home row degrades gracefully (never throws) when no owner has resolved yet', () => {
    const repo: Repo = {
      id: 'r1',
      name: 'crowbar',
      avatarLabel: 'C',
      avatarColor: 'bg-indigo-700',
      workspaces: [],
      defaultWorkspaceId: 'ws-home',
      defaultBranch: 'main',
      // No `defaultOwningChatId`, no `chats` at all — the daemon's chat-first
      // create is racing this repo's own seed, a real (if narrow) window
      // since the two ride separate streams.
    }
    expect(() => repoRows(repo)).not.toThrow()
    const home = repoRows(repo).find((r) => r.parentId === null)
    // Degrades exactly like every other unresolved workspace-owning row:
    // left as its own raw workspace id rather than dropped.
    expect(home?.id).toBe('ws-home')
    expect(home?.kind).toBe('branch')
  })
})
