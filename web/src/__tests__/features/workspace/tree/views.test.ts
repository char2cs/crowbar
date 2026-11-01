import { describe, expect, it } from 'vitest'
import type { Chat, Repo, Workspace } from '@/lib/store/sidebar'
import { chatIconIndex } from '@/components/sidebar/lib/chat-icon-index'
import {
  HOME_ROW_ID,
  makeFolder,
  makeOwnedWorkspace,
  makeRepo,
  makeWorkspace,
  repoRows,
} from '@/__tests__/__fixtures__/tree-rows'

describe('a repo', () => {
  it('a locked branch becomes a branch-kind row', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'develop', status: 'locked' },
      { id: 'branch-chat-1' },
    )
    const repo = makeRepo({ workspaces: [workspace], chats: [chat] })
    const rows = repoRows(repo)
    const row = rows.find((r) => r.workspaceId === 'ws-1')
    expect(row?.kind).toBe('branch')
    expect(row?.ownsWorktree).toBe(true)
    expect(row?.locked).toBe(true)
    // Exactly one row — this is the fold this whole file pins, not a
    // coincidence of the fixture.
    expect(rows).toHaveLength(1)
  })

  // `RowGlyph` (sidebar-row.tsx) delegates to `WorkspaceBranchIcon` off exactly these two fields —
  // a row that dropped either would silently fall back to the plain GitBranch/Lock guess, which is
  // the bug this pins: a workspace stuck as a placeholder (no on-disk worktree) or carrying a PR
  it('carries the workspace status and placeholder fact onto the row', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'feature/x', status: 'pr-conflicts', provisioning: 'placeholder' },
      { id: 'branch-chat-1' },
    )
    const repo = makeRepo({ workspaces: [workspace], chats: [chat] })
    const row = repoRows(repo).find((r) => r.workspaceId === 'ws-1')
    expect(row?.status).toBe('pr-conflicts')
    // A placeholder by the daemon's own record, same as a workspace whose
    // worktree is held elsewhere.
    expect(row?.isPlaceholder).toBe(true)
  })

  it('is not a placeholder once the workspace has a real on-disk worktree', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'feature/x', status: 'new', localPath: '/repo/feature-x' },
      { id: 'branch-chat-1' },
    )
    const repo = makeRepo({ workspaces: [workspace], chats: [chat] })
    const row = repoRows(repo).find((r) => r.workspaceId === 'ws-1')
    expect(row?.isPlaceholder).toBe(false)
  })

  it('a chat folder becomes a folder-kind row', () => {
    const repo = makeRepo({
      folders: [makeFolder({ id: 'f-1', name: 'Bugs' })],
    })
    const rows = repoRows(repo)
    expect(rows.find((r) => r.id === 'f-1')?.kind).toBe('folder')
  })

  // A repo folder always sits under a real worktree, unlike a project-home
  // one (deriveRows) — its own "+" always forks a branch.
  it('a repo folder owns a worktree, so its own "+" can fork a branch', () => {
    const repo = makeRepo({
      folders: [makeFolder({ id: 'f-1', name: 'Bugs' })],
    })
    const rows = repoRows(repo)
    expect(rows.find((r) => r.id === 'f-1')?.ownsWorktree).toBe(true)
  })

  it('the default workspace becomes the one root row, labelled with the repo name', () => {
    const repo = makeRepo({ defaultWorkspaceId: 'ws-home', defaultBranch: 'main' })
    const rows = repoRows(repo)
    const home = rows.find((r) => r.id === HOME_ROW_ID)
    expect(home?.kind).toBe('branch')
    expect(home?.parentId).toBeNull()
    expect(home?.label).toBe('crowbar')
    expect(home?.branchName).toBe('main')
  })

  it('a root-level workspace nests under the default workspace', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeWorkspace({ id: 'ws-1', branch: 'feature/x' })],
    })
    const rows = repoRows(repo)
    expect(rows.find((r) => r.id === 'ws-1')?.parentId).toBe(HOME_ROW_ID)
  })

  it('a forked workspace nests under its fork parent, not the default workspace', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [
        makeWorkspace({ id: 'ws-1', branch: 'feature/x' }),
        makeWorkspace({ id: 'ws-2', branch: 'feature/x/child', parentId: 'ws-1' }),
      ],
    })
    const rows = repoRows(repo)
    expect(rows.find((r) => r.id === 'ws-2')?.parentId).toBe('ws-1')
  })

  it('drops a workspace whose status is a deleted tombstone', () => {
    const repo = makeRepo({
      workspaces: [makeWorkspace({ id: 'ws-1', branch: 'gone', status: 'deleted' })],
    })
    const rows = repoRows(repo)
    expect(rows.find((r) => r.workspaceId === 'ws-1')).toBeUndefined()
  })

  it('produces no rows for a repo with nothing yet', () => {
    expect(repoRows(makeRepo())).toEqual([])
  })

  // Task 5 (icon personalization): the home row's own icon (EditableRepoIcon, repo-icon-mark.tsx)
  // needs the repo's ids and avatar fields to reach the right REST base — see SidebarRow's own doc
  // on the `repoIcon` field.
  describe('the home row’s repoIcon', () => {
    it('carries the repo’s own identity once its owning project has seeded', () => {
      const repo = makeRepo({
        id: 'r1',
        projectId: 'p1',
        defaultWorkspaceId: 'ws-home',
        avatarLabel: 'C',
        avatarColor: 'bg-indigo-700',
        avatarURL: 'emoji:🚀',
      })
      const home = repoRows(repo).find((r) => r.id === HOME_ROW_ID)
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
      const repo = makeRepo({ id: 'r1', defaultWorkspaceId: 'ws-home' })
      const home = repoRows(repo).find((r) => r.id === HOME_ROW_ID)
      expect(home?.repoIcon).toBeUndefined()
    })

    it('is absent on every non-home row', () => {
      const repo = makeRepo({
        id: 'r1',
        projectId: 'p1',
        defaultWorkspaceId: 'ws-home',
        workspaces: [makeWorkspace({ id: 'ws-1', branch: 'feature/x' })],
      })
      const child = repoRows(repo).find((r) => r.id === 'ws-1')
      expect(child?.repoIcon).toBeUndefined()
    })
  })
})

