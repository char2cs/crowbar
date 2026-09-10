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

/**
 * The id of the `branch` row every fixture's repo home owns.
 *
 * Every repo with a home workspace has one by the time `rowsFromRepo` is
 * called (2026-09-08 sidebar-placement-unification Task 9: chat-first, at
 * creation — `MintOwningChat`/`AttachOwningWorkspace`, never a boot
 * backfill), and `SidebarTreeSurface` does not build rows for a repo until
 * its chat seed has landed. So the fixtures carry it too — a repo without
 * one is not a state this function is ever handed, and modelling it would be
 * modelling the loading state the caller already excludes. Its OWN `type` is
 * deliberately `'chat'`, not `'branch'` — Task 9 retires the retype, so
 * `makeTestRepo` also stamps `defaultOwningChatId`, the direct field
 * `rowsFromRepo` reads instead.
 */
const HOME_ROW_ID = 'home-branch-row'

/**
 * A workspace wired to the chat that owns it, `Workspace.owningChatId` ->
 * `Chat.id`, the way `rows-from-repo.ts`'s `resolveOwnerChats` reads it for
 * every fold — a locked branch and an ordinary fork alike. Neither half is
 * useful alone once a test cares about folding: a workspace with no
 * `owningChatId` (or a chat this repo hasn't seeded) is the "genuinely
 * chat-less" state `rowsFromRepo` deliberately leaves unfolded, covered on
 * its own below.
 */
function makeOwnedWorkspace(
  wsOver: Partial<Workspace> & { id: string; branch: string },
  chatOver: Partial<Chat> & { id: string },
): { workspace: Workspace; chat: Chat } {
  const chat = makeTestChat({ title: '', ...chatOver, workspaceId: wsOver.id, ownsWorktree: true })
  const workspace = makeTestWorkspace({ ...wsOver, owningChatId: chat.id })
  return { workspace, chat }
}

function makeTestRepo(over: Partial<Repo> = {}): Repo {
  const repo: Repo = {
    id: 'r1',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [],
    ...over,
  }
  if (!repo.defaultWorkspaceId) return repo
  const home = makeTestChat({
    id: HOME_ROW_ID,
    title: '',
    workspaceId: repo.defaultWorkspaceId,
    repoId: repo.id,
    ownsWorktree: true,
  })
  return {
    ...repo,
    defaultOwningChatId: repo.defaultOwningChatId ?? HOME_ROW_ID,
    chats: [home, ...(repo.chats ?? [])],
  }
}

