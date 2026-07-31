import { useCallback, useEffect, useMemo, useState } from 'react'
import { useStore } from 'zustand'
import { cn } from '@/lib/utils'
import { DragGhost, DragGhostChip } from '@/components/layout/drag-ghost'
import { AGENT_CHAT_ROW_HEIGHT } from './agent-chat-drop-geometry'
import { useAgentChatListVirtualizer } from './use-agent-chat-list-virtualizer'
import { useAgentChatListDrag } from './use-agent-chat-list-drag'
import { ADD_GLYPH_PATH, ROW_BASE, ROW_INACTIVE } from '@/components/layout/workspace-row-base'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useActiveWorkspaceState } from '@/features/workspace/stores/hooks/use-active-workspace-state'
import { useWorkspaceAgentChatsStream } from '@/features/workspace/stores/hooks/use-workspace-agent-chats-stream'
import { orderedChats } from '@/features/workspace/stores/slices/agent-chats-slice'
import { createChat, deleteChat, renameChat } from '@/features/agent/api/agent-api'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { openAgentChat } from '@/features/agent/lib/open-agent-chat'
import { AgentChatRow } from './agent-chat-row'
import { reorderIds } from './agent-chats-reorder'
import type { AgentProvider } from '@/features/agent/api/agent-api'

interface AgentChatsPanelProps {
  /**
   * The workspace to list chats for. Optional: when the sidebar host doesn't
   * already hold a wsId it can render <AgentChatsPanel /> bare and the panel
   * tracks the ACTIVE workspace itself.
   */
  wsId?: string
}

/**
 * The sidebar "Chats" panel: every agent chat in the active workspace, then one
 * unified "New chat" row that opens the first enabled provider. Rows drag to
 * reorder (persisted per workspace) and drag onto the footer trash to delete — no
 * confirm dialog.
 *
 * Works for every workspace kind (project home, repo home, worktree). The wsId
 * comes from the active WORKSPACE STORE, not the URL: the project-home route is
 * /ide/:projectId/home, whose wsId is resolved asynchronously and never appears
 * in the path, so a pathname-derived wsId (parseWorkspaceScopeFromPath, which
 * only matches /ide/:p/:r/:w) is null there and the panel rendered nothing.
 * WorkspaceView publishes the active store — and it is what BOTH route shapes
 * render — so useActiveWorkspaceState is reactive on every workspace kind and
 * re-renders this panel whenever the user switches workspace. Every API/WS URL
 * underneath goes through workspaceBase, which already resolves home workspaces.
 */
export function AgentChatsPanel({ wsId }: AgentChatsPanelProps = {}) {
  const activeWsId = useActiveWorkspaceState((s) => s.workspaceId, null)
  const resolved = wsId ?? activeWsId

  // Keyed: a workspace switch must not carry this panel's drag/rename state over.
  return resolved ? <AgentChatsPanelInner key={resolved} wsId={resolved} /> : null
}

