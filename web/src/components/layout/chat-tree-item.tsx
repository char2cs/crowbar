import { ChatCircle, Plus } from '@phosphor-icons/react'
import { cn } from '@/lib/utils'
import { ROW_BASE } from './workspace-row-base'
import { WorkspaceAgentSpinner } from './workspace-branch-icon'
import { WorkspaceInlineInput } from './workspace-inline-input'
import { useChatTreeContext } from './chat-tree-context'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { ProjectChat } from '@/lib/store/sidebar'

export interface ChatTreeNode {
  chat: ProjectChat
  children: ChatTreeNode[]
}

interface ChatTreeItemProps {
  node: ChatTreeNode
  depth: number
  activeChatId: string
  onChatClick: (chatId: string) => void
}

export function ChatTreeItem({ node, depth, activeChatId, onChatClick }: ChatTreeItemProps) {
  const { chat, children } = node
  const isActive = chat.id === activeChatId
  const hasChildren = children.length > 0
  const isCollapsed = useSidebarStore(s => s.collapsedChats.has(chat.id))

  const {
    creatingChildOf, startCreating, confirmCreate, cancelCreate,
    renamingId, startRenaming, confirmRename, cancelRename,
    draggingChat, onPointerDownDrag,
  } = useChatTreeContext()

  const isCreatingChild = creatingChildOf?.parentId === chat.id
  const isRenaming = renamingId === chat.id
  const isDraggingThis = draggingChat?.id === chat.id
  const showChildren = (hasChildren && !isCollapsed) || isCreatingChild

  const variant = isActive
    ? 'border-background bg-background text-foreground shadow-xs shadow-black/10 not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)] active:inset-shadow-[0_1px_--theme(--color-black/8%)] active:shadow-none'
    : 'border-transparent text-foreground hover:bg-accent'

  return (
    <div>
      <div style={{ paddingLeft: (depth + 1) * 14 }}>
        <div
          role="button"
          tabIndex={0}
          className={cn(ROW_BASE, variant, isDraggingThis && 'opacity-40')}
          onClick={() => !isRenaming && onChatClick(chat.id)}
          onKeyDown={(e) => {
            if (!isRenaming && (e.key === 'Enter' || e.key === ' ')) {
              e.preventDefault()
              onChatClick(chat.id)
            }
          }}
          onPointerDown={!isRenaming ? (e) => onPointerDownDrag(chat.id, chat.title, e) : undefined}
        >
          <span className="size-4 shrink-0 flex items-center justify-center">
            {chat.status === 'agent-running'
              ? <WorkspaceAgentSpinner />
              : <ChatCircle aria-hidden="true" className="size-4 text-foreground" weight="fill" />
            }
          </span>

          {isRenaming ? (
            <WorkspaceInlineInput
              defaultValue={chat.title}
              placeholder="chat title"
              onConfirm={confirmRename}
              onCancel={cancelRename}
            />
          ) : (
            <span
              className="min-w-0 flex-1 truncate text-left text-[13px]"
              onDoubleClick={(e) => { e.stopPropagation(); startRenaming(chat.id) }}
            >
              {chat.title}
            </span>
          )}

          {!isRenaming && (
            <span className="shrink-0 font-mono text-[11px] text-muted-foreground">{chat.age}</span>
          )}

          {!isRenaming && (hasChildren ? (
            <button
              type="button"
              className="shrink-0 rounded-md p-1 text-foreground/30 hover:text-foreground/60 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              onClick={(e) => { e.stopPropagation(); useSidebarStore.getState().toggleChat(chat.id) }}
              onPointerDown={(e) => e.stopPropagation()}
              aria-label={isCollapsed ? 'Expand' : 'Collapse'}
            >
              <svg
                aria-hidden="true"
                className={cn('size-3 transition-transform', !isCollapsed && 'rotate-90')}
                viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round"
              >
                <path d="M6 3l5 5-5 5" />
              </svg>
            </button>
          ) : !isCreatingChild ? (
            <button
              type="button"
              className="shrink-0 rounded-md p-1 text-foreground/30 hover:text-foreground/60 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
              onClick={(e) => { e.stopPropagation(); startCreating(chat.id) }}
              onPointerDown={(e) => e.stopPropagation()}
              aria-label="Fork chat"
            >
              <Plus aria-hidden="true" className="size-3" />
            </button>
          ) : null)}
        </div>
      </div>

      {showChildren && (
        <div>
          {hasChildren && !isCollapsed && children.map(child => (
            <ChatTreeItem
              key={child.chat.id}
              node={child}
              depth={depth + 1}
              activeChatId={activeChatId}
              onChatClick={onChatClick}
            />
          ))}
          <div style={{ paddingLeft: (depth + 2) * 14 }}>
            {isCreatingChild ? (
              <div className={cn(ROW_BASE, 'border-transparent text-foreground')}>
                <Plus aria-hidden="true" className="size-4 shrink-0 text-foreground/30" />
                <WorkspaceInlineInput
                  placeholder="chat title"
                  onConfirm={confirmCreate}
                  onCancel={cancelCreate}
                />
              </div>
            ) : (
              <div
                role="button"
                tabIndex={0}
                className={cn(ROW_BASE, 'border-transparent text-muted-foreground/40 hover:bg-accent hover:text-muted-foreground/60')}
                onClick={() => startCreating(chat.id)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); startCreating(chat.id) }
                }}
              >
                <Plus aria-hidden="true" className="size-4 shrink-0" />
                <span className="text-[13px]">New fork</span>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
