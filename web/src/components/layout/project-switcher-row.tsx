import { ChevronRight } from 'lucide-react'
import { useProjectStore } from '@/lib/store/projects'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { ProjectSwitcherPanel } from './project-switcher-panel'

/**
 * Thin entry-point row at the top of the Workspaces tab. Shows the active
 * project and opens the project switcher as a pushed sidebar screen.
 */
export function ProjectSwitcherRow() {
  const projects = useProjectStore((s) => s.projects)
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  const activeProject = projects.find((p) => p.id === activeProjectId)

  function open() {
    useSidebarNavStore.getState().push({
      id: 'project-switcher',
      title: 'Projects',
      component: <ProjectSwitcherPanel />,
    })
  }

  return (
    <button
      type="button"
      onClick={open}
      className="flex w-full items-center gap-2 border-b border-border/60 px-3 py-2 text-left text-xs text-muted-foreground transition-colors hover:text-foreground"
    >
      <span className="min-w-0 flex-1 truncate font-mono">
        {activeProject?.name ?? 'Select project'}
      </span>
      <ChevronRight className="size-3.5 shrink-0 text-muted-foreground/60" />
    </button>
  )
}
