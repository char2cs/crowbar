import { ChevronsUpDown } from 'lucide-react'
import { useProjectStore } from '@/lib/store/projects'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { ProjectSwitcherPanel } from './project-switcher-panel'

/**
 * Thin entry-point row at the top of the Workspaces tab. Renders as a hairline
 * rule with the active project name sitting inside the line (right-aligned);
 * clicking it opens the project switcher as a pushed sidebar screen.
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
      className="group flex w-full items-center gap-2 px-3 py-2 text-muted-foreground transition-colors hover:text-foreground"
    >
      {/* Hairline that fills the empty space to the left of the label, so the
          project name reads as sitting inside the divider line. */}
      <span className="h-px flex-1 bg-border/60" aria-hidden="true" />
      <ChevronsUpDown className="size-3 shrink-0 text-muted-foreground/60 group-hover:text-foreground" />
      <span className="shrink-0 truncate font-mono text-xs">
        {activeProject?.name ?? 'Select project'}
      </span>
    </button>
  )
}
