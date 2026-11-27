import { useState } from 'react'
import { CHAT_TITLE_TEXT } from '@/components/ui/chat-title-text'
import { performRenameChat } from '@/components/sidebar/lib/row-actions'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { useChatBranchHeaderModel } from '@/features/tabs/hooks/use-chat-branch-header-model'
import { ChatBranchHeaderView } from './chat-branch-header-view'

interface ChatBranchHeaderProps {
  chatId: string
  wsId: string | null
  className?: string
}

export function ChatBranchHeader({ chatId, wsId, className }: ChatBranchHeaderProps) {
  const model = useChatBranchHeaderModel(chatId, wsId)
  const [renaming, setRenaming] = useState(false)
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
