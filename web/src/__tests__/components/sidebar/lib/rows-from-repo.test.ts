import { describe, expect, it } from 'vitest'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import type { Chat, Folder, Repo, Workspace } from '@/lib/store/sidebar'

function makeTestWorkspace(over: Partial<Workspace> & { id: string; branch: string }): Workspace {
  return { age: '', ...over }
}

function makeTestFolder(over: Partial<Folder> & { id: string; name: string }): Folder {
  return { repoId: 'r1', order: 0, ...over }
}

function makeTestChat(over: Partial<Chat> & { id: string; title: string }): Chat {
  return { repoId: 'r1', order: 0, ...over }
}

function makeTestRepo(over: Partial<Repo> = {}): Repo {
  return {
    id: 'r1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [],
    ...over,
  }
}

describe('rowsFromRepo', () => {
  it('a locked branch becomes a branch-kind row', () => {
    const repo = makeTestRepo({
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'develop', status: 'locked' })],
    })
    const rows = rowsFromRepo(repo)
    const row = rows.find((r) => r.workspaceId === 'ws-1')
    expect(row?.kind).toBe('branch')
    expect(row?.ownsWorktree).toBe(true)
  })

  it('a chat folder becomes a folder-kind row', () => {
    const repo = makeTestRepo({
      folders: [makeTestFolder({ id: 'f-1', name: 'Bugs' })],
    })
    const rows = rowsFromRepo(repo)
    expect(rows.find((r) => r.id === 'f-1')?.kind).toBe('folder')
  })

  it('the default workspace becomes the one root row, labelled with the repo name', () => {
    const repo = makeTestRepo({ defaultWorkspaceId: 'ws-home', defaultBranch: 'main' })
    const rows = rowsFromRepo(repo)
    const home = rows.find((r) => r.id === 'ws-home')
    expect(home?.kind).toBe('branch')
    expect(home?.parentId).toBeNull()
    expect(home?.label).toBe('crowbar')
    expect(home?.branchName).toBe('main')
  })

  it('a root-level workspace nests under the default workspace', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'feature/x' })],
    })
    const rows = rowsFromRepo(repo)
    expect(rows.find((r) => r.id === 'ws-1')?.parentId).toBe('ws-home')
  })

  it('a forked workspace nests under its fork parent, not the default workspace', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [
        makeTestWorkspace({ id: 'ws-1', branch: 'feature/x' }),
        makeTestWorkspace({ id: 'ws-2', branch: 'feature/x/child', parentId: 'ws-1' }),
      ],
    })
    const rows = rowsFromRepo(repo)
    expect(rows.find((r) => r.id === 'ws-2')?.parentId).toBe('ws-1')
  })

  it('drops a workspace whose status is a deleted tombstone', () => {
    const repo = makeTestRepo({
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'gone', status: 'deleted' })],
    })
    const rows = rowsFromRepo(repo)
    expect(rows.find((r) => r.workspaceId === 'ws-1')).toBeUndefined()
  })

  it('produces no rows for a repo with nothing yet', () => {
    expect(rowsFromRepo(makeTestRepo())).toEqual([])
  })

  // Task 5 (icon personalization): the home row's own icon
  // (EditableRepoIcon, repo-icon-mark.tsx) needs the repo's ids and avatar
  // fields to reach the right REST base — see SidebarRow's own doc on the
  // `repoIcon` field.
  describe('the home row’s repoIcon', () => {
    it('carries the repo’s own identity once its owning project has seeded', () => {
      const repo = makeTestRepo({
        id: 'r1',
        projectId: 'p1',
        defaultWorkspaceId: 'ws-home',
        avatarLabel: 'C',
        avatarColor: 'bg-indigo-700',
        avatarURL: 'emoji:🚀',
      })
      const home = rowsFromRepo(repo).find((r) => r.id === 'ws-home')
      expect(home?.repoIcon).toEqual({
        repoId: 'r1',
        projectId: 'p1',
        name: 'crowbar',
        avatarLabel: 'C',
        avatarColor: 'bg-indigo-700',
        avatarURL: 'emoji:🚀',
      })
    })

    it('is absent when the repo has no projectId yet — no REST base to build', () => {
      const repo = makeTestRepo({ id: 'r1', defaultWorkspaceId: 'ws-home' })
      const home = rowsFromRepo(repo).find((r) => r.id === 'ws-home')
      expect(home?.repoIcon).toBeUndefined()
    })

    it('is absent on every non-home row', () => {
      const repo = makeTestRepo({
        id: 'r1',
        projectId: 'p1',
        defaultWorkspaceId: 'ws-home',
        workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'feature/x' })],
      })
      const child = rowsFromRepo(repo).find((r) => r.id === 'ws-1')
      expect(child?.repoIcon).toBeUndefined()
    })
  })
})

