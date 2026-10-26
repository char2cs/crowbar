import { useState } from 'react'
import { CHAT_TITLE_TEXT } from '@/components/ui/chat-title-text'
import { performRenameChat } from '@/components/sidebar/lib/row-actions'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { useChatBranchHeaderModel } from '@/features/tabs/hooks/use-chat-branch-header-model'
import { ChatBranchHeaderView } from './chat-branch-header-view'

interface ChatBranchHeaderProps {
  chatId: string
  /** The chat's own workspace (see pane-container.tsx's `wsId` resolution) —
   *  `null` when it hasn't resolved yet, in which case only the title shows. */
  wsId: string | null
  /** No fixed height/padding of its own — this renders in two different
   *  contexts (its own row atop the chat view, or nested inside
   *  ChatOnlyPaneHeader's row alongside other buttons) that size it
   *  differently. The caller owns sizing entirely. */
  className?: string
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
export function ChatBranchHeader({ chatId, wsId, className }: ChatBranchHeaderProps) {
  const model = useChatBranchHeaderModel(chatId, wsId)
  const [renaming, setRenaming] = useState(false)
  // A null title is a gap with nothing to edit from, so it offers no rename.
  const title = model.title

  return (
    <ChatBranchHeaderView
      model={model}
      className={className}
      onDoubleClick={() => {
        if (title !== null) setRenaming(true)
      }}
      titleSlot={
        renaming && title !== null ? (
          <InlineRenameInput
            defaultValue={title}
            className={CHAT_TITLE_TEXT}
            onConfirm={(name) => {
              setRenaming(false)
              if (name !== title) void performRenameChat(chatId, name)
            }}
            onCancel={() => setRenaming(false)}
          />
        ) : undefined
      }
    />
  )
}