// Spec §3.4: a freshly-minted workspace's branch is provisional (italic) until it is renamed — the
// same idea `labelProvisional` already carries for an untitled chat's title, applied to the branch
// half.
describe('provisional branch naming', () => {
  // These two model a workspace with NO resolvable owner — the "genuinely chat-less" fallback (see
  // the describe block above) — where the row's label IS still its branch, so the branch-
  // placeholder pattern is still the right provisional signal.
  it('a genuinely chat-less workspace whose branch is still the generated placeholder renders provisional', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeWorkspace({ id: 'ws-1', branch: 'chat-a1b2c3d4' })],
    })
    const row = repoRows(repo).find((r) => r.id === 'ws-1')
    expect(row?.labelProvisional).toBe(true)
  })

  it('a genuinely chat-less workspace with a real (renamed) branch does not render provisional', () => {
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [makeWorkspace({ id: 'ws-1', branch: 'feature/real-name' })],
    })
    const row = repoRows(repo).find((r) => r.id === 'ws-1')
    expect(row?.labelProvisional).toBeFalsy()
  })

  it('the home row is provisional when the default branch is still generated', () => {
    const repo = makeRepo({ defaultWorkspaceId: 'ws-home', defaultBranch: 'chat-deadbeef' })
    const home = repoRows(repo).find((r) => r.id === HOME_ROW_ID)
    expect(home?.labelProvisional).toBe(true)
  })

  it('the home row is not provisional for a real default branch', () => {
    const repo = makeRepo({ defaultWorkspaceId: 'ws-home', defaultBranch: 'main' })
    const home = repoRows(repo).find((r) => r.id === HOME_ROW_ID)
    expect(home?.labelProvisional).toBeFalsy()
  })

  // A FOLDED (owned, unlocked) fork's label is now its chat's title, not its branch (rule 6) — so
  // its provisional-ness now tracks the same thing a bubble chat's already does (an empty title),
  // regardless of whether the branch itself is still the generated placeholder.
  it('a folded fork with no chat title yet renders provisional, even with a real branch name', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'feature/real-name' },
      { id: 'c-1', title: '' },
    )
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const row = repoRows(repo).find((r) => r.id === 'c-1')
    expect(row?.labelProvisional).toBe(true)
  })

  it('a folded fork with a real chat title does not render provisional, even on a still-generated branch', () => {
    const { workspace, chat } = makeOwnedWorkspace(
      { id: 'ws-1', branch: 'chat-a1b2c3d4' },
      { id: 'c-1', title: 'Fix the parser' },
    )
    const repo = makeRepo({
      defaultWorkspaceId: 'ws-home',
      workspaces: [workspace],
      chats: [chat],
    })
    const row = repoRows(repo).find((r) => r.id === 'c-1')
    expect(row?.labelProvisional).toBeFalsy()
  })
})

// The repo's own default branch, held by the repo's own checkout: the resting state of every
// imported repo, not a failed provision.
const REPO_ROOT = '/Users/me/repro-data/repo-beta'

