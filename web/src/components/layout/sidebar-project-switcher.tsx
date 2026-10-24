import { memo, useState } from 'react'
import { PlusIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { performRenameProject } from '@/components/sidebar/lib/row-actions'
import type { Project } from '@/lib/types'
import { EditableProjectIcon, ProjectIconMark } from './project-icon-mark'

interface SidebarProjectSwitcherProps {
  /** Every open project. With none and no `onAddProject`, nothing renders. */
  projects: Project[]
  /** The id the route tracks; that project is named, not listed as an icon. */
  activeProjectId?: string
  onSelectProject: (id: string) => void
  /** The tree's only entry point for a SECOND project. */
  onAddProject?: () => void
}

/**
 * First line of the sidebar body: the active project's icon and name, then
 * the other projects' icons and a trailing `+`. The name truncates before the
 * icons are pushed off. The active mark edits the icon; double-clicking the
 * name renames the project. Memoized: every prop is a primitive or a stable callback.
 */
export const SidebarProjectSwitcher = memo(function SidebarProjectSwitcher({
  projects,
  activeProjectId,
  onSelectProject,
  onAddProject,
}: SidebarProjectSwitcherProps) {
  // Holds the project id, so switching spaces mid-edit drops the editor.
  const [renamingId, setRenamingId] = useState<string | null>(null)
  if (projects.length === 0 && !onAddProject) return null

  const active = projects.find((p) => p.id === activeProjectId)
  const others = projects.filter((p) => p.id !== activeProjectId)

  return (
    <div
      data-testid="sidebar-project-switcher"
      className="flex shrink-0 items-center gap-1 px-2 pt-1.5"
    >
      <div className="flex min-w-0 flex-1 items-center gap-2 pl-1">
        {active && (
          <>
            <EditableProjectIcon project={active} size="lg" />
            {renamingId === active.id ? (
              <InlineRenameInput
                defaultValue={active.name}
                onConfirm={(name) => {
                  setRenamingId(null)
                  if (name !== active.name) void performRenameProject(active.id, name)
                }}
                onCancel={() => setRenamingId(null)}
              />
            ) : (
              <span
                data-testid="active-project-name"
                className="min-w-0 truncate font-heading text-[19px] leading-6 font-normal tracking-tight"
                onDoubleClick={() => setRenamingId(active.id)}
              >
                {active.name}
              </span>
            )}
          </>
        )}
      </div>
      {others.map((project) => (
        <Button
          key={project.id}
          onClick={() => onSelectProject(project.id)}
          variant="ghost"
          size="icon-sm"
          data-testid="space-mark"
          className="shrink-0 rounded-sm text-muted-foreground opacity-60 hover:bg-sidebar-element-hover hover:opacity-100"
          tooltip={project.name}
          tooltipSide="bottom"
          aria-label={project.name}
        >
          <ProjectIconMark project={project} size="lg" />
        </Button>
      ))}
      {onAddProject && (
        <Button
          onClick={onAddProject}
          variant="ghost"
          size="icon-sm"
          className="shrink-0 rounded-sm text-muted-foreground hover:bg-sidebar-element-hover"
          tooltip="Add space"
          tooltipSide="bottom"
          aria-label="Add space"
        >
          <PlusIcon size={16} />
        </Button>
      )}
    </div>
  )
})
