import { useState } from 'react'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { ScrollArea } from '@/components/ui/scroll-area'
import { useSidebarStore } from '@/lib/store/sidebar'
import { cn } from '@/lib/utils'
import { ROW_BASE } from './workspace-row-base'
import { WorkspaceTreeFooter } from './workspace-tree-footer'
import { WorkspaceTreeItem } from './workspace-tree-item'
import { WorkspaceTreeProvider, useWorkspaceTreeContext } from './workspace-tree-context'
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
      while (cursor) {
        if (cursor.workspace.id === ws.id) { cycle = true; break }
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
  const pathname = useRouterState({ select: s => s.location.pathname })
  const repos = useSidebarStore(s => s.repos)
  const [collapsedRepos, setCollapsedRepos] = useState<Set<string>>(new Set())
  const { draggingWs, dropOnRepo } = useWorkspaceTreeContext()

  const activeWorkspaceId = pathname.match(/\/workspaces\/([^/]+)/)?.[1] ?? ''

  function handleWorkspaceClick(wsId: string) {
    void navigate({ to: '/workspaces/$wsId', params: { wsId } })
  }

  function toggleRepo(repoId: string) {
    setCollapsedRepos(prev => {
      const next = new Set(prev)
      if (next.has(repoId)) next.delete(repoId)
      else next.add(repoId)
      return next
    })
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ScrollArea className="flex-1">
        <div className="py-1">
          {repos.map(repo => {
            const roots = buildWorkspaceTree(repo.workspaces)
            const isCollapsed = collapsedRepos.has(repo.id)
            const isRepoDragOver = draggingWs?.repoId === repo.id
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
                  onClick={() => toggleRepo(repo.id)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleRepo(repo.id) }
                  }}
                  aria-label={isCollapsed ? 'Expand repo' : 'Collapse repo'}
                  onDragOver={(e) => { e.preventDefault(); e.dataTransfer.dropEffect = 'move' }}
                  onDrop={(e) => { e.preventDefault(); dropOnRepo(repo.id) }}
                >
                  <span className={cn('inline-flex h-4 w-4 shrink-0 items-center justify-center rounded px-1 text-[10px] font-bold text-primary-foreground', repo.avatarColor)}>
                    {repo.avatarLabel}
                  </span>
                  <span className="min-w-0 flex-1 truncate text-left font-mono text-muted-foreground/60">
                    {repo.name}
                  </span>
                  <span className="shrink-0 rounded-md p-1 text-foreground/30">
                    <svg
                      aria-hidden="true"
                      className={cn('size-3 transition-transform', !isCollapsed && 'rotate-90')}
                      viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"
                    >
                      <path d="M6 3l5 5-5 5" />
                    </svg>
                  </span>
                </div>
                {!isCollapsed && (
                  <div>
                    {roots.map(node => (
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
