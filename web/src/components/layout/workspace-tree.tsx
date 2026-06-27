import { useCallback, useMemo } from 'react'
import { Settings } from 'lucide-react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { InlineError } from '@/components/ui/inline-error'
import { cn } from '@/lib/utils'
import { ROW_BASE, ROW_ACTIVE, ROW_INACTIVE, ADD_GLYPH_PATH } from './workspace-row-base'
import { WorkspaceInlineInput } from './workspace-inline-input'
import { findWorkspaceForBranch } from '@/lib/workspace/branch-workspace'
import { WorkspaceTreeFooter } from './workspace-tree-footer'
import { WorkspaceTreeItem } from './workspace-tree-item'
import { PendingCreateRow } from './pending-create-row'
import {
  WorkspaceTreeProvider,
  useWorkspaceTreeActions,
  useWorkspaceTreeDrag,
} from './workspace-tree-context'
import { RepoSettingsPanel } from './repo-settings-panel'
import { ProjectSwitcherRow } from './project-switcher-row'
import { ProjectHomeRow } from './project-home-row'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import type { Workspace } from '@/lib/store/sidebar'

export interface WorkspaceTreeNode {
  workspace: Workspace
  children: WorkspaceTreeNode[]
}

export function buildWorkspaceTree(workspaces: Workspace[]): WorkspaceTreeNode[] {
  const nodeMap = new Map<string, WorkspaceTreeNode>()
  for (const ws of workspaces) {
    nodeMap.set(ws.id, { workspace: ws, children: [] })
  }

  const roots: WorkspaceTreeNode[] = []
  for (const ws of workspaces) {
    const node = nodeMap.get(ws.id)!
    const parent = ws.parentId ? nodeMap.get(ws.parentId) : undefined

    if (!parent || parent === node) {
      roots.push(node)
    } else {
      let cursor: WorkspaceTreeNode | undefined = parent
      let cycle = false
      const visited = new Set<string>()
      while (cursor) {
        if (cursor.workspace.id === ws.id) {
          cycle = true
          break
        }
        if (visited.has(cursor.workspace.id)) {
          cycle = true
          break
        }
        visited.add(cursor.workspace.id)
        cursor = cursor.workspace.parentId ? nodeMap.get(cursor.workspace.parentId) : undefined
      }
      if (cycle) roots.push(node)
      else parent.children.push(node)
    }
  }
  return roots
}