/**
 * Design spec §3.1: a chat is one of the FOUR row kinds the tree model is built
 * on — a worktree chat (owns a workspace) and a bubble chat (owns none, threads
 * off a parent) are both first-class tree rows, on equal footing with branches
 * and folders. §3.2: their placement is ONE `parentId` walk, and folders are
 * transparent to it.
 */
describe('rowsFromRepo — chat rows', () => {
  it('a chat becomes a chat-kind row that owns no worktree', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [makeTestChat({ id: 'c-1', title: 'Fix the parser', workspaceId: 'ws-home' })],
    })
    const row = rowsFromRepo(repo).find((r) => r.id === 'c-1')
    expect(row?.kind).toBe('chat')
    expect(row?.label).toBe('Fix the parser')
    expect(row?.ownsWorktree).toBe(false)
    expect(row?.workspaceId).toBe('ws-home')
  })

  it('a chat at the repo root hangs off the repo-home row', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [makeTestChat({ id: 'c-1', title: 'Root chat' })],
    })
    expect(rowsFromRepo(repo).find((r) => r.id === 'c-1')?.parentId).toBe('ws-home')
  })

  it('a chat whose workspace is the repo home hangs off the repo-home row', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [makeTestChat({ id: 'c-1', title: 'Home chat', workspaceId: 'ws-home' })],
    })
    expect(rowsFromRepo(repo).find((r) => r.id === 'c-1')?.parentId).toBe('ws-home')
  })

  it('a chat parented to a folder nests under that folder', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      folders: [makeTestFolder({ id: 'f-1', name: 'Bugs' })],
      chats: [makeTestChat({ id: 'c-1', title: 'Filed chat', parentId: 'f-1' })],
    })
    expect(rowsFromRepo(repo).find((r) => r.id === 'c-1')?.parentId).toBe('f-1')
  })

  it('a chat parented to a workspace nests under that workspace', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'feature/x' })],
      chats: [makeTestChat({ id: 'c-1', title: 'Branch chat', parentId: 'ws-1' })],
    })
    expect(rowsFromRepo(repo).find((r) => r.id === 'c-1')?.parentId).toBe('ws-1')
  })

  it('a chat with no parentId nests under the workspace it owns', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'feature/x' })],
      chats: [makeTestChat({ id: 'c-1', title: 'Owned chat', workspaceId: 'ws-1' })],
    })
    expect(rowsFromRepo(repo).find((r) => r.id === 'c-1')?.parentId).toBe('ws-1')
  })

  it('a chat parented to another chat nests under it — a thread', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [
        makeTestChat({ id: 'c-1', title: 'Parent' }),
        makeTestChat({ id: 'c-2', title: 'Thread', parentId: 'c-1' }),
      ],
    })
    const rows = rowsFromRepo(repo)
    expect(rows.find((r) => r.id === 'c-2')?.parentId).toBe('c-1')
    expect(rows.find((r) => r.id === 'c-1')?.parentId).toBe('ws-home')
  })

  it('a chat nested N chats deep still resolves to its own parent', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'feature/x' })],
      folders: [makeTestFolder({ id: 'f-1', name: 'Bugs', parentId: 'ws-1' })],
      chats: [
        makeTestChat({ id: 'c-1', title: 'One', parentId: 'f-1' }),
        makeTestChat({ id: 'c-2', title: 'Two', parentId: 'c-1' }),
        makeTestChat({ id: 'c-3', title: 'Three', parentId: 'c-2' }),
        makeTestChat({ id: 'c-4', title: 'Four', parentId: 'c-3' }),
      ],
    })
    const parentOf = new Map(rowsFromRepo(repo).map((r) => [r.id, r.parentId]))
    expect(parentOf.get('c-1')).toBe('f-1')
    expect(parentOf.get('c-2')).toBe('c-1')
    expect(parentOf.get('c-3')).toBe('c-2')
    expect(parentOf.get('c-4')).toBe('c-3')
    // …and the chain is genuinely anchored to the repo, not floating.
    expect(parentOf.get('f-1')).toBe('ws-1')
    expect(parentOf.get('ws-1')).toBe('ws-home')
  })

  it('a folder inside a chat holds that chat’s threads', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      folders: [makeTestFolder({ id: 'f-1', name: 'Spikes', parentId: 'c-1' })],
      chats: [
        makeTestChat({ id: 'c-1', title: 'Parent' }),
        makeTestChat({ id: 'c-2', title: 'Filed thread', parentId: 'f-1' }),
      ],
    })
    const parentOf = new Map(rowsFromRepo(repo).map((r) => [r.id, r.parentId]))
    expect(parentOf.get('f-1')).toBe('c-1')
    expect(parentOf.get('c-2')).toBe('f-1')
  })

  it('chats, folders and branches interleave on their SHARED order at one level', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'feature/x', order: 1 })],
      folders: [makeTestFolder({ id: 'f-1', name: 'Bugs', order: 2 })],
      chats: [makeTestChat({ id: 'c-1', title: 'First', order: 0 })],
    })
    const rows = rowsFromRepo(repo).filter((r) => r.parentId === 'ws-home')
    expect([...rows].sort((a, b) => a.order - b.order).map((r) => r.id)).toEqual([
      'c-1',
      'ws-1',
      'f-1',
    ])
  })

  it('a chat whose parent is unknown root-anchors rather than vanishing', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [makeTestChat({ id: 'c-1', title: 'Orphan', parentId: 'gone' })],
    })
    expect(rowsFromRepo(repo).find((r) => r.id === 'c-1')?.parentId).toBe('ws-home')
  })

  it('a chat cycle degrades to rows rather than hanging the render', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [
        makeTestChat({ id: 'c-1', title: 'A', parentId: 'c-2' }),
        makeTestChat({ id: 'c-2', title: 'B', parentId: 'c-1' }),
      ],
    })
    const rows = rowsFromRepo(repo)
    expect(rows.filter((r) => r.kind === 'chat')).toHaveLength(2)
  })

  it('a repo with no chats yet is byte-identical to one built without the field', () => {
    const over = {
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'feature/x' })],
      folders: [makeTestFolder({ id: 'f-1', name: 'Bugs' })],
    }
    expect(rowsFromRepo(makeTestRepo({ ...over, chats: [] }))).toEqual(
      rowsFromRepo(makeTestRepo(over)),
    )
  })

  // The bug class tasks 21/22/26/34 each found a version of, in this same area.
  describe('cross-repo / cross-workspace isolation', () => {
    it('never renders a chat that belongs to another repo', () => {
      const repo = makeTestRepo({
        id: 'r1',
        defaultWorkspaceId: 'ws-home',
        chats: [
          makeTestChat({ id: 'mine', title: 'Mine', repoId: 'r1' }),
          makeTestChat({ id: 'theirs', title: 'Theirs', repoId: 'r2' }),
        ],
      })
      const rows = rowsFromRepo(repo)
      expect(rows.find((r) => r.id === 'mine')).toBeDefined()
      expect(rows.find((r) => r.id === 'theirs')).toBeUndefined()
    })

    it('never nests one repo’s chat under another repo’s row', () => {
      // `theirs` names r1's own folder as its parent — the shape a mis-scoped
      // list would produce. It must not be drawn under it; it must not be drawn.
      const repo = makeTestRepo({
        id: 'r1',
        defaultWorkspaceId: 'ws-home',
        folders: [makeTestFolder({ id: 'f-1', name: 'Bugs' })],
        chats: [makeTestChat({ id: 'theirs', title: 'Theirs', repoId: 'r2', parentId: 'f-1' })],
      })
      expect(rowsFromRepo(repo).some((r) => r.id === 'theirs')).toBe(false)
    })

    it('a chat naming a workspace of another repo root-anchors in its own', () => {
      // Spec §9.2: a bubble moved across repos keeps reading ancestors that live
      // in the repo it left, so a repo's chats are not a closed set. The row is
      // this repo's (the daemon's cwd walk said so) — render it here, at the
      // root, rather than losing it to an edge that resolves to nothing.
      const repo = makeTestRepo({
        id: 'r1',
        defaultWorkspaceId: 'ws-home',
        chats: [makeTestChat({ id: 'c-1', title: 'Moved', workspaceId: 'ws-in-r2' })],
      })
      const row = rowsFromRepo(repo).find((r) => r.id === 'c-1')
      expect(row?.parentId).toBe('ws-home')
      expect(row?.workspaceId).toBe('ws-in-r2')
    })
  })
})