function heldWorkspace(
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
  const held = heldWorkspace(
    { id: 'ws-held', branch, status: 'locked', heldByPath: REPO_ROOT, provisioning: 'placeholder' },
    'chat-held',
  )
  // An unrelated branch some OTHER worktree holds: a real failure, and the
  // control this fixture needs so a blanket "never warn" would not pass.
  const foreign = heldWorkspace(
    {
      id: 'ws-foreign',
      branch: 'release/1.x',
      status: 'locked',
      heldByPath: '/Users/me/elsewhere',
      provisioning: 'placeholder',
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
    const rows = repoRows(repoWithOwnCheckoutRow('main'))
    const row = rows.find((r) => r.workspaceId === 'ws-held')
    expect(row?.needsProvisioning).toBe(false)
  })

  // A branch some OTHER worktree holds is a real failure and must still warn —
  // without this the fix reads as "stop warning", which is a different bug.
  it('still warns for a branch held by a worktree that is not the repo home', () => {
    const rows = repoRows(repoWithOwnCheckoutRow('main'))
    const row = rows.find((r) => r.workspaceId === 'ws-foreign')
    expect(row?.needsProvisioning).toBe(true)
  })

  // Matched by BRANCH, never by comparing `heldByPath` to `Repo.localPath`: git worktree list emits
  // fully symlink-resolved paths while the repo's own path is the folder the user handed the
  // importer, so the same directory routinely has two spellings (/var vs /private/var on macOS).
  it('raises no warning even when the holder path spells the repo root differently', () => {
    const repo = repoWithOwnCheckoutRow('main')
    const workspaces = repo.workspaces.map((w) =>
      w.id === 'ws-held' ? { ...w, heldByPath: '/private/var/repro-data/repo-beta' } : w,
    )
    const row = repoRows({ ...repo, workspaces }).find((r) => r.workspaceId === 'ws-held')
    expect(row?.needsProvisioning).toBe(false)
  })

  // It still has no worktree, so the verbs that need one stay off it and the
  // remedy it DOES have (Detach, spec §3.5) keeps the holder to act on.
  it('is still a worktree-less row carrying its holder and its reason', () => {
    const rows = repoRows(repoWithOwnCheckoutRow('main'))
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

const chatlessRepo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'repo-alpha',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'ws-main',
  defaultBranch: 'main',
  defaultOwningChatId: '',
  workspaces: [],
  folders: [],
  chats: [],
  ...over,
})

const lockedChatless = {
  id: 'ws-locked',
  branch: 'release',
  age: '',
  order: 0,
  status: 'locked' as const,
  owningChatId: '',
}

describe('chatless workspaces', () => {
  it('ids a chatless home row and a chatless locked branch by their WORKSPACE id', () => {
    const rows = repoRows(chatlessRepo({ workspaces: [lockedChatless] }))
    expect(rows.map((r) => [r.id, r.kind, r.workspaceId])).toEqual([
      ['ws-main', 'branch', 'ws-main'],
      ['ws-locked', 'branch', 'ws-locked'],
    ])
  })

  it('draws a thread started inside a chatless locked branch as its own row while the daemon still reports no owner', () => {
    const thread: Chat = {
      id: 't1',
      repoId: 'r1',
      title: '',
      order: 0,
      parentId: 'ws-locked',
      workspaceId: 'ws-locked',
      ownsWorktree: false,
    }
    const rows = repoRows(chatlessRepo({ workspaces: [lockedChatless], chats: [thread] }))
    expect(rows.map((r) => [r.id, r.kind, r.parentId])).toEqual([
      ['ws-main', 'branch', null],
      ['ws-locked', 'branch', 'ws-main'],
      ['t1', 'chat', 'ws-locked'],
    ])
  })

  // REGRESSION: the daemon used to promote the only chat of a chatless workspace — the thread just
  // started inside it — to the workspace owner, and the thread row vanished into the branch row.
  it('a thread inside a locked branch stays its own row under the recorded owner', () => {
    const owner: Chat = {
      id: 'o1',
      repoId: 'r1',
      title: '',
      order: 0,
      parentId: 'ws-main',
      workspaceId: 'ws-locked',
      ownsWorktree: true,
    }
    const thread: Chat = {
      id: 't1',
      repoId: 'r1',
      title: 'fix the build',
      order: 0,
      parentId: 'o1',
      workspaceId: 'ws-locked',
      ownsWorktree: false,
    }
    const rows = repoRows(
      chatlessRepo({
        workspaces: [{ ...lockedChatless, owningChatId: 'o1' }],
        chats: [owner, thread],
      }),
    )
    expect(rows.map((r) => [r.id, r.kind, r.parentId, r.locked])).toEqual([
      ['ws-main', 'branch', null, false],
      ['o1', 'branch', 'ws-main', true],
      ['t1', 'chat', 'o1', undefined],
    ])
  })

  it('a thread on the repo home stays its own row under the recorded owner', () => {
    const owner: Chat = {
      id: 'o1',
      repoId: 'r1',
      title: '',
      order: 0,
      workspaceId: 'ws-main',
      ownsWorktree: true,
    }
    const thread: Chat = {
      id: 't1',
      repoId: 'r1',
      title: 'fix the build',
      order: 0,
      parentId: 'o1',
      workspaceId: 'ws-main',
      ownsWorktree: false,
    }
    const rows = repoRows(chatlessRepo({ defaultOwningChatId: 'o1', chats: [owner, thread] }))
    expect(rows.map((r) => [r.id, r.kind, r.label, r.parentId])).toEqual([
      ['o1', 'branch', 'repo-alpha', null],
      ['t1', 'chat', 'fix the build', 'o1'],
    ])
  })
})