function AgentChatsPanelInner({ wsId }: { wsId: string }) {
  // Seeds chats + providers and keeps them live (turn spinners, titles, deletes).
  useWorkspaceAgentChatsStream(wsId)
  const store = getOrCreateWorkspaceStore(wsId)

  const chats = useStore(store, (s) => s.agentChats.chats)
  const order = useStore(store, (s) => s.agentChats.order)
  const providers = useStore(store, (s) => s.agentChats.providers)
  const buffers = useStore(store, (s) => s.buffers)
  // NOTE: the panel deliberately does NOT subscribe to `agentChats.working`.
  // Immer replaces that whole map's reference on every single-chat turn frame,
  // so a panel-level subscription re-rendered the entire list on the feed's
  // hottest event. Each AgentChatRow subscribes to its own working state instead.

  const [renamingId, setRenamingId] = useState<string | null>(null)

  // The client-persisted row order (localStorage) is not part of the WS seed —
  // read it back on mount, before the first list paint.
  useEffect(() => {
    store.getState().hydrateAgentChatOrder()
  }, [store])

  const ordered = useMemo(() => orderedChats(chats, order), [chats, order])
  const providerIcons = useMemo(() => new Map(providers.map((p) => [p.id, p.icon])), [providers])

  // The unified New-chat pick: the FIRST ENABLED provider in priority order —
  // the same rule every New-chat surface follows (selectEnabledProviders). Derived
  // from the already-subscribed `providers` (a referentially stable store slice)
  // via useMemo, rather than subscribing to selectEnabledProviders directly: that
  // selector returns a fresh array each call, which a render-path useStore read
  // would flag as an ever-changing snapshot. `undefined` when nothing is enabled →
  // no row is offered (spec §5.3).
  const primaryProvider = useMemo(() => providers.find((p) => p.enabled), [providers])

  // Window the chat rows: only the visible slice is mounted. scrollRef goes on
  // the overflow-auto scroll container the virtualizer owns.
  const { scrollRef, rowVirtualizer } = useAgentChatListVirtualizer(ordered.length)

  // A row is lit when the chat HAS A TAB, not when it was the last one clicked.
  // The row is a view of the tab strip: close the tab and the row goes dark, open
  // three chats and all three read as open. (The old highlight followed a stored
  // activeChatId that nothing ever cleared, so a closed chat stayed lit forever.)
  const openChatIds = useMemo(
    () => new Set(buffers.flatMap((b) => (b.type === 'agentChat' ? [b.chatId] : []))),
    [buffers],
  )

  // Shared with the New Tab surface's recent-chat rows, so the two can never
  // disagree about what clicking a chat does (open-agent-chat.ts).
  const openChat = useCallback(
    (chatId: string) => openAgentChat(store, wsId, chatId),
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
        .catch((err: unknown) => toastSpawnFailure(err, provider.displayName, 'start'))
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
      const wasActive = st.agentChats.activeChatId === chatId
      const buffer = st.buffers.find((b) => b.type === 'agentChat' && b.chatId === chatId)

      st.removeAgentChat(chatId)
      if (buffer) {
        // Raw closeBuffer is NOT enough — it filters the buffer out of the array but
        // leaves the owning pane's activeBufferId pointing at it, so deleting the chat you
        // were looking at blanks the whole pane (the remaining tabs stay in the bar, but
        // the pane renders its empty state until you click one). removeBufferFromPane first
        // activates an adjacent tab; this is the exact closeTab pattern the WS-stream hook
        // documents, and the reason the backend's own 'deleted' frame cannot repair this
        // afterwards is that we have already removed the buffer here, so its lookup by
        // chatId finds nothing.
        for (const pane of Object.values(st.panes ?? {})) {
          if (pane.bufferIds.includes(buffer.id)) {
            st.paneActions.removeBufferFromPane(pane.id, buffer.id)
          }
        }
        st.bufferActions.closeBuffer(buffer.id)
      }

      deleteChat(wsId, chatId).catch((err: unknown) => {
        console.error('Failed to delete agent chat:', err)
        if (!previous) return
        const snap = store.getState()
        snap.upsertAgentChat(previous)
        snap.setAgentChatOrder(previousOrder)
        // The optimistic close took the chat's pane tab with it — put that back
        // too, or the chat snaps into the list with its open tab silently gone.
        if (buffer) {
          snap.bufferActions.openContent({
            type: 'agentChat',
            chatId,
            wsId,
            name: buffer.name,
          })
        }
        if (wasActive) snap.setActiveAgentChatId(chatId)
      })
    },
    [store, wsId],
  )

  // Takes the chatId (not the chat object) so the row can hold ONE stable
  // callback for its whole life — passing `(chat) => …` per render would give
  // every row a fresh closure each parent render and defeat AgentChatRow's memo.
  const confirmRename = useCallback(
    (chatId: string, title: string) => {
      setRenamingId(null)
      const chat = store.getState().agentChats.chats.find((c) => c.id === chatId)
      if (!chat || title === chat.title) return
      store.getState().upsertAgentChat({ ...chat, title }) // optimistic; WS title_set confirms
      renameChat(wsId, chatId, title).catch((err: unknown) =>
        console.error('Failed to rename agent chat:', err),
      )
    },
    [store, wsId],
  )

  // Stable rename open/close callbacks — same reason as confirmRename above.
  const startRename = useCallback((chatId: string) => setRenamingId(chatId), [])
  const cancelRename = useCallback(() => setRenamingId(null), [])

  // Drag-to-reorder / drag-to-trash lives in its own hook: with the list
  // windowed, the drop target is resolved from scroll geometry (plus edge
  // auto-scroll), not DOM hit-testing (use-agent-chat-list-drag.ts).
  const {
    draggingId,
    hoverSlot,
    isOverTrash,
    ghostRef,
    ghostOriginRef,
    trashRef,
    onPointerDownDrag,
  } = useAgentChatListDrag({
    scrollRef,
    ordered,
    onReorder: (dragId, slot) =>
      store.getState().setAgentChatOrder(
        reorderIds(
          ordered.map((c) => c.id),
          dragId,
          slot,
        ),
      ),
    onDelete: removeChat,
  })

  const draggingChat = draggingId ? ordered.find((c) => c.id === draggingId) : undefined

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      {/* The virtualizer owns this scroll element directly (like the file tree
          beside it). No top padding/border: the drag geometry treats the content
          origin as row 0's box top. */}
      <div ref={scrollRef} data-agent-chat-scroll="true" className="min-h-0 flex-1 overflow-auto">
        {/* Windowed chat region: a spacer of the full list height, with only the
            visible rows absolutely positioned inside it (avoids margin-collapse
            and keeps each row at a clean index * AGENT_CHAT_ROW_HEIGHT offset). */}
        <div className="relative" style={{ height: rowVirtualizer.getTotalSize() }}>
          {/* THE INSERTION LINE — the drag's only affordance, drawn in the GAP
              the row would land in. A ring on a row could say one thing only,
              "in front of this one", which left the end of the list with no
              affordance (and no reachable slot), and lit up the neighbouring row
              for a drop that produced the identical list. A line between rows
              says exactly where the row goes, including after the last one.
              Outside the windowed rows so it survives any scroll position. */}
          {draggingId !== null && hoverSlot !== null && (
            <div
              data-agent-chat-drop-line={hoverSlot}
              aria-hidden="true"
              className="pointer-events-none absolute inset-x-1.5 z-10 h-0.5 -translate-y-1/2 rounded-full bg-ring"
              style={{ top: hoverSlot * AGENT_CHAT_ROW_HEIGHT }}
            />
          )}
          {rowVirtualizer.getVirtualItems().map((virtualItem) => {
            const chat = ordered[virtualItem.index]
            if (!chat) return null
            return (
              <div
                key={chat.id}
                className="absolute inset-x-0 top-0 flex flex-col"
                style={{
                  height: AGENT_CHAT_ROW_HEIGHT,
                  transform: `translateY(${virtualItem.start}px)`,
                }}
              >
                <AgentChatRow
                  wsId={wsId}
                  chatId={chat.id}
                  title={chat.title || 'Untitled chat'}
                  providerIcon={providerIcons.get(chat.activeProviderId) ?? ''}
                  active={openChatIds.has(chat.id)}
                  renaming={renamingId === chat.id}
                  dragging={draggingId === chat.id}
                  onSelect={openChat}
                  onStartRename={startRename}
                  onConfirmRename={confirmRename}
                  onCancelRename={cancelRename}
                  onPointerDownDrag={onPointerDownDrag}
                />
              </div>
            )
          })}
        </div>

        {/* One unified "New chat" row, below every real chat, behind a hairline.
            It opens the first ENABLED provider; when none is enabled there is no
            provider to start, so the row (and its separator) give way to the
            notice that says so — an empty panel that explains nothing was the
            defect here. flex-col so the row STRETCHES to fill the width minus its
            own mx-1.5 (a <button> is shrink-to-fit at display:flex in WebKit). */}
        <div className="flex flex-col">
          {primaryProvider && ordered.length > 0 && (
            <div data-new-chat-separator="true" className="mx-3 my-1 border-t border-border/60" />
          )}
          {primaryProvider ? (
            <NewChatRow provider={primaryProvider} onClick={() => newChat(primaryProvider)} />
          ) : (
            <NoProvidersNotice />
          )}
        </div>
      </div>

      <TrashFooter dropRef={trashRef} dragging={draggingId !== null} isOver={isOverTrash} />

      {draggingChat && (
        <DragGhost ref={ghostRef} origin={ghostOriginRef.current}>
          {/* A chip rather than a clone of the row: this list is windowed, so
              the row a drag started from may not be in the DOM by the time the
              ghost mounts. */}
          <DragGhostChip label={draggingChat.title || 'Untitled chat'} />
        </DragGhost>
      )}
    </div>
  )
}

