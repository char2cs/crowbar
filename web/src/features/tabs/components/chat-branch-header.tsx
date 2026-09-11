import { GitBranch } from '@phosphor-icons/react'
import { useState } from 'react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { performRenameChat } from '@/components/sidebar/lib/row-actions'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { formatChangeCount } from '@/components/layout/format-change-count'
import {
  ROW_GLYPH_BOX,
  ROW_SUBLABEL,
  ROW_SUBLABEL_ADD,
  ROW_SUBLABEL_DEL,
} from '@/components/layout/workspace-row-base'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { useWorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { useSidebarStore } from '@/lib/store/sidebar'

interface ChatBranchHeaderProps {
  chatId: string
  /** The chat's own workspace (see pane-container.tsx's `wsId` resolution) —
   *  `null` when it hasn't resolved yet, in which case only the title shows. */
  wsId: string | null
}

/**
 * The chat interface's own identity header — replaces `ChatHead` at this
 * position now that the chat and the IDE sector are two separate boxes with
 * their own headers. Deliberately NOT a `Tab`: this isn't one of several
 * things to switch between here, so it carries none of the sidebar row's
 * hover/selection chrome, just its rule-6 two-line shape (title, then branch
 * name + change counts) — see sidebar-row.tsx's `BranchSecondLine` for the
 * row this mirrors.
 *
 * Rename is double-click, matching the sidebar's own convention
 * (`sidebar-row.tsx`), routed through the same `performRenameChat` action —
 * a chat's name has one write path regardless of which surface edits it.
 */
export function ChatBranchHeader({ chatId, wsId }: ChatBranchHeaderProps) {
  const title = useWorkspaceStoreContext(
    (s) => s.agentChats.chats.find((c) => c.id === chatId)?.title || UNTITLED_CHAT_LABEL,
  )
  const working = useWorkspaceStoreContext((s) => s.agentChats.working[chatId] ?? false)
  const workspace = useSidebarStore((s) => {
    if (!wsId) return null
    for (const repo of s.repos) {
      const ws = repo.workspaces.find((w) => w.id === wsId)
      if (ws) return ws
    }
    return null
  })
  const [renaming, setRenaming] = useState(false)

  const added = workspace?.added ?? 0
  const deleted = workspace?.deleted ?? 0

  return (
    <div
      data-testid="chat-branch-header"
      className="flex h-8 shrink-0 items-center gap-1.5 px-2.5 text-[13px]"
      onDoubleClick={() => setRenaming(true)}
    >
      <span data-testid="chat-branch-header-glyph" className={ROW_GLYPH_BOX}>
        {working ? (
          <FlickerSpinner className="size-3.5" />
        ) : (
          <GitBranch
            data-testid="chat-branch-header-branch-icon"
            className="size-3.5 text-muted-foreground"
          />
        )}
      </span>

      {renaming ? (
        <InlineRenameInput
          defaultValue={title}
          onConfirm={(name) => {
            setRenaming(false)
            if (name !== title) void performRenameChat(chatId, name)
          }}
          onCancel={() => setRenaming(false)}
        />
      ) : workspace?.branch ? (
        <span className="flex min-w-0 flex-1 flex-col justify-center">
          <span className="truncate">{title}</span>
          <span className={ROW_SUBLABEL}>
            {workspace.branch}
            {(added > 0 || deleted > 0) && ' -- '}
            {added > 0 && <span className={ROW_SUBLABEL_ADD}>+{formatChangeCount(added)}</span>}
            {added > 0 && deleted > 0 && ' '}
            {deleted > 0 && <span className={ROW_SUBLABEL_DEL}>-{formatChangeCount(deleted)}</span>}
          </span>
        </span>
      ) : (
        <span className="min-w-0 flex-1 truncate">{title}</span>
      )}
    </div>
  )
}
