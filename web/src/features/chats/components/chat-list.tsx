import { nanoid } from 'nanoid'
import { Plus } from '@phosphor-icons/react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/utils/cn'
import { ROW_BASE } from '@/components/layout/workspace-row-base'
import { useSidebarStore } from '@/lib/store/sidebar'
import { ChatListItem } from './chat-list-item'

export function ChatList() {
  const chats = useSidebarStore(s => s.chats)
  const addChat = useSidebarStore(s => s.addChat)

  function handleNew() {
    addChat({ id: nanoid(), title: 'New chat', age: 'just now' })
  }

  return (
    <ScrollArea className="flex-1">
      <div className="py-1">
        {chats.map(chat => (
          <ChatListItem
            key={chat.id}
            chat={chat}
            isActive={false} // TODO: wire to active chat ID from routing/store
            onClick={() => {}} // TODO: navigate to chat
          />
        ))}
        <button
          type="button"
          aria-label="New chat"
          className={cn(ROW_BASE, 'border-transparent text-muted-foreground/50 hover:bg-accent hover:text-muted-foreground')}
          onClick={handleNew}
        >
          <Plus size={14} className="shrink-0" />
          <span className="text-[13px]">New</span>
        </button>
      </div>
    </ScrollArea>
  )
}
