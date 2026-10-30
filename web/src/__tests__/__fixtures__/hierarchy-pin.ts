import { vi } from 'vitest'
import * as api from '@/lib/api'
import { ensureHomeWorkspaceResolved } from '@/features/workspace/lib/home-workspace-resolver'
import { buildRepoTree, toSidebarChat, toSidebarFolder } from '@/lib/store/build-repo-tree'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useHomeTreeStore } from '@/lib/store/home-tree'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { ChatDTO, FolderDTO, RepoDTO, WorkspaceDTO } from '@/lib/types'

const workspace = (over: Partial<WorkspaceDTO> & Pick<WorkspaceDTO, 'id'>): WorkspaceDTO => ({
  repoId: 'r-alpha',
  projectId: 'p1',
  branch: over.id,
  parentId: '',
  forkPointSha: '',
  status: 'new',
  working: false,
  lastError: '',
  added: 0,
  deleted: 0,
  mergeStrategy: '',
  canMergeLocally: false,
  mergeConflicts: false,
  parentBranch: '',
  prUrl: '',
  prTitle: '',
  prTargetBranch: '',
  provisioning: 'provisioned',
  localPath: `/wt/${over.id}`,
  ...over,
})

const chat = (over: Partial<ChatDTO> & Pick<ChatDTO, 'id' | 'title' | 'order'>): ChatDTO => ({
  repoId: 'r-alpha',
  projectId: 'p1',
  workspaceId: '',
  ownsWorktree: false,
  ...over,
})

const repo = (id: string, name: string, over: Partial<RepoDTO>): RepoDTO => ({
  id,
  projectId: 'p1',
  name,
  path: `/repos/${name}`,
  defaultBranch: 'main',
  avatarLabel: '',
  avatarColor: '',
  avatarUrl: '',
  avatarEmoji: '',
  ...over,
})

const homeFolder = (over: Partial<FolderDTO> & Pick<FolderDTO, 'id' | 'name' | 'order'>) => ({
  repoId: '',
  projectId: 'p1',
  ...over,
})

/** One project `p1` holding every kind of placement the sidebar draws today, as wire DTOs. */
export const PIN = {
  // The project's home workspace, as GET /home answers it; its owning chat is never a row.
  home: workspace({ id: 'home-p1', repoId: '', kind: 'home', owningChatId: 'oc-home' }),
  repos: [
    // Root order 1: between the home chat and the home folder.
    repo('r-alpha', 'alpha', { order: 1, folderId: '' }),
    // Filed into the home folder "Ideas", after the chat filed there.
    repo('r-beta', 'beta', { order: 1, folderId: 'f-ideas' }),
  ],
  workspaces: [
    workspace({ id: 'home-alpha', isDefault: true, branch: 'main', owningChatId: 'oc-alpha' }),
    // Locked and owned by no chat: drawn as its branch name under its workspace id.
    workspace({ id: 'ws-release', branch: 'release/1.0', status: 'locked', order: 1 }),
    workspace({ id: 'ws-feat', branch: 'feature-a', owningChatId: 'oc-feat', order: 2 }),
    // Forked from ws-feat and filed in no folder: nests under its fork parent.
    workspace({
      id: 'ws-fork',
      branch: 'fork-a',
      parentId: 'ws-feat',
      owningChatId: 'oc-fork',
      order: 1,
    }),
    workspace({
      id: 'home-beta',
      repoId: 'r-beta',
      isDefault: true,
      branch: 'main',
      owningChatId: 'oc-beta',
    }),
  ],
  chats: [
    chat({ id: 'oc-alpha', title: '', order: 0, workspaceId: 'home-alpha', ownsWorktree: true }),
    // A conversation in alpha's own checkout: under the repo row, first.
    chat({ id: 'c-alpha-q', title: 'Repo question', order: 0, workspaceId: 'home-alpha' }),
    // Owns ws-feat, so the branch row takes this chat's title and id.
    chat({
      id: 'oc-feat',
      title: 'Feature A',
      order: 0,
      workspaceId: 'ws-feat',
      ownsWorktree: true,
    }),
    // A thread of the branch's owning chat, before the fork.
    chat({
      id: 'c-follow',
      title: 'Follow-up',
      order: 0,
      workspaceId: 'ws-feat',
      parentId: 'oc-feat',
    }),
    chat({
      id: 'oc-fork',
      title: 'Fork of A',
      order: 0,
      workspaceId: 'ws-fork',
      ownsWorktree: true,
    }),
    chat({ id: 'c-try', title: 'Try X', order: 0, workspaceId: 'home-alpha', parentId: 'f-alpha' }),
    chat({
      id: 'oc-beta',
      repoId: 'r-beta',
      title: '',
      order: 0,
      workspaceId: 'home-beta',
      ownsWorktree: true,
    }),
  ],
  // A repo-scoped folder, last among alpha's root children.
  folders: [{ id: 'f-alpha', repoId: 'r-alpha', projectId: 'p1', name: 'Experiments', order: 3 }],
  homeChats: [
    chat({
      id: 'oc-home',
      repoId: '',
      title: '',
      order: 0,
      workspaceId: 'home-p1',
      ownsWorktree: true,
    }),
    // Root order 0: the project's first row.
    chat({ id: 'c-notes', repoId: '', title: 'Notes', order: 0, workspaceId: 'home-p1' }),
    chat({
      id: 'c-draft',
      repoId: '',
      title: 'Draft',
      order: 0,
      workspaceId: 'home-p1',
      parentId: 'f-ideas',
    }),
  ],
  // Root order 2: after the repo.
  homeFolders: [homeFolder({ id: 'f-ideas', name: 'Ideas', order: 2 })],
}

