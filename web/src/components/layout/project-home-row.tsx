import { useState } from 'react'
import { Library, FolderSymlink } from 'lucide-react'
import { useNavigate, useMatch } from '@tanstack/react-router'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  ROW_BASE,
  ROW_ACTIVE,
  ROW_INACTIVE,
  ROW_SUB_ACTION,
  ROW_NEST_TARGET,
} from './workspace-row-base'
import { useProjectStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useSidebarSelectionStore } from '@/lib/store/sidebar-selection'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { WorkspaceAgentSpinner } from './workspace-branch-icon'
import { AddRepositoryModal } from '@/components/projects/add-repository-modal'
import { useWorkspaceTreeActions, useWorkspaceTreeDrag } from './workspace-tree-context'
import { dropRowProps } from './drop-target-dom'

/**
 * The icon⇄chevron swap, at the spec's measured timings: `transform 0.1s`,
 * `opacity 0.15s`. Both marks share one slot and exactly one of them is ever in
 * the flow, so the label never moves as the swap happens.
 */
const GLYPH_SWAP = '[transition:transform_0.1s,opacity_0.15s]'

interface ProjectHomeRowProps {
  /** The project this row stands for. Every project has a row now, not just the active one. */
  project: { id: string; name: string }
  /** Whether this project's whole tree is folded away. */
  isCollapsed: boolean
  /** Whether it has repos to fold — what the keyboard's Left/Right act on. */
  hasRepos?: boolean
}

/**
 * One project's row: the head of its section of the sidebar.
 *
 * Two gestures, one row. The leading slot folds the project's entire tree;
 * everything else opens project home. There is no trailing chevron — a second
 * collapse control at the far end of the row would be a second answer to a
 * question the glyph already answers, and the row is only 36px wide of meaning.
 *
 * The chevron stays visible for as long as the project is closed, hover or not:
 * a folded project whose only affordance appeared on hover would read as a dead
 * end.
 */
