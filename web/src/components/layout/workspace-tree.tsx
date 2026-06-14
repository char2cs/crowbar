import { useCallback, useState } from 'react'
import { Settings } from 'lucide-react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { InlineError } from '@/components/ui/inline-error'
import { cn } from '@/lib/utils'
import { ROW_BASE } from './workspace-row-base'
import { WorkspaceTreeFooter } from './workspace-tree-footer'
import { WorkspaceTreeItem } from './workspace-tree-item'
import { WorkspaceTreeProvider, useWorkspaceTreeContext } from './workspace-tree-context'
import { RepoSettingsPanel } from './repo-settings-panel'
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
  const { draggingWs, dragPos, hoverTargetId } = useWorkspaceTreeContext()
  const wsListData = useWorkspaceListStore((s) => s.data)
  const retryWorkspaces = useCallback(() => {
    void useWorkspaceListStore.getState().fetch()
  }, [])
  const [hoveredRepoId, setHoveredRepoId] = useState<string | null>(null)
  const [openSettingsRepoId, setOpenSettingsRepoId] = useState<string | null>(null)

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1] ?? ''

  function handleWorkspaceClick(wsId: string) {
    void navigate({ to: '/workspaces/$wsId', params: { wsId } })
  }

  if (wsListData.status === 'error' && repos.length === 0) {
    return (
      <div className="flex flex-1 flex-col overflow-hidden">
        <InlineError error={wsListData.error} onRetry={retryWorkspaces} />
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ScrollArea className="flex-1">
        <div className="py-1">
          {repos.map((repo) => {
            const roots = buildWorkspaceTree(repo.workspaces)
            const isCollapsed = collapsedRepos.has(repo.id)
            const isRepoDragOver = hoverTargetId === `repo:${repo.id}`
            return (
              <div key={repo.id} className="mb-1">
                <div
                  role="button"
                  tabIndex={0}
                  className={cn(
                    ROW_BASE,
                    'border-transparent text-foreground hover:bg-accent',
                    isRepoDragOver && 'ring-1 ring-ring',
                  )}
                  onClick={() => useSidebarStore.getState().toggleRepo(repo.id)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') {
                      e.preventDefault()
                      useSidebarStore.getState().toggleRepo(repo.id)
                    }
                  }}
                  onMouseEnter={() => setHoveredRepoId(repo.id)}
                  onMouseLeave={() => setHoveredRepoId(null)}
                  aria-label={isCollapsed ? 'Expand repo' : 'Collapse repo'}
                  data-repo-drop={repo.id}
                >
                  {repo.avatarURL ? (
                    <img
                      src={repo.avatarURL}
                      alt={repo.name}
                      className="h-4 w-4 shrink-0 rounded object-cover"
                    />
                  ) : (
                    <span
                      className={cn(
                        'inline-flex h-4 w-4 shrink-0 items-center justify-center rounded px-1 text-[10px] font-bold text-primary-foreground',
                        repo.avatarColor,
                      )}
                    >
                      {repo.avatarLabel}
                    </span>
                  )}
                  <span className="min-w-0 flex-1 truncate text-left font-mono text-foreground">
                    {repo.name}
                  </span>
                  {hoveredRepoId === repo.id ? (
                    <button
                      aria-label="Repo settings"
                      className="shrink-0 rounded-md p-1 text-foreground/50 hover:text-foreground"
                      onClick={(e) => {
                        e.stopPropagation()
                        setOpenSettingsRepoId(repo.id)
                      }}
                    >
                      <Settings className="size-3" />
                    </button>
                  ) : (
                    <span className="shrink-0 rounded-md p-1 text-foreground/30">
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
                    </span>
                  )}
                </div>
                {!isCollapsed && (
                  <div>
                    {roots.map((node) => (
                      <WorkspaceTreeItem
                        key={node.workspace.id}
                        node={node}
                        depth={0}
                        repoId={repo.id}
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
      {openSettingsRepoId != null && (() => {
        const settingsRepo = repos.find((r) => r.id === openSettingsRepoId)
        if (!settingsRepo) return null
        return (
          <RepoSettingsPanel
            key={settingsRepo.id}
            repoId={settingsRepo.id}
            repoName={settingsRepo.name}
            open={openSettingsRepoId === settingsRepo.id}
            onOpenChange={(open) => { if (!open) setOpenSettingsRepoId(null) }}
          />
        )
      })()}
      {draggingWs && dragPos && (
        <div
          className="pointer-events-none fixed z-50 rounded-md border border-border bg-secondary px-2 py-1 font-mono text-[13px] text-secondary-foreground shadow-md opacity-90"
          style={{ left: dragPos.x + 12, top: dragPos.y - 10 }}
        >
          {draggingWs.label}
        </div>
      )}
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