/** The hierarchy `PIN` must render, one `outline` line per row. */
export const PIN_OUTLINE = [
  'Notes [c-notes]',
  'alpha [oc-alpha]',
  '  Repo question [c-alpha-q]',
  '  release/1.0 [ws-release]',
  '  Feature A [oc-feat]',
  '    Follow-up [c-follow]',
  '    Fork of A [oc-fork]',
  '  Experiments [f-alpha]',
  '    Try X [c-try]',
  'Ideas [f-ideas]',
  '  Draft [c-draft]',
  '  beta [oc-beta]',
]

/** Writes `PIN` into the stores the sidebar reads, and answers GET /home with its home. */
export function loadPinFixture(): void {
  vi.spyOn(api, 'fetchHomeWorkspace').mockResolvedValue(PIN.home)
  ensureHomeWorkspaceResolved('p1')
  useSidebarStore
    .getState()
    .setRepos(
      buildRepoTree(
        PIN.repos,
        PIN.workspaces,
        PIN.folders.map(toSidebarFolder),
        PIN.chats.map(toSidebarChat),
      ),
    )
  useFolderSignalStore.setState({ seededRepoIds: new Set(PIN.repos.map((r) => r.id)) })
  useHomeTreeStore.setState({
    trees: {
      p1: {
        chats: PIN.homeChats.map(toSidebarChat),
        folders: PIN.homeFolders.map(toSidebarFolder),
      },
    },
  })
}

/** The sidebar tree's own rows: a project panel's scroll content, minus its Recents band. */
export const SIDEBAR_TREE_ROWS =
  '[data-testid="space-scroll-content"] > :not([data-testid="recents-band"]) [role="treeitem"]'

/** One line per tree row in DOM order: indent by `aria-level`, then label and row id. */
export function outline(root: HTMLElement): string[] {
  return [...root.querySelectorAll<HTMLElement>(SIDEBAR_TREE_ROWS)].map(
    (e) =>
      '  '.repeat(Number(e.getAttribute('aria-level')) - 1) +
      `${e.getAttribute('aria-label')} [${e.dataset.sidebarRowId}]`,
  )
}
