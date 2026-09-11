import { useState } from 'react'
import { ArrowElbowDownRight, CaretDown, DotsThree, Plus } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import {
  ROW_BASE,
  ROW_GLYPH_BOX,
  ROW_INACTIVE,
  ROW_SUB_ACTION,
} from '@/components/layout/workspace-row-base'
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from '@/components/ui/dropdown-menu'
import { EditableProjectIcon } from '@/components/layout/project-icon-mark'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { performRenameProject } from '@/components/sidebar/lib/row-actions'
import type { Project } from '@/lib/types'

interface SpaceHeaderProps {
  project: Project
  folded: boolean
  onToggleFold: () => void
  /** Starts a new thread on the project's home workspace — same mechanism
   *  a row's own Thread button uses (`onCreate(homeRowId, 'thread')`). */
  onCreateThread: () => void
  /** Opens the "Import a repo" / "Create a folder" menu. */
  onOpenAddMenu: () => void
  /** Trashes the whole space — `SpacePanel`'s own `onTrashProject`, already
   *  threaded down from `sidebar-tree-surface.tsx` (spec §9: "the space
   *  header for the project" carries a trash too). */
  onDeleteSpace: () => void
}

/**
 * The space header (spec §4): "the `.row` component with different controls."
 * Built on the same layout tokens as SidebarRow (B.2) — ROW_BASE, the size-5
 * glyph box the row's own comment already calls out as a "section header"
 * exception, ROW_SUB_ACTION — rather than wrapping `<SidebarRow>` itself: this
 * row's interaction is a LEADING-slot swap (mark -> chevron) that SidebarRow's
 * trailing-controls-only model has no shape for.
 *
 * Hover is tracked in state, not CSS `group-hover`, because the trailing
 * thread/add-menu buttons are a CONTENT swap (nothing renders at rest), not
 * just a visibility toggle. The leading mark's own swap (icon -> chevron)
 * rides a SECOND, narrower hover state — see `showChevron`'s own doc for why.
 */
