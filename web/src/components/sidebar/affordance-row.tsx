import { ChatsCircle, GitBranch } from '@phosphor-icons/react'
import { ROW_SUB_ACTION } from '@/components/layout/workspace-row-base'

interface AffordanceRowProps {
  onCreateThread: () => void
  onCreateWorkspace?: () => void
}

/**
 * The affordance row for empty containers — the only way to create a thread
 * (or workspace, when git-capable). Icon-only, always visible (ROW_SUB_ACTION,
 * not the *_HOVER variant — this icon IS the row's content, not a secondary
 * trailing control).
 *
 * Spec §3.5: "a split control (bubble = makes a chat, git mark = makes a
 * worktree) ... No subtitles, no descriptions, no visible dropdown chrome —
 * just the icon." That is two distinct icon targets side by side, each firing
 * its own action directly, not one icon behind a text menu — the menu
 * language in the spec describes the row's create surface disambiguating
 * between two legal actions, not a literal dropdown widget. Falls back to the
 * single relevant icon when only one action is legal.
 */
export function AffordanceRow({ onCreateThread, onCreateWorkspace }: AffordanceRowProps) {
  if (!onCreateWorkspace) {
    return (
      <button
        type="button"
        className={ROW_SUB_ACTION}
        aria-label="Create new thread"
        onClick={(e) => {
          e.stopPropagation()
          onCreateThread()
        }}
      >
        <ChatsCircle aria-hidden="true" className="size-3" weight="regular" />
      </button>
    )
  }

  return (
    <>
      <button
        type="button"
        data-testid="affordance-thread"
        className={ROW_SUB_ACTION}
        aria-label="Create new thread"
        onClick={(e) => {
          e.stopPropagation()
          onCreateThread()
        }}
      >
        <ChatsCircle aria-hidden="true" className="size-3" weight="regular" />
      </button>
      <button
        type="button"
        data-testid="affordance-workspace"
        className={ROW_SUB_ACTION}
        aria-label="Create new workspace"
        onClick={(e) => {
          e.stopPropagation()
          onCreateWorkspace()
        }}
      >
        <GitBranch aria-hidden="true" className="size-3" weight="fill" />
      </button>
    </>
  )
}
