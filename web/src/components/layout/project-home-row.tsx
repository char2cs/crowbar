import { memo, useState } from 'react'
import { FolderSymlink } from 'lucide-react'
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
import { AddRepositoryModal } from '@/components/projects/add-repository-modal'
import { ProjectIconPopover } from './project-icon-popover'
import { RowDisclosureButton } from './row-disclosure-button'
import { WorkspaceInlineInput } from './workspace-inline-input'
import { renameProject } from '@/lib/api'
import { toast } from '@/features/window/stores/toast-store'
import { useWorkspaceTreeActions, useWorkspaceTreeDrag } from './workspace-tree-context'
import { dropRowProps } from './drop-target-dom'

interface ProjectHomeRowProps {
  /** The project this row stands for. Every project has a row now, not just the active one. */
  project: { id: string; name: string; avatarUrl?: string; avatarEmoji?: string }
  /** Whether this project's whole tree is folded away. */
  isCollapsed: boolean
  /** Whether it has repos to fold — what the keyboard's Left/Right act on. */
  hasRepos?: boolean
  /**
   * Whether THIS row's name is in inline-rename mode. Owned by the parent so
   * only one row across the whole tree can be renaming at a time — the same
   * arrangement the repo rows' `renamingRepoId` uses.
   */
  isRenaming?: boolean
  startRenaming?: (projectId: string) => void
  stopRenaming?: () => void
}

/**
 * One project's row: the head of its section of the sidebar.
 *
 * It discloses the way EVERY other row in this tree discloses — a leading mark
 * that only ever says what the row is, and a trailing chevron that folds it
 * (RowDisclosureButton). It used to do neither: the leading slot was itself the
 * collapse button, swapping Library⇄chevron on hover, and there was no trailing
 * chevron at all. That made the sidebar's most-repeated control mean two
 * different things one row apart — on a project you pressed the left edge and
 * had to hover to learn that you could, on the repo directly beneath it you
 * pressed the right edge and could see it at rest.
 *
 * The swap is gone with it. It was a `display:none/block` toggle on hover, so
 * crossing a project row changed which box was in the flex flow and forced a
 * layout on a row the pointer was only passing over.
 */
function ProjectHomeRowComponent({
  project,
  isCollapsed,
  hasRepos = false,
  isRenaming = false,
  startRenaming,
  stopRenaming,
}: ProjectHomeRowProps) {
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

  // No stopPropagation here: RowDisclosureButton already stops both the click
  // and the pointerdown, so the row neither navigates nor arms a drag.
  function handleToggle() {
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
        onClick={() => {
          // Clicks inside the inline rename editor must not navigate.
          if (isRenaming) return
          handleClick()
        }}
        onKeyDown={(e) => {
          if (e.target !== e.currentTarget) return
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            handleClick()
          }
        }}
        onPointerDown={(e) => onPointerDownDrag({ kind: 'project', id: project.id }, e)}
      >
        {/* Identity, and the editor for it — the collapse gesture lives at the
            other end of the row. Not a collapse button: while the leading mark
            was one, this row had two collapse affordances competing for the same
            20px and neither of them where every row below put its own.

            ONE slot, 20px (h-5 w-5) so it shares the repo avatars' centre line —
            the two section-header row types line up with each other, and the
            16px glyphs of everything below line up with each other.

            Library as the DEFAULT rather than the only mark: a project holding
            many repos is a collection, and a shelf of spines says that where a
            roof says "dwelling". Outline, because the row already signals
            selection with its raised surface (ROW_ACTIVE) and a filled glyph was
            a second, louder signal in an otherwise all-outline sidebar. */}
        <ProjectIconPopover project={project} working={working} />

        {isRenaming ? (
          <WorkspaceInlineInput
            defaultValue={project.name}
            placeholder="project-name"
            // No `kind` — the default is mono, which is what this row's LABEL is
            // (see the span below). That is the whole rule: the editor reads in
            // the same face as the text it replaced, so the row does not change
            // typeface under the cursor the moment you start typing.
            //
            // It was briefly `prose` on the reasoning that a project name is free
            // text rather than a git ref. True, but beside the point — the folder
            // row can take prose because its label is prose too, and this label
            // is not. Change them together or not at all.
            onConfirm={(name) => {
              stopRenaming?.()
              // The renamed ProjectDTO arrives on the projects WS stream and
              // merges into the store, refreshing this row's name — no
              // optimistic write here; only surface a failure.
              void renameProject(project.id, name).catch((err) => {
                toast.error(err instanceof Error ? err.message : 'Failed to rename project')
              })
            }}
            onCancel={() => stopRenaming?.()}
          />
        ) : (
          /* Clicks bubble, as they do on every other renameable row: `dblclick`
             is delivered only AFTER both of its `click` events, so renaming
             opens project home on the way to the editor and lands back where it
             started. The alternative is holding every row click open for a
             double-click window, which puts a stall in front of the sidebar's
             most-used gesture to pay for its rarest. */
          <span
            className="min-w-0 flex-1 truncate font-mono text-left"
            onDoubleClick={(e) => {
              e.stopPropagation()
              startRenaming?.(project.id)
            }}
          >
            {project.name}
          </span>
        )}

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

        {/* Last in the row, after the row's own actions — the same slot the repo
            header beneath it, and every folder and branch below that, close
            with. The label names the project because this row's own accessible
            name is the project name, and "Collapse" alone would repeat once per
            project in the tree. */}
        <RowDisclosureButton expanded={!isCollapsed} label={project.name} onToggle={handleToggle} />
      </div>
      {/* Explicitly scoped: this row is one of several, so the modal must not
          fall back to whichever project happens to be active. */}
      <AddRepositoryModal open={addRepoOpen} onOpenChange={setAddRepoOpen} projectId={project.id} />
    </>
  )
}

export const ProjectHomeRow = memo(ProjectHomeRowComponent)