// What the panel says instead of the New-chat row when NOTHING can start a chat.
//
// Turning every provider off is a setting the user can reach in two clicks, and
// it used to leave this panel completely blank on a fresh worktree — no row, no
// message, nothing to read or click, and no hint that a settings toggle is what
// did it. The chats a workspace already has still list above this; only the
// ability to start a new one is gone, so this says exactly that and names the
// screen that undoes it.
function NoProvidersNotice() {
  return (
    <p
      data-testid="no-providers-notice"
      className="ui-text-xs mx-3 my-2 text-balance text-muted-foreground"
    >
      No providers are enabled, so there is nothing to start a chat with. Turn one on in Settings →
      Providers.
    </p>
  )
}

// The first-enabled provider's icon on the left, the constant "New chat" label,
// + on the right edge. Provider-agnostic by design: one row, not one-per-provider
// (the provider it opens is the first enabled one, matching the New Tab surface's
// "New Chat" action). A real <button> — no nested interactive children (unlike
// AgentChatRow, which conditionally renders a rename <input> and so keeps role="button").
function NewChatRow({ provider, onClick }: { provider: AgentProvider; onClick: () => void }) {
  return (
    <button
      type="button"
      data-new-chat={provider.id}
      // No `w-full`: ROW_BASE is `display:flex` (block-level), so the button
      // already fills the row MINUS its `mx-1.5`. `w-full` forces width:100% of
      // the parent AND keeps the 6px side margins, overflowing the sidebar by
      // 6px → a stray horizontal scrollbar in the Chats panel (regression from
      // the div→button swap; the old <div> never had w-full).
      className={cn(ROW_BASE, ROW_INACTIVE, 'text-muted-foreground hover:text-foreground')}
      onClick={onClick}
    >
      <span
        aria-hidden="true"
        className="flex size-4 shrink-0 items-center justify-center [&>svg]:size-full"
        dangerouslySetInnerHTML={{ __html: provider.icon }}
      />
      {/* text-left counters the <button>'s UA text-align:center (same fix as
          workspace-tree's "Import project" and "New" rows). Without it the
          label centers in the flex-1 span. */}
      <span className="min-w-0 flex-1 truncate text-left">New chat</span>
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
    </button>
  )
}

// Always mounted (so the
// list doesn't resize on drag start), slid in with a max-height transition.
// dropRef points at the drop target so the drag can hit-test it by rect (it sits
// outside the windowed scroll container that the geometry resolver covers).
function TrashFooter({
  dropRef,
  dragging,
  isOver,
}: {
  dropRef: React.Ref<HTMLDivElement>
  dragging: boolean
  isOver: boolean
}) {
  return (
    <div
      className={cn(
        'shrink-0 overflow-hidden transition-[max-height] duration-150 ease-out',
        dragging ? 'max-h-16' : 'max-h-0',
      )}
    >
      <div className="flex items-center justify-center border-t border-border bg-background p-2">
        <div
          ref={dropRef}
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
