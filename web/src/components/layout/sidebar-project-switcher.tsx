import { memo, useState } from 'react'
import { PlusIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { performRenameProject } from '@/components/sidebar/lib/row-actions'
import { WithTooltip } from '@/components/ui/tooltip'
import { HostStatusIcon } from '@/features/hosts/components/host-status-icon'
import { useHostView } from '@/features/hosts/hooks/use-host-status'
import { hostDotLabel, hostTone, hostToneText } from '@/features/hosts/lib/host-tone'
import { LOCAL_HOST } from '@/features/hosts/types'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { cn } from '@/lib/utils'
import type { Project } from '@/lib/types'
import { EditableProjectIcon, ProjectIconMark } from './project-icon-mark'

// One footprint for every mark on the line, so the active project, the others
// and the `+` share a hit area (proportioned to the 19px name).
const TILE = 'flex size-7 shrink-0 items-center justify-center rounded-[8px]'

// Shared with the rename input so entering edit mode never changes the text.
const PROJECT_NAME_TEXT = 'font-heading text-[19px] leading-6 font-normal tracking-tight'

// 10px on a 12px line: under the 24px name it fits the h-11 line without clipping either.
const HOST_LINE =
  'flex min-w-0 max-w-full cursor-pointer items-center gap-1 self-start rounded-sm text-[10px]/[12px] ' +
  'outline-none focus-visible:ring-1 focus-visible:ring-ring'

/** A remote project's host under its name — its computer and dot, its name and, short of
 *  online, its state in its tone — and the way into Settings → Hosts. */
function ProjectHostLine({ hostId }: { hostId: string }) {
  const { name, state } = useHostView(hostId)
  const stateWord = hostDotLabel(state)
  const online = hostTone(state) === 'online'
  const label = `Host settings: ${name === undefined ? '' : `${name}, `}${stateWord}`
  const text = name === undefined ? stateWord : online ? name : `${name} · ${stateWord}`
  const tone = online
    ? { className: cn(HOST_LINE, 'text-muted-foreground hover:text-foreground') }
    : hostToneText(state, HOST_LINE)

  return (
    <WithTooltip
      content={label}
      trigger={
        <button
          type="button"
          data-control="host-settings"
          {...tone}
          aria-label={label}
          onClick={() => useUIState.getState().openSettingsDialog('hosts')}
        >
          <HostStatusIcon
            hostId={hostId}
            state={state}
            weight="bold"
            showTitle={false}
            className="size-3"
          />
          <span className="truncate">{text}</span>
        </button>
      }
    />
  )
}

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
 * First line of the sidebar body: the active project's icon and name (its host under it, when
 * remote), then the other projects' icons and a trailing `+`. The name truncates before the
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
      className="flex h-11 shrink-0 items-center gap-2 overflow-hidden px-3"
    >
      <div className="flex min-w-0 flex-1 items-center gap-2">
        {active && (
          <>
            <span className={TILE}>
              <EditableProjectIcon project={active} size="lg" />
            </span>
            <div className="flex min-w-0 flex-1 flex-col">
              {renamingId === active.id ? (
                <InlineRenameInput
                  defaultValue={active.name}
                  className={PROJECT_NAME_TEXT}
                  onConfirm={(name) => {
                    setRenamingId(null)
                    if (name !== active.name) void performRenameProject(active.id, name)
                  }}
                  onCancel={() => setRenamingId(null)}
                />
              ) : (
                <span
                  data-testid="active-project-name"
                  className={cn('min-w-0 truncate', PROJECT_NAME_TEXT)}
                  onDoubleClick={() => setRenamingId(active.id)}
                >
                  {active.name}
                </span>
              )}
              {active.hostId !== LOCAL_HOST.id && <ProjectHostLine hostId={active.hostId} />}
            </div>
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
          className={cn(TILE, 'p-0 opacity-80 hover:bg-sidebar-element-hover hover:opacity-100')}
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
          className={cn(TILE, 'p-0 text-muted-foreground hover:bg-sidebar-element-hover')}
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