function WorkspaceTreeInner() {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const repos = useSidebarStore((s) => s.repos)
  const collapsedRepos = useSidebarStore((s) => s.collapsedRepos)
  const { creatingChildOf, startCreating, confirmCreate, cancelCreate, pendingCreates, clearPendingCreate } =
    useWorkspaceTreeActions()
  const { hoverTargetId } = useWorkspaceTreeDrag()
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
    <div className="flex flex-1 flex-col overflow-hidden">
      <ProjectHomeRow />
      <ProjectSwitcherRow />
      <ScrollArea className="flex-1">
        <div className="py-1">
          {repos.map((repo) => {
            const roots = rootsByRepo.get(repo.id) ?? []
            const isCollapsed = collapsedRepos.has(repo.id)
            const isRepoDragOver = hoverTargetId === `repo:${repo.id}`
            return (
              <div key={repo.id} className="mb-1">
                <div
                  role="button"
                  tabIndex={0}
                  className={cn(
                    ROW_BASE,
                    'group',
                    activeWorkspaceId !== '' && activeWorkspaceId === repo.defaultWorkspaceId
                      ? ROW_ACTIVE
                      : ROW_INACTIVE,
                    isRepoDragOver && 'ring-1 ring-ring',
                  )}
                  data-repo-drop={repo.id}
                  aria-label={`Open ${repo.name}`}
                  onClick={() => {
                    if (repo.projectId && repo.defaultWorkspaceId) {
                      handleWorkspaceClick(repo.defaultWorkspaceId, repo.projectId, repo.id)
                    }
                  }}
                  onKeyDown={(e) => {
                    if (
                      e.target === e.currentTarget &&
                      (e.key === 'Enter' || e.key === ' ') &&
                      repo.projectId &&
                      repo.defaultWorkspaceId
                    ) {
                      e.preventDefault()
                      handleWorkspaceClick(repo.defaultWorkspaceId, repo.projectId, repo.id)
                    }
                  }}
                >
                  {repo.avatarURL?.startsWith('emoji:') ? (
                    <span className="pointer-events-none inline-flex h-5 w-5 shrink-0 items-center justify-center text-lg leading-none">
                      {repo.avatarURL.slice(6)}
                    </span>
                  ) : repo.avatarURL ? (
                    <img
                      src={repo.avatarURL}
                      alt={repo.name}
                      draggable={false}
                      className="pointer-events-none h-5 w-5 shrink-0 rounded-md object-cover"
                    />
                  ) : (
                    <span
                      className={cn(
                        'pointer-events-none inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-md px-1 text-[11px] font-bold text-primary-foreground',
                        repo.avatarColor,
                      )}
                    >
                      {repo.avatarLabel}
                    </span>
                  )}
                  <span className="flex min-w-0 flex-1 items-baseline gap-1.5">
                    <span className="shrink-0 font-mono text-foreground">{repo.name}</span>
                    {repo.defaultWorkspaceId && (
                      <span className="hidden min-w-0 truncate font-mono text-[11px] text-foreground/40 group-hover:inline">
                        - default
                      </span>
                    )}
                  </span>

                  <button
                    type="button"
                    aria-label="Repo settings"
                    className="inline-flex shrink-0 cursor-pointer rounded-md p-1 text-foreground/50 hover:text-foreground"
                    onClick={(e) => {
                      e.stopPropagation()
                      useSidebarNavStore.getState().push({
                        id: `repo-settings:${repo.id}`,
                        title: repo.name,
                        component: (
                          <RepoSettingsPanel
                            projectId={repo.projectId ?? ''}
                            repoId={repo.id}
                            repoName={repo.name}
                          />
                        ),
                      })
                    }}
                  >
                    <Settings className="size-3" />
                  </button>

                  {repo.defaultWorkspaceId && (
                    <button
                      type="button"
                      aria-label="Add child workspace"
                      className="shrink-0 cursor-pointer rounded-md p-1 text-foreground/30 hover:text-foreground/60 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                      onClick={(e) => {
                        e.stopPropagation()
                        if (collapsedRepos.has(repo.id)) useSidebarStore.getState().toggleRepo(repo.id)
                        startCreating(repo.id, repo.defaultWorkspaceId!)
                      }}
                    >
                      <svg
                        aria-hidden="true"
                        className="size-3"
                        viewBox="0 0 16 16"
                        fill="none"
                        stroke="currentColor"
                        strokeWidth="2"
                        strokeLinecap="round"
                      >
                        <path d={ADD_GLYPH_PATH} />
                      </svg>
                    </button>
                  )}

                  <button
                    type="button"
                    aria-label={isCollapsed ? 'Expand repo' : 'Collapse repo'}
                    className="inline-flex shrink-0 cursor-pointer rounded-md p-1 text-foreground/30 hover:text-foreground"
                    onClick={(e) => {
                      e.stopPropagation()
                      useSidebarStore.getState().toggleRepo(repo.id)
                    }}
                  >
                    <svg
                      aria-hidden="true"
                      className={cn('size-3 transition-transform', !isCollapsed && 'rotate-90')}
                      viewBox="0 0 16 16"
                      fill="none"
                      stroke="currentColor"
                      strokeWidth="2"
                      strokeLinecap="round"
                    >
                      <path d="M6 3l5 5-5 5" />
                    </svg>
                  </button>
                </div>
                {!isCollapsed && (
                  <div>
                    {creatingChildOf?.repoId === repo.id &&
                      creatingChildOf?.parentId === repo.defaultWorkspaceId && (
                        <div style={{ paddingLeft: 14 }}>
                          <div className={cn(ROW_BASE, 'border-transparent text-foreground')}>
                            <svg
                              aria-hidden="true"
                              className="size-4 shrink-0 text-foreground/30"
                              viewBox="0 0 16 16"
                              fill="none"
                              stroke="currentColor"
                              strokeWidth="2"
                              strokeLinecap="round"
                            >
                              <path d={ADD_GLYPH_PATH} />
                            </svg>
                            <WorkspaceInlineInput
                              onConfirm={confirmCreate}
                              onCancel={cancelCreate}
                              resolveExisting={(b) => findWorkspaceForBranch(repo, b)}
                              onOpenExisting={(wsId) => {
                                cancelCreate()
                                if (repo.projectId) handleWorkspaceClick(wsId, repo.projectId, repo.id)
                              }}
                            />
                          </div>
                        </div>
                      )}
                    {Array.from(pendingCreates.entries())
                      .filter(
                        ([, p]) => p.repoId === repo.id && p.parentId === repo.defaultWorkspaceId,
                      )
                      .map(([tempId, pending]) => (
                        <PendingCreateRow
                          key={tempId}
                          tempId={tempId}
                          pending={pending}
                          paddingLeft={14}
                          onClear={clearPendingCreate}
                        />
                      ))}
                    {roots.map((node) => (
                      <WorkspaceTreeItem
                        key={node.workspace.id}
                        node={node}
                        depth={0}
                        repoId={repo.id}
                        projectId={repo.projectId ?? ''}
                        activeWorkspaceId={activeWorkspaceId}
                        onWorkspaceClick={handleWorkspaceClick}
                      />
                    ))}
                  </div>
                )}
              </div>
            )
          })}
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