export function SpaceHeader({
  project,
  folded,
  onToggleFold,
  onCreateThread,
  onOpenAddMenu,
  onDeleteSpace,
}: SpaceHeaderProps) {
  const [active, setActive] = useState(false)
  // The delete menu's own open state, ORed into the cluster's mount
  // condition below (`active || menuOpen`) — its content renders in a
  // portal outside this row's DOM subtree, so moving the pointer onto it to
  // click an item fires a real `mouseleave` on the row, and without this
  // `active` alone would unmount the whole menu (trigger included) mid-
  // click: the item's text stayed findable a beat longer than its React
  // fiber did, so the click landed on a detached node and silently did
  // nothing — caught live.
  const [menuOpen, setMenuOpen] = useState(false)
  // Whether the pointer is directly over the glyph's OWN hit-target (the
  // size-5 box below), not the row generally — see `showChevron`.
  const [glyphHovered, setGlyphHovered] = useState(false)
  // Folded reports a state rather than offering one (spec §4): the chevron
  // stays even once the pointer, or focus, has moved on.
  //
  // Spec §4: "On hover — the mark's slot becomes a chevron, and an overflow
  // (…) appears." An EARLIER version of this row (and its ancestor,
  // project-home-row.tsx, deleted in the tree retirement — git history
  // cf422bc5) hit exactly this and reverted it: gating the swap on `active`
  // (row-wide hover) meant Task 5's click-to-edit icon (EditableProjectIcon)
  // was swapped out from under the pointer before a click could ever land on
  // it — clickable in principle, unclickable in practice. A prior pass
  // "fixed" that by gating the swap on `folded` alone, which resolved the
  // click-target conflict but dropped the spec's hover behaviour entirely.
  //
  // The actual conflict is narrower than either fix treated it: it is only
  // the GLYPH's own hit-target that must stay the icon (so its own
  // `group-hover/entity-icon` pencil affordance — icon-popover.tsx — stays
  // reachable). Everywhere else on the row, hover can safely become a
  // chevron, since a click there already just folds. `glyphHovered` carves
  // that one hit-target out of `active`: the row-wide hover swap now applies
  // spec's full behaviour, while a pointer sitting exactly on the mark keeps
  // it as the icon it also is.
  const showChevron = folded || (active && !glyphHovered)
  // Double-click-to-rename the project itself — restored from the deleted
  // tree's project-home-row.tsx, which called the same `renameProject` API
  // through `startRenaming`/`isRenaming` state it owned locally, exactly like
  // this. A project has no id in the row-based `SidebarRow[]`/`renamingRowId`
  // space sidebar-tree-chrome.tsx owns (a project is not a row), so this
  // stays local to this one header rather than threading a second concept
  // through that shared state. A real inline `<input>` in place of the
  // label, matching `develop`'s actual behavior — not a modal.
  const [renaming, setRenaming] = useState(false)

  return (
    <div
      role="button"
      tabIndex={0}
      aria-expanded={!folded}
      aria-label={`${folded ? 'Expand' : 'Collapse'} ${project.name}`}
      data-testid="space-header-row"
      // `mt-0` overrides ROW_BASE's `my-0.5` top half (via twMerge — the
      // bottom half stays, spacing this row from whatever follows). This is
      // the FIRST row in the column, directly under SidebarProjectHeader —
      // `my-0.5`'s 2px top margin reads as normal inter-row rhythm
      // everywhere else in the tree, but with nothing above it to justify
      // here it read as unwanted padding under the toolbar.
      className={cn(ROW_BASE, ROW_INACTIVE, 'mt-0 pr-2.5')}
      onMouseEnter={() => setActive(true)}
      onMouseLeave={() => setActive(false)}
      onFocus={() => setActive(true)}
      onBlur={() => setActive(false)}
      onClick={() => {
        // A click inside the inline editor (or on the space it just
        // vacated before React re-renders) must not fold the space.
        if (renaming) return
        onToggleFold()
      }}
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
      <span
        data-testid="space-glyph"
        className={cn(ROW_GLYPH_BOX, 'size-5')}
        onMouseEnter={() => setGlyphHovered(true)}
        onMouseLeave={() => setGlyphHovered(false)}
      >
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
          <EditableProjectIcon project={project} size="lg" />
        )}
      </span>

      {renaming ? (
        <InlineRenameInput
          defaultValue={project.name}
          onConfirm={(name) => {
            setRenaming(false)
            if (name !== project.name) void performRenameProject(project.id, name)
          }}
          onCancel={() => setRenaming(false)}
        />
      ) : (
        /* Clicks bubble straight to the row, opening/closing the fold,
           exactly as every other renameable row's double-click does
           (sidebar-row.tsx, and the deleted project-home-row.tsx before
           it): `dblclick` is delivered only after both of its `click`
           events, so a rename click folds and unfolds the space on its
           way to opening the editor — harmless, since it ends up back
           where it started. */
        <span
          className="min-w-0 flex-1 truncate"
          onDoubleClick={(e) => {
            e.stopPropagation()
            setRenaming(true)
          }}
        >
          {project.name}
        </span>
      )}

      {(active || menuOpen) && (
        <>
          {/* First in the cluster — the fold toggle is the row's own leading
              glyph, so nothing else here competes for "last." Spec §9: "the
              space header for the project" carries a trash too, same as
              every other row. Controlled `open` (rather than leaving it
              uncontrolled) is what lets `menuOpen` keep this whole block
              mounted once opened — see that state's own doc above. */}
          <DropdownMenu open={menuOpen} onOpenChange={setMenuOpen}>
            <DropdownMenuTrigger
              data-testid="delete-menu"
              data-control="delete-menu"
              className={ROW_SUB_ACTION}
              aria-label={`More actions for ${project.name}`}
              onClick={(e) => e.stopPropagation()}
              onPointerDown={(e) => e.stopPropagation()}
            >
              <DotsThree aria-hidden="true" className="size-3.5" weight="bold" />
            </DropdownMenuTrigger>
            <DropdownMenuContent align="start" side="bottom" sideOffset={4}>
              <DropdownMenuItem
                variant="destructive"
                onClick={(e) => {
                  e.stopPropagation()
                  onDeleteSpace()
                }}
              >
                Delete Space
              </DropdownMenuItem>
            </DropdownMenuContent>
          </DropdownMenu>
          {/* Starts a thread on the project's home workspace — same
              mechanism and icon as a row's own Thread button
              (sidebar-row.tsx), just anchored at the project level instead
              of a specific row. */}
          <button
            type="button"
            data-testid="new-thread"
            data-control="thread"
            className={ROW_SUB_ACTION}
            aria-label={`New thread on ${project.name}`}
            onClick={(e) => {
              e.stopPropagation()
              onCreateThread()
            }}
            onPointerDown={(e) => e.stopPropagation()}
          >
            <ArrowElbowDownRight aria-hidden="true" className="size-3" weight="bold" />
          </button>
          {/* Replaces the old "•••" overflow, which opened a menu with
              nothing in it (addendum §4 left it wired for "the next verb
              this surface gets" — see space-scroller.tsx's SpacePanel). */}
          <button
            type="button"
            data-testid="add-menu"
            data-control="add-menu"
            className={ROW_SUB_ACTION}
            aria-label={`Add to ${project.name}`}
            onClick={(e) => {
              e.stopPropagation()
              onOpenAddMenu()
            }}
            onPointerDown={(e) => e.stopPropagation()}
          >
            <Plus aria-hidden="true" className="size-3" weight="bold" />
          </button>
        </>
      )}
    </div>
  )
}
