import { describe, expect, it } from 'vitest'
import { rowsFromRepo, chatIconIndex } from '@/components/sidebar/lib/rows-from-repo'
import type { Chat, Repo, Workspace } from '@/lib/store/sidebar'

/**
 * The "Branch needs provisioning" triangle on every repo's own default branch.
 *
 * Shape of the live repro, straight off the wire: a repo whose default
 * workspace is its main folder on `main` (localPath = the repo root), plus a
 * SECOND, worktree-less `main` row whose `heldByPath` is that same root.
 * `adoptRepoHome` adopts the main folder in place on whatever branch it sits
 * on, and provisioning then resolves `holder.HeldByHome` for that same branch
 * and mints this row (the handle the consented detach op acts on, spec §3.5) —
 * so this is the resting state of EVERY imported repo, not a failure.
 *
 * Read as a failed provision it drew a permanent amber warning glyph and fired
 * an error toast per repo per session, pointing at a Retry that
 * `RetryProvision` refuses outright (`ErrBranchStillHeld`).
 */

const REPO_ROOT = '/Users/me/repro-data/repo-beta'
const HOME_ROW_ID = 'home-branch-row'

function ownedWorkspace(
  ws: Partial<Workspace> & { id: string; branch: string },
  chatId: string,
): { workspace: Workspace; chat: Chat } {
  return {
    workspace: { age: '', owningChatId: chatId, ...ws },
    chat: { id: chatId, title: '', repoId: 'r1', order: 0, workspaceId: ws.id, ownsWorktree: true },
  }
}

function repoWithOwnCheckoutRow(branch: string): Repo {
  // The repo's own default branch, with no worktree of its own and the repo
  // root as its holder — c32b82d8 in the live repro.
  const held = ownedWorkspace(
    { id: 'ws-held', branch, status: 'locked', heldByPath: REPO_ROOT },
    'chat-held',
  )
  // An unrelated branch some OTHER worktree holds: a real failure, and the
  // control this fixture needs so a blanket "never warn" would not pass.
  const foreign = ownedWorkspace(
    {
      id: 'ws-foreign',
      branch: 'release/1.x',
      status: 'locked',
      heldByPath: '/Users/me/elsewhere',
    },
    'chat-foreign',
  )
  const home: Chat = {
    id: HOME_ROW_ID,
    title: '',
    repoId: 'r1',
    order: 0,
    workspaceId: 'ws-default',
    ownsWorktree: true,
  }
  return {
    id: 'r1',
    projectId: 'p1',
    name: 'repo-beta',
    avatarLabel: 'R',
    avatarColor: 'bg-indigo-700',
    localPath: REPO_ROOT,
    defaultWorkspaceId: 'ws-default',
    defaultBranch: 'main',
    defaultWorkspaceStatus: 'locked',
    defaultOwningChatId: HOME_ROW_ID,
    workspaces: [held.workspace, foreign.workspace],
    chats: [home, held.chat, foreign.chat],
  }
}

describe("the repo's own default branch, held by the repo's own checkout", () => {
  it('raises no provisioning warning', () => {
    const rows = rowsFromRepo(repoWithOwnCheckoutRow('main'))
    const row = rows.find((r) => r.workspaceId === 'ws-held')
    expect(row?.needsProvisioning).toBe(false)
  })

  // A branch some OTHER worktree holds is a real failure and must still warn —
  // without this the fix reads as "stop warning", which is a different bug.
  it('still warns for a branch held by a worktree that is not the repo home', () => {
    const rows = rowsFromRepo(repoWithOwnCheckoutRow('main'))
    const row = rows.find((r) => r.workspaceId === 'ws-foreign')
    expect(row?.needsProvisioning).toBe(true)
  })

  // Matched by BRANCH, never by comparing `heldByPath` to `Repo.localPath`:
  // git worktree list emits fully symlink-resolved paths while the repo's own
  // path is the folder the user handed the importer, so the same directory
  // routinely has two spellings (/var vs /private/var on macOS).
  it('raises no warning even when the holder path spells the repo root differently', () => {
    const repo = repoWithOwnCheckoutRow('main')
    const workspaces = repo.workspaces.map((w) =>
      w.id === 'ws-held' ? { ...w, heldByPath: '/private/var/repro-data/repo-beta' } : w,
    )
    const row = rowsFromRepo({ ...repo, workspaces }).find((r) => r.workspaceId === 'ws-held')
    expect(row?.needsProvisioning).toBe(false)
  })

  // It still has no worktree, so the verbs that need one stay off it and the
  // remedy it DOES have (Detach, spec §3.5) keeps the holder to act on.
  it('is still a worktree-less row carrying its holder and its reason', () => {
    const rows = rowsFromRepo(repoWithOwnCheckoutRow('main'))
    const row = rows.find((r) => r.workspaceId === 'ws-held')
    expect(row?.isPlaceholder).toBe(true)
    expect(row?.heldByPath).toBe(REPO_ROOT)
    expect(row?.placeholderReason).toContain(REPO_ROOT)
    expect(row?.placeholderReason?.toLowerCase()).not.toContain('retry')
  })

  // Recents draws the same glyph off the same fields, through its own index —
  // a row that stopped warning in the tree must not still warn there.
  it('agrees with Recents’ own icon index', () => {
    const index = chatIconIndex([repoWithOwnCheckoutRow('main')])
    expect(index.get('chat-held')?.needsProvisioning).toBe(false)
    expect(index.get('chat-foreign')?.needsProvisioning).toBe(true)
  })
})
