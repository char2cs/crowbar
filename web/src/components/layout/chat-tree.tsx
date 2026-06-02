import { useEffect, useMemo } from 'react'
import { useRouterState, useNavigate } from '@tanstack/react-router'
import { Plus } from '@phosphor-icons/react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/lib/utils'
import { ROW_BASE } from './workspace-row-base'
import { ChatTreeItem, type ChatTreeNode } from './chat-tree-item'
import { ChatTreeProvider, useChatTreeContext } from './chat-tree-context'
import { useSidebarStore, type ProjectChat } from '@/lib/store/sidebar'
import { apiFetch } from '@/lib/api'

function buildChatTree(chats: ProjectChat[]): ChatTreeNode[] {
  const nodeMap = new Map<string, ChatTreeNode>()
  for (const chat of chats) {
    nodeMap.set(chat.id, { chat, children: [] })
  }
  const roots: ChatTreeNode[] = []
  for (const chat of chats) {
    const node = nodeMap.get(chat.id)!
    const parent = chat.parentId ? nodeMap.get(chat.parentId) : undefined
    if (!parent || parent === node) {
      roots.push(node)
    } else {
      let cursor: ChatTreeNode | undefined = parent
      const visited = new Set<string>()
      let cycle = false
      while (cursor) {
        if (cursor.chat.id === chat.id) { cycle = true; break }
        if (visited.has(cursor.chat.id)) { cycle = true; break }
        visited.add(cursor.chat.id)
        cursor = cursor.chat.parentId ? nodeMap.get(cursor.chat.parentId) : undefined
      }
      if (cycle) roots.push(node)
      else parent.children.push(node)
    }
  }
  return roots
}

interface ChatTreeProps {
  wsId: string
}

function ChatTreeInner({ wsId }: ChatTreeProps) {
  const navigate = useNavigate()
  const pathname = useRouterState({ select: s => s.location.pathname })
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1] ?? ''
  const allChats = useSidebarStore(s => s.chats)
  const addChat = useSidebarStore(s => s.addChat)
  const chats = useMemo(() => allChats.filter(c => c.wsId === wsId), [allChats, wsId])
  const { draggingChat, dragPos, hoverTrash } = useChatTreeContext()

  useEffect(() => {
    apiFetch<ProjectChat[]>(`/api/v0/chats?wsId=${wsId}`)
      .then(fetched => {
        const existing = new Set(useSidebarStore.getState().chats.map(c => c.id))
        const fresh = fetched.filter(c => !existing.has(c.id))
        if (fresh.length > 0) {
          fresh.forEach(c => useSidebarStore.getState().addChat(c))
        }
      })
      .catch(() => {})
  }, [wsId])

  const roots = buildChatTree(chats)

  function handleChatClick(chatId: string) {
    void navigate({ to: '/chat/$chatId', params: { chatId } })
  }

  function handleNew() {
    addChat({
      id: crypto.randomUUID(),
      wsId,
      title: 'New chat',
      age: 'just now',
      status: 'idle',
      type: 'chat',
    })
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ScrollArea className="flex-1">
        <div className="py-1">
          {roots.map(node => (
            <ChatTreeItem
              key={node.chat.id}
              node={node}
              depth={0}
              activeChatId={activeChatId}
              onChatClick={handleChatClick}
            />
          ))}
          <button
            type="button"
            aria-label="New chat"
            className={cn(ROW_BASE, 'mx-1.5 border-transparent text-muted-foreground/50 hover:bg-accent hover:text-muted-foreground')}
            onClick={handleNew}
          >
            <Plus size={14} className="shrink-0" />
            <span className="text-[13px]">New chat</span>
          </button>
        </div>
      </ScrollArea>

      {/* Trash zone — slides in during drag */}
      <div className={cn(
        'shrink-0 overflow-hidden transition-[max-height] duration-150 ease-out',
        draggingChat ? 'max-h-16' : 'max-h-0',
      )}>
        <div className="flex items-center justify-center border-t border-border bg-background p-2">
          <div
            data-trash-drop="true"
            className={cn(
              'flex h-10 w-full items-center justify-center gap-2 rounded-lg border border-dashed text-[13px] font-medium transition-colors',
              hoverTrash
                ? 'border-destructive bg-destructive/10 text-destructive'
                : 'border-destructive/40 text-destructive/40',
            )}
          >
            <svg aria-hidden="true" className="size-4 pointer-events-none" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
              <path d="M2 4h12M5 4V2h6v2M6 7v5M10 7v5M3 4l1 9a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1l1-9" />
            </svg>
            Drop to delete
          </div>
        </div>
      </div>

      {/* Drag ghost */}
      {draggingChat && dragPos && (
        <div
          className="pointer-events-none fixed z-50 rounded-md border border-border bg-secondary px-2 py-1 text-[13px] text-secondary-foreground shadow-md opacity-90"
          style={{ left: dragPos.x + 12, top: dragPos.y - 10 }}
        >
          {draggingChat.label}
        </div>
      )}
    </div>
  )
}

export function ChatTree({ wsId }: ChatTreeProps) {
  return (
    <ChatTreeProvider wsId={wsId}>
      <ChatTreeInner wsId={wsId} />
    </ChatTreeProvider>
  )
}
