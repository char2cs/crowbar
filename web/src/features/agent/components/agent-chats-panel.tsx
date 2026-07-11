import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useRouterState } from '@tanstack/react-router'
import { useStore } from 'zustand'
import { cn } from '@/lib/utils'
import { ScrollArea } from '@/components/ui/scroll-area'
import { ADD_GLYPH_PATH, ROW_BASE, ROW_INACTIVE } from '@/components/layout/workspace-row-base'
import { parseWorkspaceScopeFromPath } from '@/lib/workspace-scope'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useWorkspaceAgentChatsStream } from '@/features/workspace/stores/hooks/use-workspace-agent-chats-stream'
import { orderedChats } from '@/features/workspace/stores/slices/agent-chats-slice'
import { createChat, deleteChat, renameChat } from '@/features/agent/api/agent-api'
import { AgentChatRow } from './agent-chat-row'
import type { AgentChat, AgentProvider } from '@/features/agent/api/agent-api'

/** Place draggedId immediately before targetId in the full ordered id list. */
export function reorderIds(orderedIds: string[], draggedId: string, targetId: string): string[] {
  if (draggedId === targetId) return orderedIds
  const without = orderedIds.filter((id) => id !== draggedId)
  const idx = without.indexOf(targetId)
  if (idx === -1) return orderedIds
  return [...without.slice(0, idx), draggedId, ...without.slice(idx)]
}

// Same hit test as the workspace tree's drag (components/layout/workspace-tree-context.tsx):
// the drop target is whatever data-* marker sits under the pointer — the trash
// zone, or another chat row (never the dragged row itself).
function findDropTarget(x: number, y: number, draggingId: string): string | null {
  for (const el of document.elementsFromPoint(x, y)) {
    if (el.getAttribute('data-trash-drop') !== null) return TRASH
    const id = el.getAttribute('data-agent-chat-drop')
    if (id !== null && id !== draggingId) return id
  }
  return null
}

const TRASH = 'trash'
const DRAG_THRESHOLD_PX = 5

// A completed drag still produces a click on the row it started from, which
// would select (and open) the dragged chat. Swallow that one click — same trick
// the workspace tree uses.
function suppressNextClick(): void {
  const swallow = (e: MouseEvent) => {
    e.stopPropagation()
    e.preventDefault()
  }
  window.addEventListener('click', swallow, { capture: true, once: true })
  setTimeout(() => window.removeEventListener('click', swallow, { capture: true }), 0)
}

/**
 * The sidebar "Chats" panel: every agent chat in the routed workspace, then one
 * "New <provider> chat" row per provider. Rows drag to reorder (persisted per
 * workspace) and drag onto the footer trash to delete — no confirm dialog.
 *
 * Works for every workspace kind (project home, repo home, worktree): it is
 * driven purely by the routed wsId, and every API/WS URL underneath goes through
 * workspaceBase, which already resolves home workspaces.
 */
export function AgentChatsPanel() {
  const pathname = useRouterState({
    select: (s: { location: { pathname: string } }) => s.location.pathname,
  })
  const wsId = parseWorkspaceScopeFromPath(pathname)?.wsId ?? null

  return wsId ? <AgentChatsPanelInner wsId={wsId} /> : null
}

