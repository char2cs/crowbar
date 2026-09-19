import { beforeEach, expect, test } from 'vitest'
import { useSidebarStore, getInitialState, type Repo } from '@/lib/store/sidebar'
import { getOwningChatId, __resetWorkspaceScopesForTest } from '@/lib/workspace-scope'
import { buildRepoTree } from '@/lib/store/build-repo-tree'
import type { RepoDTO, WorkspaceDTO } from '@/lib/types'

// REGRESSION (restyle v2): every chat-addressed URL for a workspace —
// files, git, terminals, LSP, the seven worktree verbs — is built off
// `getOwningChatId(wsId)` (workspace-scope-url.ts), and `useWorkspaceEffects`
// refuses to load the explorer/git at all until it is non-null
// (`chatScopeReady`). The sidebar's SEED path is what records it for every
// tree workspace (`recordRepoScopes`, sidebar.ts) — but for a repo's DEFAULT
// workspace it records the scope WITHOUT the owning chat, even though the
// very same `Repo` carries it as `defaultOwningChatId` (lifted by
// `toSidebarRepo` off `WorkspaceDTO.owningChatId` for exactly this row). The
// only writer left is `applyWorkspaceDTO` on a LIVE `worktree_state` frame for
// the default workspace, which nothing guarantees ever arrives. So after a
// cold boot, opening the repo's own checkout (a thread on the repo header row
// navigates straight into it) leaves the file tree stuck loading, git blank
// and every terminal throwing "no owning chat recorded".

const REPO: Repo = {
  id: 'repo-1',
  projectId: 'proj-1',
  name: 'repo-alpha',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'ws-main',
  defaultBranch: 'main',
  defaultOwningChatId: 'chat-main',
  workspaces: [{ id: 'ws-feat', branch: 'feat/x', age: '', owningChatId: 'chat-feat' }],
}

beforeEach(() => {
  useSidebarStore.setState(getInitialState())
  __resetWorkspaceScopesForTest()
})

test("setRepos records the DEFAULT workspace's owning chat, not just its project/repo", () => {
  useSidebarStore.getState().setRepos([REPO])
  // A tree workspace's owning chat is recorded — the default's must be too.
  expect(getOwningChatId('ws-feat')).toBe('chat-feat')
  expect(getOwningChatId('ws-main')).toBe('chat-main')
})

test('the seed path end to end: a WorkspaceDTO seed with isDefault resolves the owning chat', () => {
  const repoDTO: RepoDTO = {
    id: 'repo-1',
    projectId: 'proj-1',
    name: 'repo-alpha',
    path: '/x',
    avatarLabel: 'R',
    avatarColor: 'bg-indigo-700',
  } as RepoDTO
  const ws = (id: string, over: Partial<WorkspaceDTO>): WorkspaceDTO =>
    ({
      id,
      repoId: 'repo-1',
      projectId: 'proj-1',
      branch: id,
      parentId: '',
      forkPointSha: '',
      status: 'locked',
      working: false,
      lastError: '',
      added: 0,
      deleted: 0,
      mergeStrategy: 'merge',
      canMergeLocally: false,
      mergeConflicts: false,
      parentBranch: '',
      prUrl: '',
      prTitle: '',
      prTargetBranch: '',
      ...over,
    }) as WorkspaceDTO
  const repos = buildRepoTree(
    [repoDTO],
    [
      ws('ws-main', { isDefault: true, owningChatId: 'chat-main' }),
      ws('ws-feat', { owningChatId: 'chat-feat' }),
    ],
  )
  useSidebarStore.getState().setRepos(repos)
  expect(getOwningChatId('ws-main')).toBe('chat-main')
})
