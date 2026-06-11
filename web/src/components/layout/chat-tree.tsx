import { useEffect, useMemo } from 'react'
import { useRouterState } from '@tanstack/react-router'
import {
  getActiveWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import { Plus } from '@phosphor-icons/react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { cn } from '@/lib/utils'
import { ROW_BASE } from './workspace-row-base'
import { ChatTreeItem, type ChatTreeNode } from './chat-tree-item'
import { ChatTreeProvider, useChatTreeContext, performCreateChat } from './chat-tree-context'
import { useSidebarStore, type ProjectChat } from '@/lib/store/sidebar'
import { useChatListStore } from '@/lib/store/chat-list-store'
import { chatDtoToProjectChat } from '@/lib/api/chat'
import { dataOf } from '@/lib/loadable'
import { toast } from '@/components/ui/toast'

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
        if (cursor.chat.id === chat.id) {
          cycle = true
          break
        }
        if (visited.has(cursor.chat.id)) {
          cycle = true
          break
        }
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
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const activeChatId = pathname.match(/\/chat\/([^/]+)/)?.[1] ?? ''
  const allChats = useSidebarStore((s) => s.chats)
  const chats = useMemo(() => allChats.filter((c) => c.wsId === wsId), [allChats, wsId])
  const { draggingChat, dragPos, hoverTrash } = useChatTreeContext()

  // Fetch via LoadableSlice (IDB-cached, stale-while-revalidate).
  // No fetch without a workspace — an empty wsId would hit /v0/workspaces//chats.
  useEffect(() => {
    if (!wsId) return
    void useChatListStore.getState().fetch(wsId)
  }, [wsId])

  // Seed sidebar store when loadable data arrives
  useEffect(() => {
    return useChatListStore.subscribe((state) => {
      const fetched = dataOf(state.data)
      if (!fetched) return
      const existing = new Set(useSidebarStore.getState().chats.map((c) => c.id))
      const fresh = fetched.filter((c) => !existing.has(c.id))
      if (fresh.length > 0) {
        fresh.forEach((c) => useSidebarStore.getState().addChat(chatDtoToProjectChat(c)))
      }
    })
  }, [])

  const roots = buildChatTree(chats)

  function handleChatClick(chatId: string) {
    const chat = useSidebarStore.getState().chats.find((c) => c.id === chatId)
    if (!chat) return
    // Fall back to the registry store for the workspace in the URL — the
    // active-id pointer is set in a WorkspaceView effect and can lag the
    // first paint of the sidebar.
    const store = getActiveWorkspaceStore() ?? (wsId ? getOrCreateWorkspaceStore(wsId) : null)
    if (!store) {
      toast.error('Open a workspace to view this chat')
      return
    }
    store.getState().bufferActions.openContent({
      type: 'crowbarChat',
      wsId: chatId,
      name: chat.title,
    })
  }

  function handleNew() {
    if (!wsId) {
      toast.error('Open a workspace to create a chat')
      return
    }
    // Creating a chat also opens its tab — same path as clicking the row
    // (the created chat is already in the sidebar store by then).
    void performCreateChat(wsId, 'New chat').then((chat) => {
      if (chat) handleChatClick(chat.id)
    })
  }

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ScrollArea className="flex-1">
        <div className="py-1">
          {roots.map((node) => (
            <ChatTreeItem
              key={node.chat.id}
              node={node}
              depth={0}
              activeChatId={activeChatId}
              onChatClick={handleChatClick}
            />
          ))}
          <div
            role="button"
            tabIndex={0}
            aria-label="New chat"
            className={cn(
              ROW_BASE,
              'border-transparent text-muted-foreground/40 hover:bg-accent hover:text-muted-foreground/60',
            )}
            onClick={handleNew}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                handleNew()
              }
            }}
          >
            <Plus size={14} className="shrink-0" />
            <span className="text-[13px]">New chat</span>
          </div>
        </div>
      </ScrollArea>

      {/* Trash zone — slides in during drag */}
      <div
        className={cn(
          'shrink-0 overflow-hidden transition-[max-height] duration-150 ease-out',
          draggingChat ? 'max-h-16' : 'max-h-0',
        )}
      >
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
            <svg
              aria-hidden="true"
              className="size-4 pointer-events-none"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              strokeWidth="1.5"
              strokeLinecap="round"
              strokeLinejoin="round"
            >
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
    <ChatTreeProvider>
      <ChatTreeInner wsId={wsId} />
    </ChatTreeProvider>
  )
}