describe('rowsFromRepo', () => {
  it('a locked branch becomes a branch-kind row', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'develop', status: 'locked' },
      { id: 'branch-chat-1' },
    )
    const repo = makeTestRepo({ workspaces: [workspace], chats: [chat] })
    const rows = rowsFromRepo(repo)
    const row = rows.find((r) => r.workspaceId === 'ws-1')
    expect(row?.kind).toBe('branch')
    expect(row?.ownsWorktree).toBe(true)
    expect(row?.locked).toBe(true)
    // Exactly one row — this is the fold this whole file pins, not a
    // coincidence of the fixture.
    expect(rows).toHaveLength(1)
  })

  it('a chat folder becomes a folder-kind row', () => {
    const repo = makeTestRepo({
      folders: [makeTestFolder({ id: 'f-1', name: 'Bugs' })],
    })
    const rows = rowsFromRepo(repo)
    expect(rows.find((r) => r.id === 'f-1')?.kind).toBe('folder')
  })

  // A repo folder always sits under a real worktree, unlike a project-home
  // one (rows-from-home.ts) — its own "+" always forks a branch.
  it('a repo folder owns a worktree, so its own "+" can fork a branch', () => {
    const repo = makeTestRepo({
      folders: [makeTestFolder({ id: 'f-1', name: 'Bugs' })],
    })
    const rows = rowsFromRepo(repo)
    expect(rows.find((r) => r.id === 'f-1')?.ownsWorktree).toBe(true)
  })

  it('the default workspace becomes the one root row, labelled with the repo name', () => {
    const repo = makeTestRepo({ defaultWorkspaceId: 'ws-home', defaultBranch: 'main' })
    const rows = rowsFromRepo(repo)
    const home = rows.find((r) => r.id === HOME_ROW_ID)
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
    expect(rows.find((r) => r.id === 'ws-1')?.parentId).toBe(HOME_ROW_ID)
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
      const home = rowsFromRepo(repo).find((r) => r.id === HOME_ROW_ID)
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
      const home = rowsFromRepo(repo).find((r) => r.id === HOME_ROW_ID)
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
    expect(rowsFromRepo(repo).find((r) => r.id === 'c-1')?.parentId).toBe(HOME_ROW_ID)
  })

  it('a chat whose workspace is the repo home hangs off the repo-home row', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [makeTestChat({ id: 'c-1', title: 'Home chat', workspaceId: 'ws-home' })],
    })
    expect(rowsFromRepo(repo).find((r) => r.id === 'c-1')?.parentId).toBe(HOME_ROW_ID)
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
    expect(rows.find((r) => r.id === 'c-1')?.parentId).toBe(HOME_ROW_ID)
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
    expect(parentOf.get('ws-1')).toBe(HOME_ROW_ID)
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
    const rows = rowsFromRepo(repo).filter((r) => r.parentId === HOME_ROW_ID)
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
    expect(rowsFromRepo(repo).find((r) => r.id === 'c-1')?.parentId).toBe(HOME_ROW_ID)
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
      expect(row?.parentId).toBe(HOME_ROW_ID)
      expect(row?.workspaceId).toBe('ws-in-r2')
    })
  })
})

/**
 * The bug this closes: a branch row was id'd from its `Workspace` record, and
 * the backend's placement validation resolves a create's parent id as a CHAT
 * row — so "+" on a locked branch, the repo home or an existing workspace named
 * a row the daemon has never heard of. Every such workspace now owns a real
 * chat, minted chat-first at creation (`MintOwningChat`/`AttachOwningWorkspace`,
 * 2026-09-08 sidebar-placement-unification Task 9 — never a boot backfill, and
 * never retyped to `'branch'`), and THAT row's id — read off
 * `Workspace.owningChatId` directly — is the branch row's identity.
 */
describe('rowsFromRepo — a branch row is identified by its owning chat', () => {
  it('a locked branch row carries its owning branch-chat id, not the workspace id', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-locked', branch: 'develop', status: 'locked' },
      { id: 'branch-chat-1' },
    )
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const row = rowsFromRepo(repo).find((r) => r.workspaceId === 'ws-locked')
    expect(row?.id).toBe('branch-chat-1')
  })

  it('a thread under that branch hangs off the owning chat id, not the workspace id', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-locked', branch: 'develop', status: 'locked' },
      { id: 'branch-chat-1' },
    )
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat, makeTestChat({ id: 'c-1', title: 'Thread', parentId: 'branch-chat-1' })],
    })
    expect(rowsFromRepo(repo).find((r) => r.id === 'c-1')?.parentId).toBe('branch-chat-1')
  })

  it('the repo-home row carries its owning branch-chat id, not defaultWorkspaceId', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      defaultBranch: 'main',
    })
    const home = rowsFromRepo(repo).find((r) => r.parentId === null)
    expect(home?.id).toBe(HOME_ROW_ID)
    expect(home?.workspaceId).toBe('ws-home')
    expect(home?.branchName).toBe('main')
  })

  it('an owning branch chat is the branch row itself, never a second row beside it', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-locked', branch: 'develop', status: 'locked' },
      { id: 'branch-chat-1' },
    )
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const rows = rowsFromRepo(repo)
    expect(rows.map((r) => r.id)).toEqual([...new Set(rows.map((r) => r.id))])
    expect(rows.filter((r) => r.kind === 'chat')).toHaveLength(0)
  })
})

