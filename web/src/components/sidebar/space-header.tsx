import { useState } from 'react'
import { CaretDown, DotsThree } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import {
  ROW_BASE,
  ROW_GLYPH_BOX,
  ROW_INACTIVE,
  ROW_SUB_ACTION,
} from '@/components/layout/workspace-row-base'
import { ProjectIconMark } from '@/components/layout/project-icon-mark'
import { RenameDialog } from '@/components/sidebar/rename-dialog'
import { performRenameProject } from '@/components/sidebar/lib/row-actions'
import type { Project } from '@/lib/types'

interface SpaceHeaderProps {
  project: Project
  folded: boolean
  onToggleFold: () => void
  onOverflow: () => void
}

/**
 * The space header (spec §4): "the `.row` component with different controls."
 * Built on the same layout tokens as SidebarRow (B.2) — ROW_BASE, the size-5
 * glyph box the row's own comment already calls out as a "section header"
 * exception, ROW_SUB_ACTION — rather than wrapping `<SidebarRow>` itself: this
 * row's interaction is a LEADING-slot swap (mark -> chevron) that SidebarRow's
 * trailing-controls-only model has no shape for.
 *
 * Hover is tracked in state, not CSS `group-hover`, because the swap replaces
 * the mark's CONTENT, not just a control's visibility.
 */
export function SpaceHeader({ project, folded, onToggleFold, onOverflow }: SpaceHeaderProps) {
  const [active, setActive] = useState(false)
  // Folded reports a state rather than offering one (spec §4): the chevron
  // stays even once the pointer, or focus, has moved on.
  const showChevron = active || folded
  // Double-click-to-rename the project itself — restored from the deleted
  // tree's project-home-row.tsx, which called the same `renameProject` API
  // through `startRenaming`/`isRenaming` state it owned locally, exactly like
  // this. A project has no id in the row-based `SidebarRow[]`/`renamingRowId`
  // space sidebar-tree-chrome.tsx owns (a project is not a row), so this
  // stays local to this one header rather than threading a second concept
  // through that shared state — it reuses the same `RenameDialog` component,
  // per that component's own doc: "the sidebar's one rename gesture."
  const [renaming, setRenaming] = useState(false)

  return (
    <>
      <div
        role="button"
        tabIndex={0}
        aria-expanded={!folded}
        aria-label={`${folded ? 'Expand' : 'Collapse'} ${project.name}`}
        data-testid="space-header-row"
        className={cn(ROW_BASE, ROW_INACTIVE, 'pr-2.5')}
        onMouseEnter={() => setActive(true)}
        onMouseLeave={() => setActive(false)}
        onFocus={() => setActive(true)}
        onBlur={() => setActive(false)}
        onClick={onToggleFold}
        onKeyDown={(e) => {
          // Same guard as SidebarRow (sidebar-row.tsx): a keydown on the nested
          // overflow button bubbles here too, and without this check Enter/Space
          // on that button would fire onToggleFold instead of its own onClick.
          if (e.target !== e.currentTarget) return
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onToggleFold()
          }
        }}
      >
        <span className={cn(ROW_GLYPH_BOX, 'size-5')}>
          {showChevron ? (
            // rotate-180, not SidebarRow's rotate-90+DISCLOSURE_GLYPH_PATH: that
            // chevron toggles between two states of a row's OWN children;
            // this one reports the whole space's fold, matching the task
            // brief's own literal test (`toHaveClass('rotate-180')`).
            <CaretDown
              aria-hidden="true"
              data-testid="chevron"
              className={cn('size-4 transition-transform', folded && 'rotate-180')}
            />
          ) : (
            <ProjectIconMark project={project} size="lg" />
          )}
        </span>

        {/* Clicks bubble straight to the row, opening/closing the fold, exactly
            as every other renameable row's double-click does (sidebar-row.tsx,
            and the deleted project-home-row.tsx before it): `dblclick` is
            delivered only after both of its `click` events, so a rename click
            folds and unfolds the space on its way to opening the editor —
            harmless, since it ends up back where it started. */}
        <span
          className="min-w-0 flex-1 truncate"
          onDoubleClick={(e) => {
            e.stopPropagation()
            setRenaming(true)
          }}
        >
          {project.name}
        </span>

        {active && (
          <button
            type="button"
            data-testid="overflow"
            data-control="overflow"
            className={ROW_SUB_ACTION}
            aria-label={`More options for ${project.name}`}
            onClick={(e) => {
              e.stopPropagation()
              onOverflow()
            }}
            onPointerDown={(e) => e.stopPropagation()}
          >
            <DotsThree aria-hidden="true" className="size-3" weight="bold" />
          </button>
        )}
      </div>
      <RenameDialog
        open={renaming}
        initialValue={project.name}
        onOpenChange={setRenaming}
        onConfirm={(name) => {
          if (name !== project.name) void performRenameProject(project.id, name)
        }}
      />
    </>
  )
}
