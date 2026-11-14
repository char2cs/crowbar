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
  /** No sizing of its own: it renders both atop the chat view and inside ChatOnlyPaneHeader's
   *  row, so the caller owns it. */
  className?: string
}

/** The chat's identity header: the sidebar row's two-line shape (title, then branch + change
 *  counts; see sidebar-row.tsx's `BranchSecondLine`) without Tab chrome, since nothing here is
 *  switched between. Double-click renames through `performRenameChat`, the one write path. */
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