/**
 * Task 8: the sidebar's "create workspace" affordance mints the workspace AND
 * its first chat in ONE atomic backend call (`POST .../chats
 * {ownWorktree: true}`, space-content-actions.ts's `handleCreate`).
 *
 * This used to pin that both halves rendered as TWO rows the moment they
 * landed — a `chat`-kind row for the conversation, nested under a `branch`-kind
 * row for the workspace it owns. That was the bug (product rule 6: "a chat
 * with a workspace" is ONE row, not two) — this now pins the fix: the fresh
 * workspace and its owning chat fold into the single row rule 6 describes,
 * titled by the chat, the instant both land.
 */
describe('rowsFromRepo — an atomically-created own-worktree chat', () => {
  it('the fresh workspace and its owning chat fold into ONE row, titled by the chat', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'workspace-abc123' },
      { id: 'c-1' },
    )
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const rows = rowsFromRepo(repo)

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
    const home = rowsFromRepo(repo).find((r) => r.id === HOME_ROW_ID)
    expect(home?.kind).toBe('branch')
    expect(home?.ownsWorktree).toBe(true)
  })

  it('a locked branch (develop) stays chat-less and branch-kind, even with other chats in the repo', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'develop', branch: 'develop', status: 'locked' },
      { id: 'develop-branch-row' },
    )
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat, makeTestChat({ id: 'c-1', title: 'unrelated', workspaceId: 'ws-home' })],
    })
    const rows = rowsFromRepo(repo)
    const develop = rows.find((r) => r.workspaceId === 'develop')
    expect(develop?.kind).toBe('branch')
    expect(develop?.ownsWorktree).toBe(true)
    // A locked branch keeps its BRANCH-labelled row (addendum rules 1-4:
    // "Folder mechanism"), even though it is now id'd and folded exactly
    // like any other workspace-owning chat.
    expect(develop?.label).toBe('develop')
    expect(rows.some((r) => r.kind === 'chat' && r.workspaceId === 'develop')).toBe(false)
  })
})

/**
 * The bug rule 6 closes: a regular fork used to render as TWO rows — a
 * `branch`-kind row for the workspace, id'd from the `Workspace` itself, and a
 * SEPARATE `chat`-kind row beneath it for the ordinary conversation
 * (`type: 'chat'`, not `branch`) that owned it. That split is retired: an
 * ordinary fork now folds its owning chat into the SAME one row a locked
 * branch already was — id'd from the chat, labelled with the chat's title,
 * and carrying its workspace's `branchName`/`added`/`deleted` on the side
 * `rows-from-repo.ts`'s doc calls the row's "ground", not its identity.
 */
describe('rowsFromRepo — an ordinary fork folds into its owning chat', () => {
  it('an ordinary fork with no children renders as ONE branch-styled row', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'feature/x', added: 18000, deleted: 3 },
      { id: 'c-1', title: 'Fix the parser' },
    )
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const rows = rowsFromRepo(repo)

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
    const thread = makeTestChat({ id: 'c-2', title: 'A follow-up', parentId: 'c-1' })
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat, thread],
    })
    const rows = rowsFromRepo(repo)

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
    // Rule 8's git-branch-icon case: a genuine child chat (real `parentId`)
    // that ALSO owns a brand-new workspace forked from its parent's branch.
    // `ws-2`'s OWN fork lineage (`parentId: 'ws-1'`) is what actually places
    // it — see `foldWorkspaceOwners`'s own doc on why an owning chat's `parentId`
    // is used only to find what hangs off it, never to decide where the
    // folded row itself renders — so `c-2.parentId` here is deliberately left
    // unset, matching a state the wire does not guarantee agrees with it.
    const { workspace: childWs, chat: childChat } = makeOwnedWorkspace(
      { id: 'ws-2', branch: 'feature/x/nested', parentId: 'ws-1', added: 4, deleted: 0 },
      { id: 'c-2', title: 'A nested spike' },
    )
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [parentWs, childWs],
      chats: [parentChat, childChat],
    })
    const rows = rowsFromRepo(repo)

    expect(rows.filter((r) => r.id !== HOME_ROW_ID)).toHaveLength(2)
    // Never a `chat`-kind row for either — this is the bug that used to make
    // this exact case impossible to render correctly at all.
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
    // `rowsFromRepo` leaves such a workspace exactly as it always rendered.
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'feature/x' })],
    })
    const rows = rowsFromRepo(repo)
    const nonHome = rows.filter((r) => r.id !== HOME_ROW_ID)
    expect(nonHome).toHaveLength(1)
    expect(nonHome[0]).toMatchObject({ id: 'ws-1', kind: 'branch', label: 'feature/x' })
  })
})

