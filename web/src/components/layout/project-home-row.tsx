import { useState } from 'react'
import { House, FolderPlus } from '@phosphor-icons/react'
import { ChevronsUpDown } from 'lucide-react'
import { useNavigate, useMatch } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { ROW_BASE, ROW_ACTIVE, ROW_INACTIVE } from './workspace-row-base'
import { useProjectStore, useProjectDataStore, EMPTY_PROJECTS } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { dataOf } from '@/lib/loadable'
import { WorkspaceAgentSpinner } from './workspace-branch-icon'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { ProjectSwitcherPanel } from './project-switcher-panel'
import { AddRepositoryModal } from '@/components/projects/add-repository-modal'

function handleOpenSwitcher(e: React.MouseEvent) {
  e.stopPropagation()
  useSidebarNavStore.getState().push({
    id: 'project-switcher',
    title: 'Projects',
    component: <ProjectSwitcherPanel />,
  })
}

export function ProjectHomeRow() {
  const navigate = useNavigate()
  const projectId = useProjectStore((s) => s.activeProjectId)
  const projects = useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)
  const activeProject = projects.find((p) => p.id === projectId)
  const isActive = useMatch({ from: '/_shell/ide/$projectId/home', shouldThrow: false })
  // The home workspace's agent-working overlay, kept live at project scope (it
  // rides no repo, so no per-repo workspace stream carries it).
  const working = useHomeWorkspaceStore((s) => s.workspace?.working ?? false)
  const [addRepoOpen, setAddRepoOpen] = useState(false)

  function handleClick() {
    if (!projectId) return
    void navigate({ to: '/ide/$projectId/home', params: { projectId } })
  }

  function handleAddRepo(e: React.MouseEvent) {
    e.stopPropagation()
    setAddRepoOpen(true)
  }

  return (
    <>
      <div
        // react-doctor-disable-next-line prefer-tag-over-role -- can't be a real <button>: it contains two nested real <Button>s (Import repository, Switch project) below, and HTML forbids interactive content inside a <button>. role="button" + tabIndex + onKeyDown is the correct fallback for a clickable row with nested action buttons.
        role="button"
        tabIndex={0}
        className={cn(ROW_BASE, 'group', isActive ? ROW_ACTIVE : ROW_INACTIVE)}
        onClick={handleClick}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') handleClick()
        }}
      >
        {/* Match the repo-header rows: the House sits in the same 20px (h-5 w-5)
            box the repo avatar uses, so the label lines up with the repo names
            rather than shifting left off a bare 14px glyph. While an agent works
            in the home workspace the glyph becomes the spinner — the same
            treatment the worktree rows get from WorkspaceBranchIcon. */}
        <span className="inline-flex h-5 w-5 shrink-0 items-center justify-center">
          {working ? (
            <WorkspaceAgentSpinner />
          ) : (
            <House size={16} weight={isActive ? 'fill' : 'regular'} />
          )}
        </span>
        <span className="min-w-0 flex-1 truncate font-mono text-left">
          {activeProject?.name ?? 'home'}
        </span>
        {/* Trailing actions are direct row children (no wrapper) so they inherit
            ROW_BASE's gap-2 and share the repo-header rows' trailing rhythm. */}
        <Button
          variant="ghost"
          size="icon-sm"
          className="h-5 w-5 text-muted-foreground hover:text-foreground"
          tooltip="Import Repository"
          tooltipSide="bottom"
          aria-label="Import repository"
          onClick={handleAddRepo}
        >
          <FolderPlus size={13} />
        </Button>
        <Button
          variant="ghost"
          size="icon-sm"
          className="h-5 w-5 text-muted-foreground hover:text-foreground"
          tooltip="Switch Project"
          tooltipSide="bottom"
          aria-label="Switch project"
          onClick={handleOpenSwitcher}
        >
          <ChevronsUpDown className="size-3" />
        </Button>
      </div>
      <AddRepositoryModal open={addRepoOpen} onOpenChange={setAddRepoOpen} />
    </>
  )
}
