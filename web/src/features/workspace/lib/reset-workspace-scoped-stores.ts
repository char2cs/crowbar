import { useFileSystemStore } from '@/features/file-system/controllers/store'
import { useGitStore } from '@/features/git/stores/git-store'

/**
 * Clear the GLOBAL workspace-scoped stores (the file tree and the git panel /
 * decorations) so a newly-activated workspace never renders the PREVIOUS
 * workspace's data.
 *
 * These stores are singletons keyed to "the single visible workspace":
 * WorkspaceHost keeps recently-visited workspaces mounted but hidden, and only
 * the active one's watchers (WorkspaceActiveEffects) write these stores. When a
 * hidden workspace flips active, its file explorer / git panel would paint the
 * OUTGOING workspace's contents on the activation frame — a workspace with no
 * data of its own (project home has no git surface and may have an empty tree)
 * or a slow refetch leaves the stale tree on screen (the observed bug: a git
 * worktree's files rendered under home).
 *
 * This must run synchronously AT activation — a layout effect in WorkspaceView,
 * before the browser paints and before the active-only watchers (a passive
 * effect) mount and refetch. Layout effects run before paint and before passive
 * effects, so the wrong workspace's tree is never shown and the clear cannot
 * race the incoming refetch.
 *
 * The clear is a no-op when a store already holds THIS workspace's data, so a
 * warm return that kept its own data is never needlessly wiped.
 */
export function resetWorkspaceScopedStores(wsId: string): void {
  const fs = useFileSystemStore.getState()
  if (fs.rootFolderPath !== wsId) {
    useFileSystemStore.setState({
      rootFolderPath: wsId,
      files: [],
      fileTree: [],
      isFileTreeLoading: true,
    })
  }

  // The git store keys its status/decorations/panel by the wsId it loaded
  // (currentWorkspaceRepoPath). Only reset when it holds a different workspace,
  // so re-activating a workspace whose git data survived isn't wiped. Home has
  // no git surface and never refetches, so without this its git panel would keep
  // showing the outgoing workspace's status/commits.
  const git = useGitStore.getState()
  if (git.currentWorkspaceRepoPath !== wsId) {
    git.actions.reset()
  }
}