function AgentChatsPanelInner({ wsId }: { wsId: string }) {
  // Seeds chats + providers and keeps them live (turn spinners, titles, deletes).
  useWorkspaceAgentChatsStream(wsId)
  const store = getOrCreateWorkspaceStore(wsId)

  const chats = useStore(store, (s) => s.agentChats.chats)
  const order = useStore(store, (s) => s.agentChats.order)
  const working = useStore(store, (s) => s.agentChats.working)
  const providers = useStore(store, (s) => s.agentChats.providers)
  const activeChatId = useStore(store, (s) => s.agentChats.activeChatId)

  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [draggingId, setDraggingId] = useState<string | null>(null)
  const [hoverTarget, setHoverTarget] = useState<string | null>(null)

  // The client-persisted row order (localStorage) is not part of the WS seed —
  // read it back on mount, before the first list paint.
  useEffect(() => {
    store.getState().hydrateAgentChatOrder()
  }, [store])

  const ordered = useMemo(() => orderedChats(chats, order), [chats, order])
  const providerIcons = useMemo(() => new Map(providers.map((p) => [p.id, p.icon])), [providers])

  // Read by the window-level pointer handlers, which are registered once — a ref
  // keeps them off the render-identity treadmill (no listener churn per keystroke).
  const orderedRef = useRef(ordered)
  orderedRef.current = ordered

  const openChat = useCallback(
    (chatId: string) => {
      const st = store.getState()
      const title = st.agentChats.chats.find((c) => c.id === chatId)?.title
      st.setActiveAgentChatId(chatId)
      st.bufferActions.openContent({
        type: 'agentChat',
        chatId,
        wsId,
        name: title || 'Agent chat',
      })
    },
    [store, wsId],
  )

  const newChat = useCallback(
    (provider: AgentProvider) => {
      createChat(wsId, provider.id)
        .then((chatId) => {
          const st = store.getState()
          const title = st.agentChats.chats.find((c) => c.id === chatId)?.title
          st.setActiveAgentChatId(chatId)
          st.bufferActions.openContent({
            type: 'agentChat',
            chatId,
            wsId,
            name: title || `${provider.displayName} chat`,
          })
        })
        .catch((err: unknown) => console.error('Failed to create agent chat:', err))
    },
    [store, wsId],
  )

  // Drop-to-delete: remove the row and close its pane tab at once (no confirm),
  // then fire the DELETE. The backend's 'deleted' WS frame replays both removals
  // idempotently; a failed request snaps the chat back into the list.
  const removeChat = useCallback(
    (chatId: string) => {
      const st = store.getState()
      const previous = st.agentChats.chats.find((c) => c.id === chatId)
      const previousOrder = st.agentChats.order
      const buffer = st.buffers.find((b) => b.type === 'agentChat' && b.chatId === chatId)

      st.removeAgentChat(chatId)
      if (buffer) st.bufferActions.closeBuffer(buffer.id)

      deleteChat(wsId, chatId).catch((err: unknown) => {
        console.error('Failed to delete agent chat:', err)
        if (!previous) return
        const snap = store.getState()
        snap.upsertAgentChat(previous)
        snap.setAgentChatOrder(previousOrder)
      })
    },
    [store, wsId],
  )

  const confirmRename = useCallback(
    (chat: AgentChat, title: string) => {
      setRenamingId(null)
      if (title === chat.title) return
      store.getState().upsertAgentChat({ ...chat, title }) // optimistic; WS title_set confirms
      renameChat(wsId, chat.id, title).catch((err: unknown) =>
        console.error('Failed to rename agent chat:', err),
      )
    },
    [store, wsId],
  )

  // Pointer drag (mirrors the workspace tree's): arm on pointer-down, promote to
  // a real drag past the movement threshold, resolve the drop target by hit test.
  const dragRef = useRef<{ id: string; startX: number; startY: number; active: boolean } | null>(
    null,
  )

  useEffect(() => {
    const endDrag = () => {
      dragRef.current = null
      setDraggingId(null)
      setHoverTarget(null)
    }

    const onPointerMove = (e: PointerEvent) => {
      const drag = dragRef.current
      if (!drag) return
      if (!drag.active) {
        if (Math.hypot(e.clientX - drag.startX, e.clientY - drag.startY) <= DRAG_THRESHOLD_PX)
          return
        drag.active = true
        setDraggingId(drag.id)
      }
      setHoverTarget(findDropTarget(e.clientX, e.clientY, drag.id))
    }

    const onPointerUp = (e: PointerEvent) => {
      const drag = dragRef.current
      if (!drag?.active) {
        endDrag()
        return
      }
      // The post-drop click must never double as a row selection.
      suppressNextClick()

      const target = findDropTarget(e.clientX, e.clientY, drag.id)
      if (target === TRASH) {
        removeChat(drag.id)
      } else if (target) {
        const ids = orderedRef.current.map((c) => c.id)
        store.getState().setAgentChatOrder(reorderIds(ids, drag.id, target))
      }
      endDrag()
    }

    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', endDrag)
    return () => {
      window.removeEventListener('pointermove', onPointerMove)
      window.removeEventListener('pointerup', onPointerUp)
      window.removeEventListener('pointercancel', endDrag)
    }
  }, [removeChat, store])

  const onPointerDownDrag = useCallback((chatId: string, e: React.PointerEvent) => {
    if (e.button !== 0) return
    dragRef.current = { id: chatId, startX: e.clientX, startY: e.clientY, active: false }
  }, [])

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      <ScrollArea className="min-h-0 flex-1">
        <div className="py-1">
          {ordered.map((chat) => (
            <AgentChatRow
              key={chat.id}
              chatId={chat.id}
              title={chat.title || 'Untitled chat'}
              providerIcon={providerIcons.get(chat.activeProviderId) ?? ''}
              working={working[chat.id] ?? false}
              active={activeChatId === chat.id}
              renaming={renamingId === chat.id}
              onSelect={() => openChat(chat.id)}
              onStartRename={() => setRenamingId(chat.id)}
              onConfirmRename={(title) => confirmRename(chat, title)}
              onCancelRename={() => setRenamingId(null)}
              onPointerDownDrag={(e) => onPointerDownDrag(chat.id, e)}
            />
          ))}

          {/* One New row per provider, below every real chat, behind a hairline. */}
          {providers.length > 0 && ordered.length > 0 && (
            <div data-new-chat-separator="true" className="mx-3 my-1 border-t border-border/60" />
          )}
          {providers.map((provider) => (
            <NewChatRow key={provider.id} provider={provider} onClick={() => newChat(provider)} />
          ))}
        </div>
      </ScrollArea>

      <TrashFooter dragging={draggingId !== null} isOver={hoverTarget === TRASH} />
    </div>
  )
}

