import { useCallback, useMemo, useState } from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { InlineError } from '@/components/ui/inline-error'
import { WorkspaceTreeFooter } from './workspace-tree-footer'
import { RepoSection } from './repo-section'
import {
  WorkspaceTreeProvider,
  useWorkspaceTreeActions,
  useWorkspaceTreeDrag,
} from './workspace-tree-context'
import { ProjectHomeRow } from './project-home-row'
import { buildWorkspaceTree, type WorkspaceTreeNode } from './workspace-tree-utils'

function WorkspaceTreeInner() {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const repos = useSidebarStore((s) => s.repos)
  const collapsedRepos = useSidebarStore((s) => s.collapsedRepos)
  const {
    creatingChildOf,
    startCreating,
    confirmCreate,
    cancelCreate,
    pendingCreates,
    clearPendingCreate,
    startImport,
  } = useWorkspaceTreeActions()
  const { hoverTargetId } = useWorkspaceTreeDrag()
  // Which repo header row is in inline-rename mode (double-clicked its name),
  // mirroring the branch rows' renamingId — null when none. Owned here, not per
  // RepoSection, so opening one row's rename editor closes any other's.
  const [renamingRepoId, setRenamingRepoId] = useState<string | null>(null)
  const wsListData = useWorkspaceListStore((s) => s.data)
  const retryWorkspaces = useCallback(() => {
    void useWorkspaceListStore.getState().fetch()
  }, [])
  const activeWorkspaceId = pathname.match(/\/ide\/[^/]+\/[^/]+\/([^/]+)/)?.[1] ?? ''

  // Per-repo tree, memoized so a drag/hover re-render (or any unrelated state
  // change) doesn't rebuild every repo's node graph from scratch.
  const rootsByRepo = useMemo(() => {
    const map = new Map<string, WorkspaceTreeNode[]>()
    for (const repo of repos) map.set(repo.id, buildWorkspaceTree(repo.workspaces))
    return map
  }, [repos])

  const handleWorkspaceClick = useCallback(
    (wsId: string, projectId: string, repoId: string) => {
      void navigate({ to: '/ide/$projectId/$repoId/$wsId', params: { projectId, repoId, wsId } })
    },
    [navigate],
  )

  if (wsListData.status === 'error' && repos.length === 0) {
    return (
      <div className="flex flex-1 flex-col overflow-hidden">
        <InlineError error={wsListData.error} onRetry={retryWorkspaces} />
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden" data-oracle-id="workspace-tree">
      <ProjectHomeRow />
      <ScrollArea className="flex-1">
        {/* A real tree, declared as one. The rows were `role="button"`, which
            takes presentational children — so every per-row control nested
            inside them (import, add-child, expand) was stripped of its own
            semantics for assistive tech. `treeitem` is what these rows
            actually are, and unlike `button` it permits interactive
            descendants. */}
        <div role="tree" aria-label="Workspaces" className="pb-1">
          {repos.map((repo) => (
            <RepoSection
              key={repo.id}
              repo={repo}
              roots={rootsByRepo.get(repo.id) ?? []}
              isCollapsed={collapsedRepos.has(repo.id)}
              isRepoDragOver={hoverTargetId === `repo:${repo.id}`}
              collapsedRepos={collapsedRepos}
              activeWorkspaceId={activeWorkspaceId}
              onWorkspaceClick={handleWorkspaceClick}
              renamingRepoId={renamingRepoId}
              setRenamingRepoId={setRenamingRepoId}
              creatingChildOf={creatingChildOf}
              startCreating={startCreating}
              confirmCreate={confirmCreate}
              cancelCreate={cancelCreate}
              pendingCreates={pendingCreates}
              clearPendingCreate={clearPendingCreate}
              startImport={startImport}
            />
          ))}
        </div>
      </ScrollArea>
      <WorkspaceTreeFooter />
    </div>
  )
}

export function WorkspaceTree() {
  return (
    <WorkspaceTreeProvider>
      <WorkspaceTreeInner />
    </WorkspaceTreeProvider>
  )
}
