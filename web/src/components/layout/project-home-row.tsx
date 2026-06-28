import { useState } from 'react'
import { House, FolderPlus } from '@phosphor-icons/react'
import { ChevronsUpDown } from 'lucide-react'
import { useNavigate, useMatch } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { ROW_BASE, ROW_ACTIVE, ROW_INACTIVE } from './workspace-row-base'
import { useProjectStore } from '@/lib/store/projects'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { ProjectSwitcherPanel } from './project-switcher-panel'
import { AddRepositoryModal } from '@/components/projects/add-repository-modal'

export function ProjectHomeRow() {
  const navigate = useNavigate()
  const projectId = useProjectStore((s) => s.activeProjectId)
  const projects = useProjectStore((s) => s.projects)
  const activeProject = projects.find((p) => p.id === projectId)
  const isActive = useMatch({ from: '/_shell/ide/$projectId/home', shouldThrow: false })
  const [addRepoOpen, setAddRepoOpen] = useState(false)

  function handleClick() {
    if (!projectId) return
    void navigate({ to: '/ide/$projectId/home', params: { projectId } })
  }

  function handleOpenSwitcher(e: React.MouseEvent) {
    e.stopPropagation()
    useSidebarNavStore.getState().push({
      id: 'project-switcher',
      title: 'Projects',
      component: <ProjectSwitcherPanel />,
    })
  }

  function handleAddRepo(e: React.MouseEvent) {
    e.stopPropagation()
    setAddRepoOpen(true)
  }

  return (
    <>
      <div
        role="button"
        tabIndex={0}
        className={cn(ROW_BASE, 'group pr-1', isActive ? ROW_ACTIVE : ROW_INACTIVE)}
        onClick={handleClick}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') handleClick()
        }}
      >
        <House size={14} weight={isActive ? 'fill' : 'regular'} className="shrink-0" />
        <span className="min-w-0 flex-1 truncate font-mono text-left">
          {activeProject?.name ?? 'home'}
        </span>
        <div className="flex shrink-0 items-center">
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
      </div>
      <AddRepositoryModal open={addRepoOpen} onOpenChange={setAddRepoOpen} />
    </>
  )
}