// Provider icon on the left, "New <provider> chat", + on the right edge.
function NewChatRow({ provider, onClick }: { provider: AgentProvider; onClick: () => void }) {
  return (
    <div
      role="button"
      tabIndex={0}
      data-new-chat={provider.id}
      className={cn(ROW_BASE, ROW_INACTIVE, 'text-muted-foreground hover:text-foreground')}
      onClick={onClick}
      onKeyDown={(e) => {
        if (e.key === 'Enter' || e.key === ' ') {
          e.preventDefault()
          onClick()
        }
      }}
    >
      <span
        aria-hidden="true"
        className="flex size-4 shrink-0 items-center justify-center [&>svg]:size-full"
        dangerouslySetInnerHTML={{ __html: provider.icon }}
      />
      <span className="min-w-0 flex-1 truncate">New {provider.displayName} chat</span>
      <svg
        aria-hidden="true"
        data-add-glyph="true"
        className="size-3 shrink-0"
        viewBox="0 0 16 16"
        fill="none"
        stroke="currentColor"
        strokeWidth="2"
        strokeLinecap="round"
      >
        <path d={ADD_GLYPH_PATH} />
      </svg>
    </div>
  )
}

// Mirrors components/layout/workspace-tree-footer.tsx: always mounted (so the
// list doesn't resize on drag start), slid in with a max-height transition.
function TrashFooter({ dragging, isOver }: { dragging: boolean; isOver: boolean }) {
  return (
    <div
      className={cn(
        'shrink-0 overflow-hidden transition-[max-height] duration-150 ease-out',
        dragging ? 'max-h-16' : 'max-h-0',
      )}
    >
      <div className="flex items-center justify-center border-t border-border bg-background p-2">
        <div
          data-trash-drop="true"
          className={cn(
            'flex h-10 w-full items-center justify-center gap-2 rounded-lg border border-dashed text-[13px] font-medium transition-colors',
            isOver
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
  )
}