/**
 * Spec §3.4: a freshly-minted workspace's branch is provisional (italic)
 * until it is renamed — the same idea `labelProvisional` already carries for
 * an untitled chat's title, applied to the branch half. There is no separate
 * wire flag for this; the signal is the backend's own generated-name shape
 * (`hierarchy.branch_name.go`'s `provisionalBranchName`: `"chat-" + 8 hex
 * chars`), which self-clears the moment a real rename replaces it.
 */
describe('rowsFromRepo — provisional branch naming', () => {
  // These two model a workspace with NO resolvable owner — the "genuinely
  // chat-less" fallback (see the describe block above) — where the row's
  // label IS still its branch, so the branch-placeholder pattern is still
  // the right provisional signal.
  it('a genuinely chat-less workspace whose branch is still the generated placeholder renders provisional', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'chat-a1b2c3d4' })],
    })
    const row = rowsFromRepo(repo).find((r) => r.id === 'ws-1')
    expect(row?.labelProvisional).toBe(true)
  })

  it('a genuinely chat-less workspace with a real (renamed) branch does not render provisional', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeTestWorkspace({ id: 'ws-1', branch: 'feature/real-name' })],
    })
    const row = rowsFromRepo(repo).find((r) => r.id === 'ws-1')
    expect(row?.labelProvisional).toBeFalsy()
  })

  it('the home row is provisional when the default branch is still generated', () => {
    const repo = makeTestRepo({ defaultWorkspaceId: 'ws-home', defaultBranch: 'chat-deadbeef' })
    const home = rowsFromRepo(repo).find((r) => r.id === HOME_ROW_ID)
    expect(home?.labelProvisional).toBe(true)
  })

  it('the home row is not provisional for a real default branch', () => {
    const repo = makeTestRepo({ defaultWorkspaceId: 'ws-home', defaultBranch: 'main' })
    const home = rowsFromRepo(repo).find((r) => r.id === HOME_ROW_ID)
    expect(home?.labelProvisional).toBeFalsy()
  })

  // A FOLDED (owned, unlocked) fork's label is now its chat's title, not its
  // branch (rule 6) — so its provisional-ness now tracks the same thing a
  // bubble chat's already does (an empty title), regardless of whether the
  // branch itself is still the generated placeholder. The branch-placeholder
  // pattern still decides the SECOND line's text, just never its italics.
  it('a folded fork with no chat title yet renders provisional, even with a real branch name', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'feature/real-name' },
      { id: 'c-1', title: '' },
    )
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const row = rowsFromRepo(repo).find((r) => r.id === 'c-1')
    expect(row?.labelProvisional).toBe(true)
  })

  it('a folded fork with a real chat title does not render provisional, even on a still-generated branch', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'chat-a1b2c3d4' },
      { id: 'c-1', title: 'Fix the parser' },
    )
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const row = rowsFromRepo(repo).find((r) => r.id === 'c-1')
    expect(row?.labelProvisional).toBeFalsy()
  })
})