export function ProjectHomeRow({ project, isCollapsed, hasRepos = false }: ProjectHomeRowProps) {
  const navigate = useNavigate()
  const activeProjectId = useProjectStore((s) => s.activeProjectId)
  const homeMatch = useMatch({ from: '/_shell/ide/$projectId/home', shouldThrow: false })
  const isActive = homeMatch?.params.projectId === project.id
  // The home workspace's agent-working overlay. It rides no repo, so no per-repo
  // workspace stream carries it — it is tracked for the ACTIVE project only
  // (useHomeWorkspaceStore has a single slot), which is why the flag is gated on
  // this row being that project's.
  const homeWorking = useHomeWorkspaceStore((s) => s.workspace?.working ?? false)
  const working = homeWorking && activeProjectId === project.id
  const [addRepoOpen, setAddRepoOpen] = useState(false)
  const { onPointerDownDrag } = useWorkspaceTreeActions()
  const { draggingIds, dropTarget } = useWorkspaceTreeDrag()
  const dropMode = dropTarget?.id === project.id ? dropTarget.mode : null

  function handleClick() {
    // A project row never joins a multiselection, but opening one is still a
    // plain click, and a plain click is what clears the lit rows. It leaves the
    // keep set alone — a kept row must not evaporate because you navigated.
    useSidebarSelectionStore.getState().clearSelected()
    // The route is the source of truth (ide-shell syncs activeProjectId back
    // from it), but set it here too: this row is now the project switcher, and
    // without the eager write the context pill lags a navigation behind.
    useProjectStore.getState().setActiveProject(project.id)
    void navigate({ to: '/ide/$projectId/home', params: { projectId: project.id } })
  }

  function handleAddRepo(e: React.MouseEvent) {
    e.stopPropagation()
    setAddRepoOpen(true)
  }

  function handleToggle(e: React.MouseEvent) {
    e.stopPropagation()
    useSidebarStore.getState().toggleProject(project.id)
  }

  return (
    <>
      <div
        role="treeitem"
        // One tab stop for the whole tree; the roving 0 is set in
        // use-tree-keyboard.ts.
        tabIndex={-1}
        aria-expanded={!isCollapsed}
        {...dropRowProps({
          kind: 'project',
          id: project.id,
          label: project.name,
          expanded: !isCollapsed,
          hasChildren: hasRepos,
        })}
        className={cn(
          ROW_BASE,
          'group',
          isActive ? ROW_ACTIVE : ROW_INACTIVE,
          draggingIds.has(project.id) && 'opacity-40',
          dropMode === 'into' && ROW_NEST_TARGET,
        )}
        onClick={handleClick}
        onKeyDown={(e) => {
          if (e.target !== e.currentTarget) return
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            handleClick()
          }
        }}
        onPointerDown={(e) => onPointerDownDrag({ kind: 'project', id: project.id }, e)}
      >
        {/* ONE slot, 20px (h-5 w-5) so it shares the repo avatars' centre line —
            the two section-header row types line up with each other, and the
            16px glyphs of everything below line up with each other.

            Library rather than a house: this row is a project holding many
            repos, and a shelf of spines says "collection" where a roof says
            "dwelling". Outline in both states — the row already signals
            selection with its raised surface (ROW_ACTIVE), so a filled glyph was
            a second, louder signal and the only solid mark in an otherwise
            all-outline sidebar. */}
        <button
          type="button"
          aria-label={isCollapsed ? `Expand ${project.name}` : `Collapse ${project.name}`}
          className="inline-flex size-5 shrink-0 cursor-pointer items-center justify-center rounded-sm text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          onClick={handleToggle}
          onPointerDown={(e) => e.stopPropagation()}
        >
          {working ? (
            <WorkspaceAgentSpinner />
          ) : (
            <>
              {/* `display`, not opacity: exactly one mark occupies the slot at a
                  time, so neither can nudge the other. */}
              <Library
                size={16}
                className={cn(GLYPH_SWAP, isCollapsed ? 'hidden' : 'group-hover:hidden')}
              />
              <svg
                aria-hidden="true"
                className={cn(
                  'size-3',
                  GLYPH_SWAP,
                  isCollapsed ? 'block rotate-0' : 'hidden rotate-90 group-hover:block',
                )}
                viewBox="0 0 16 16"
                fill="none"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
              >
                <path d="M6 3l5 5-5 5" />
              </svg>
            </>
          )}
        </button>

        <span className="min-w-0 flex-1 truncate font-mono text-left">{project.name}</span>

        {/* The shared UI Button, so it gets a real tooltip — this is an icon-only
            24px control with no visible label, and without one the only way to
            learn what it does is to click. It wears the shared ROW_SUB_ACTION
            chrome so it stays identical to the repo-header rows' buttons.

            Density is the reason for the explicit size-6: the class string alone
            has no box, and Button's own `icon-xs` size variant is size-7 that
            only steps down to size-6 at the sm: breakpoint — which would make
            this row's action 28px in a narrow window while every other row's
            stayed 24px. The glyph keeps its own `size-3`: buttonVariants only
            sizes svgs that carry no size-* class of their own.

            No strokeWidth — Lucide's default is the sidebar's weight (see
            workspace-row-base.ts); an override here made this the boldest mark
            on screen. */}
        <Button
          variant="ghost"
          size="icon-xs"
          className={cn(ROW_SUB_ACTION, 'size-6')}
          tooltip="Import repository"
          aria-label="Import repository"
          onPointerDown={(e) => e.stopPropagation()}
          onClick={handleAddRepo}
        >
          {/* Arrow-into-folder, not folder-plus: this brings an existing repo in
              rather than creating an empty one. (Lucide's name says symlink;
              nothing about the mark does.) */}
          <FolderSymlink className="size-3" />
        </Button>
      </div>
      {/* Explicitly scoped: this row is one of several, so the modal must not
          fall back to whichever project happens to be active. */}
      <AddRepositoryModal open={addRepoOpen} onOpenChange={setAddRepoOpen} projectId={project.id} />
    </>
  )
}
