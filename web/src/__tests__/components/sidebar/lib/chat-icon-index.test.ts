import { describe, expect, it } from 'vitest'
import { rowsFromRepo } from '@/components/sidebar/lib/rows-from-repo'
import { chatIconIndex } from '@/components/sidebar/lib/chat-icon-index'
import type { Chat, Repo, Workspace } from '@/lib/store/sidebar'

function makeTestWorkspace(over: Partial<Workspace> & { id: string; branch: string }): Workspace {
  return { age: '', ...over }
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

/**
 * `chatIconIndex` is what `recents-band.tsx` (via `recents-for-project.ts`)
 * reads instead of hand-building a row with no ownership data — it must
 * agree with `rowsFromRepo`'s own branch-row icon fields for the identical
 * chat, since the two draw the same chat in two different places (the tree
 * and Recents) and a divergence between them is exactly the bug this file
 * exists to prevent.
 */
describe('chatIconIndex', () => {
  it("agrees with rowsFromRepo's own icon fields for a workspace-owning chat", () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'feature/x', status: 'pr-conflicts' },
      { id: 'branch-chat-1' },
    )
    const repo = makeTestRepo({ workspaces: [workspace], chats: [chat] })

    const treeRow = rowsFromRepo(repo).find((r) => r.id === 'branch-chat-1')
    const icon = chatIconIndex([repo]).get('branch-chat-1')

    expect(icon).toEqual({
      kind: treeRow?.kind,
      ownsWorktree: treeRow?.ownsWorktree,
      branchName: treeRow?.branchName,
      added: treeRow?.added,
      deleted: treeRow?.deleted,
      locked: treeRow?.locked,
      status: treeRow?.status,
      isPlaceholder: treeRow?.isPlaceholder,
      needsProvisioning: treeRow?.needsProvisioning,
      heldByPath: treeRow?.heldByPath,
      placeholderReason: treeRow?.placeholderReason,
    })
    expect(icon?.kind).toBe('branch')
    expect(icon?.status).toBe('pr-conflicts')
  })

  it('a chat that owns no workspace is absent from the index', () => {
    const repo = makeTestRepo({ chats: [makeTestChat({ id: 'bubble-1', title: 'Just a chat' })] })
    expect(chatIconIndex([repo]).has('bubble-1')).toBe(false)
  })

  it('a fresh fork whose Workspace record has not landed yet still marks ownership, with no decoration', () => {
    // Mirrors rowsFromRepo's own "ownedWorkspaceId resolved but no Workspace
    // node" branch — resolveOwnerOfChat only needs Chat.ownsWorktree, not a
    // Workspace record, to know this chat owns something.
    const chat = makeTestChat({
      id: 'fresh-fork-chat',
      title: 'New fork',
      workspaceId: 'ws-not-here-yet',
      ownsWorktree: true,
    })
    const repo = makeTestRepo({ chats: [chat] })
    expect(chatIconIndex([repo]).get('fresh-fork-chat')).toEqual({
      kind: 'branch',
      ownsWorktree: true,
    })
  })
})