/**
 * 2026-09-08 sidebar-placement-unification Task 10: Task 9 deleted the
 * boot backfill that used to retype a workspace's owning chat to
 * `type: 'branch'` and stop maintaining it thereafter — a fresh workspace's
 * owning chat is minted (and stays) `type: 'chat'`. `rowsFromRepo` must
 * therefore resolve a workspace-owning row WITHOUT ever comparing
 * `Chat.type`, straight off `Workspace.owningChatId` (and, for the repo-home
 * row specifically — never a member of `repo.workspaces` — off the
 * equivalent `Repo.defaultOwningChatId` Task 10 lifts for it).
 */
describe('rowsFromRepo — resolves the owning chat without ever reading Chat.type', () => {
  it('a locked branch folds by Workspace.owningChatId alone, even when no chat claims ownsWorktree', () => {
    const workspace = makeTestWorkspace({
      id: 'ws-locked',
      branch: 'develop',
      status: 'locked',
      owningChatId: 'branch-chat-1',
    })
    // Deliberately no `ownsWorktree` and no `type` on the candidate chat — the
    // direct `Workspace.owningChatId` field is the whole resolution.
    const chat = makeTestChat({ id: 'branch-chat-1', title: '', workspaceId: 'ws-locked' })
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const row = rowsFromRepo(repo).find((r) => r.workspaceId === 'ws-locked')
    expect(row?.id).toBe('branch-chat-1')
    expect(row?.kind).toBe('branch')
    expect(row?.locked).toBe(true)
  })

  it('the repo-home row folds by Repo.defaultOwningChatId directly, with no chats array at all', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      defaultBranch: 'main',
      defaultOwningChatId: 'home-owner',
      chats: [],
    })
    const home = rowsFromRepo(repo).find((r) => r.parentId === null)
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
    expect(() => rowsFromRepo(repo)).not.toThrow()
    const home = rowsFromRepo(repo).find((r) => r.parentId === null)
    // Degrades exactly like every other unresolved workspace-owning row:
    // left as its own raw workspace id rather than dropped.
    expect(home?.id).toBe('ws-home')
    expect(home?.kind).toBe('branch')
  })
})

/**
 * Task 9's `owningChatOf` (owning_chat.go) files a NEW child directly under
 * its parent workspace's own id — no owning-chat lookup — the instant that
 * parent's `Node{Kind:workspace}` row exists, which is now unconditionally
 * true the moment ANY workspace (locked, home, or an ordinary fork) is
 * created. `rowsFromRepo` has to resolve such a child (`Chat.parentId` ===
 * the literal `Workspace.id`, never the owning chat's own id) as a child of
 * the FOLDED row, for both a regular tree workspace and the repo-home
 * workspace — the repo-home case is the one this file's own tree builder
 * excludes from `workspaces` today, so it needs its own coverage.
 */
describe('rowsFromRepo — a fresh child parents directly onto the workspace id (Task 9)', () => {
  it('a chat parented onto a locked branch’s raw workspace id resolves under its folded row', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-locked', branch: 'develop', status: 'locked' },
      { id: 'branch-chat-1' },
    )
    const child = makeTestChat({ id: 'c-1', title: 'New-style child', parentId: 'ws-locked' })
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat, child],
    })
    expect(rowsFromRepo(repo).find((r) => r.id === 'c-1')?.parentId).toBe('branch-chat-1')
  })

  it('a chat parented onto the repo-home’s raw workspace id resolves under the home row', () => {
    const repo = makeTestRepo({
      defaultWorkspaceId: 'ws-home',
      chats: [makeTestChat({ id: 'c-1', title: 'New-style home child', parentId: 'ws-home' })],
    })
    expect(rowsFromRepo(repo).find((r) => r.id === 'c-1')?.parentId).toBe(HOME_ROW_ID)
  })
})