/**
 * Task 8: the sidebar's "create workspace" affordance now mints the
 * workspace AND its first chat in ONE atomic backend call
 * (`POST .../chats {ownWorktree: true}`, space-content-actions.ts's
 * `handleCreate`) instead of the old, chat-less `postWorkspace` — a bare
 * branch row on its own, with a conversation appearing under it only later,
 * from a SEPARATE user action. This pins that once both land, the chat half
 * of that atomic create renders as a REAL, complete `chat`-kind row —
 * labelled, correctly typed, carrying the workspace it owns — not dropped or
 * misrouted into looking like a second, empty branch.
 */
describe('rowsFromRepo — an atomically-created own-worktree chat', () => {
  it('the fresh workspace and its owning chat both render, together, the moment they land', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'workspace-abc123' })],
      chats: [makeTestChat({ id: 'c-1', title: '', workspaceId: 'ws-1' })],
    })
    const rows = rowsFromRepo(repo)

    const chatRow = rows.find((r) => r.id === 'c-1')
    expect(chatRow?.kind).toBe('chat')
    expect(chatRow?.workspaceId).toBe('ws-1')
    expect(chatRow?.label).toBe(UNTITLED_CHAT_LABEL)

    const branchRow = rows.find((r) => r.id === 'ws-1')
    expect(branchRow?.kind).toBe('branch')

    // Unlike the old two-step flow, there is no repo state where the fresh
    // workspace exists with nothing running in it: both rows come from the
    // SAME repo snapshot, in the SAME render.
    expect(rows.filter((r) => r.id === 'ws-1' || r.id === 'c-1')).toHaveLength(2)
  })
})

