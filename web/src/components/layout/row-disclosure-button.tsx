import { cn } from '@/lib/utils'
import { DISCLOSURE_GLYPH_PATH, ROW_SUB_ACTION } from './workspace-row-base'

interface RowDisclosureButtonProps {
  /** Whether the row this closes is currently open. Drives the 90° rotation. */
  expanded: boolean
  /**
   * What the label names, appended to "Collapse"/"Expand" — "repo", a project
   * name. Omit on the tree's own rows, where the row's accessible name already
   * says which one this belongs to.
   */
  label?: string
  onToggle: () => void
}

/**
 * The trailing chevron that folds a row's children away.
 *
 * ONE control for every expandable row in the sidebar — project headers, repo
 * headers, folders and workspaces. The rows differ in what they lead with (a
 * project's Library, a repo's avatar, a folder's folder, a branch's glyph);
 * they must not differ in how they disclose, because that mark is the same
 * promise on all of them and the pointer goes to the same place to press it.
 *
 * `pointerdown` is stopped so pressing the chevron does not arm a drag of the
 * row under it, and `click` is stopped so it does not also open the row's
 * destination.
 */
export function RowDisclosureButton({ expanded, label, onToggle }: RowDisclosureButtonProps) {
  return (
    <button
      type="button"
      className={ROW_SUB_ACTION}
      aria-label={`${expanded ? 'Collapse' : 'Expand'}${label ? ` ${label}` : ''}`}
      onClick={(e) => {
        e.stopPropagation()
        onToggle()
      }}
      onPointerDown={(e) => e.stopPropagation()}
    >
      <svg
        aria-hidden="true"
        className={cn('size-3 transition-transform', expanded && 'rotate-90')}
        viewBox="0 0 16 16"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      >
        <path d={DISCLOSURE_GLYPH_PATH} />
      </svg>
    </button>
  )
}
