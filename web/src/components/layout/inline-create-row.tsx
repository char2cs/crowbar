import { cn } from '@/lib/utils'
import { WorkspaceInlineInput } from './workspace-inline-input'
import { findWorkspaceForBranch } from '@/lib/workspace/branch-workspace'
import {
  ADD_GLYPH_PATH,
  CREATE_ROW_PLACEHOLDER,
  ROW_BASE,
  ROW_INDENT_TRANSITION,
  ROW_SUB_ACTION_GLYPH,
} from './workspace-row-base'
import type { Repo } from '@/lib/store/sidebar'

interface InlineCreateRowProps {
  /** Indent (px) so the input sits where the new row will. */
  indent: number
  /** For the "that branch already has a workspace" hint; absent skips it. */
  repo?: Repo
  onConfirm: (value: string) => void
  onCancel: () => void
  onOpenExisting: (wsId: string) => void
}

/**
 * The one input both new-row kinds go through.
 *
 * A trailing slash means folder; anything else is a workspace. One input rather
 * than two buttons, because the two are the same act at the same place in the
 * tree, and a second control would have to explain which level it added to —
 * which is the question the standing "New" row could never answer.
 *
 * `flex-col` on the container so the row STRETCHES to the width minus its own
 * `mx-1.5` (flex stretch respects margins). `w-full` would force width:100% AND
 * keep the 6px side margins, overflowing the panel into a stray scrollbar.
 */
export function InlineCreateRow({
  indent,
  repo,
  onConfirm,
  onCancel,
  onOpenExisting,
}: InlineCreateRowProps) {
  return (
    <div
      className={cn('flex flex-col', ROW_INDENT_TRANSITION)}
      style={{ marginInlineStart: indent }}
    >
      <div className={cn(ROW_BASE, 'border-transparent text-foreground')}>
        <svg
          aria-hidden="true"
          className={cn('size-4', ROW_SUB_ACTION_GLYPH)}
          viewBox="0 0 16 16"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
        >
          <path d={ADD_GLYPH_PATH} />
        </svg>
        <WorkspaceInlineInput
          placeholder={CREATE_ROW_PLACEHOLDER}
          onConfirm={onConfirm}
          onCancel={onCancel}
          resolveExisting={(branch) => (repo ? findWorkspaceForBranch(repo, branch) : null)}
          onOpenExisting={onOpenExisting}
        />
      </div>
    </div>
  )
}