/**
 * Constraint from the plan: the three protected/locked-branch rows (develop,
 * main, project home) never go through `handleCreate`'s create-workspace
 * path — but this is the one case the migration must never touch, so it gets
 * an explicit regression test proving they still render exactly as before:
 * chat-less `branch`-kind rows, even in a repo that otherwise has chats.
 */
describe('rowsFromRepo — protected branches stay chat-less branch rows', () => {
  it('the project-home row stays chat-less and branch-kind', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      defaultBranch: 'main',
      chats: [makeTestChat({ id: 'c-1', title: 'unrelated', workspaceId: 'ws-1' })],
    })
    const home = rowsFromRepo(repo).find((r) => r.id === 'ws-home')
    expect(home?.kind).toBe('branch')
    expect(home?.ownsWorktree).toBe(true)
  })

  it('a locked branch (develop) stays chat-less and branch-kind, even with other chats in the repo', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeTestWorkspace({ id: 'develop', branch: 'develop', status: 'locked' })],
      chats: [makeTestChat({ id: 'c-1', title: 'unrelated', workspaceId: 'ws-home' })],
    })
    const rows = rowsFromRepo(repo)
    const develop = rows.find((r) => r.id === 'develop')
    expect(develop?.kind).toBe('branch')
    expect(develop?.ownsWorktree).toBe(true)
    expect(rows.some((r) => r.kind === 'chat' && r.workspaceId === 'develop')).toBe(false)
  })
})
