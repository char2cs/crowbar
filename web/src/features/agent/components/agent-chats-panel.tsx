import { useCallback, useMemo, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { cn } from '@/lib/utils'
import { DragGhost, DragGhostRows } from '@/components/layout/drag-ghost'
import { DropIndicator } from '@/components/layout/drop-indicator'
import {
  AGENT_CHAT_ROW_HEIGHT,
  useAgentChatListVirtualizer,
} from './use-agent-chat-list-virtualizer'
import { ADD_GLYPH_PATH, ROW_BASE, ROW_INACTIVE } from '@/components/layout/workspace-row-base'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { useActiveWorkspaceState } from '@/features/workspace/stores/hooks/use-active-workspace-state'
import { useWorkspaceAgentChatsStream } from '@/features/workspace/stores/hooks/use-workspace-agent-chats-stream'
import { createChat, renameChat } from '@/features/agent/api/agent-api'
import { toastSpawnFailure } from '@/features/agent/lib/spawn-error'
import { openAgentChat } from '@/features/agent/lib/open-agent-chat'
import { AgentChatRow } from './agent-chat-row'
import { AgentChatFolderRow } from './agent-chat-folder-row'
import { AgentChatsSearch } from './agent-chats-search'
import { AgentChatContextMenu } from './agent-chat-context-menu'
import { useAgentChatFolders } from './use-agent-chat-folders'
import { useAgentChatsDrag } from './use-agent-chats-drag'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { shownChatIds } from '@/features/agent/lib/shown-chats'
import { buildChatTree, type ChatRow } from '@/features/agent/lib/chat-rows'
import {
  commitChatDrop,
  groupIntoChatFolder,
  renameChatFolderRow,
} from '@/features/agent/lib/chat-tree-commit'
import {
  applyPendingChatRemovals,
  describeChatRemoval,
  planChatRemoval,
} from '@/features/agent/lib/chat-removal'
import { RemovalTray } from '@/components/layout/removal-tray'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { ChatDragSubject, ResolvedChatDrop } from '@/features/agent/lib/chat-drop'
import type { AgentProvider } from '@/features/agent/api/agent-api'

interface AgentChatsPanelProps {
  /**
   * The workspace to list chats for. Optional: when the sidebar host doesn't
   * already hold a wsId it can render <AgentChatsPanel /> bare and the panel
   * tracks the ACTIVE workspace itself.
   */
  wsId?: string
}

/** Stable empties, so a panel that has selected/folded nothing hands out one identity. */
const NO_IDS: ReadonlySet<string> = new Set<string>()

/**
 * The sidebar "Chats" panel: this workspace's chats as a real tree, then the
 * rows that make new ones.
 *
 * A chat may hold other chats — a child is a THREAD, which reads its parent's
 * turns — and folders group rows anywhere a chat may live, including inside a
 * chat. A folder holds no turns, so it can never be mistaken for a thread and
 * changes nothing about what one reads.
 *
 * Works for every workspace kind (project home, repo home, worktree). The wsId
 * comes from the active WORKSPACE STORE, not the URL: the project-home route is
 * /ide/:projectId/home, whose wsId is resolved asynchronously and never appears
 * in the path, so a pathname-derived wsId is null there and the panel rendered
 * nothing. WorkspaceView publishes the active store — and it is what BOTH route
 * shapes render — so useActiveWorkspaceState is reactive on every workspace kind.
 */
export function AgentChatsPanel({ wsId }: AgentChatsPanelProps = {}) {
  const activeWsId = useActiveWorkspaceState((s) => s.workspaceId, null)
  const resolved = wsId ?? activeWsId

  // Keyed: a workspace switch must not carry this panel's drag, rename or
  // selection over. What the user FOLDED is deliberately not among them — it
  // outlives the remount in the sidebar store, and is persisted (see
  // `collapsedChatRows` in lib/store/sidebar.ts).
  return resolved ? <AgentChatsPanelInner key={resolved} wsId={resolved} /> : null
}

// react-doctor-disable-next-line no-giant-component -- accepted: one tree with one drag and one write path; the parts that CAN be lifted out already are (chat-rows.ts builds the model, chat-drop.ts decides drops, chat-tree-commit.ts writes them, use-agent-chats-drag.ts owns the pointer).
function AgentChatsPanelInner({ wsId }: { wsId: string }) {
  // Seeds chats + providers and keeps them live (turn spinners, titles, deletes).
  useWorkspaceAgentChatsStream(wsId)
  // Folders are a second aggregate on their own route — one GET, no poll.
  useAgentChatFolders(wsId)
  const store = getOrCreateWorkspaceStore(wsId)

  const chats = useStore(store, (s) => s.agentChats.chats)
  const folders = useStore(store, (s) => s.agentChats.folders)
  const providers = useStore(store, (s) => s.agentChats.providers)
  const buffers = useStore(store, (s) => s.buffers)
  // NOTE: the panel deliberately does NOT subscribe to `agentChats.working`.
  // Immer replaces that whole map's reference on every single-chat turn frame,
  // so a panel-level subscription would re-render the entire list — and rebuild
  // the tree — on the feed's hottest event. Each AgentChatRow subscribes to its
  // own working state instead.

  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  // Folded rows survive this panel: the workspace switch above remounts it, so
  // component state re-opened every folder on the way back. Narrow selector —
  // nothing else in that store re-renders the tree.
  const collapsed = useSidebarStore((s) => s.collapsedChatRows)
  const [foldedAway, setFoldedAway] = useState<ReadonlySet<string>>(NO_IDS)
  const [selected, setSelected] = useState<ReadonlySet<string>>(NO_IDS)

  const providerIcons = useMemo(() => new Map(providers.map((p) => [p.id, p.icon])), [providers])

  // The unified New-chat pick: the FIRST ENABLED provider in priority order —
  // the same rule every New-chat surface follows. Derived from the already
  // subscribed `providers` rather than through selectEnabledProviders, which
  // returns a fresh array per call and would read as an ever-changing snapshot.
  const primaryProvider = useMemo(() => providers.find((p) => p.enabled), [providers])

  // A row is lit when its chat is ON SCREEN — the active tab of a pane the
  // layout renders. Not when it merely HAS a tab: a chat in a background tab
  // stayed lit with nothing on screen to justify it, and the highlight stopped
  // meaning "this is what you are looking at". rootLayout, not `panes`: the
  // store holds panes nothing draws (see shown-chats.ts).
  const panes = useStore(store, (s) => s.panes)
  const rootLayout = useStore(store, (s) => s.rootLayout)
  const shown = useMemo(
    () => shownChatIds(buffers, panes, rootLayout),
    [buffers, panes, rootLayout],
  )

  // A blank title is normalised ONCE, before the tree is built, so the search
  // matches what the row actually says. Identity is preserved for every titled
  // chat, so this costs nothing downstream.
  const treeChats = useMemo(
    () => chats.map((c) => (c.title ? c : { ...c, title: UNTITLED_CHAT_LABEL })),
    [chats],
  )

  // The rows the removal tray is holding come OUT of the tree while it holds
  // them: they are hidden, never deleted, which is what makes the undo a matter
  // of dropping an id rather than putting a subtree back together. At rest the
  // set is a stable empty and this hands the very arrays it was given straight
  // back, so the memo below is untouched on every workspace with nothing held.
  const hiddenIds = useRemovalTrayStore((s) => s.hiddenIds)
  const visible = useMemo(
    () => applyPendingChatRemovals(treeChats, folders, hiddenIds),
    [treeChats, folders, hiddenIds],
  )

  // ONE build feeds the render and the drag's sibling index. Memoised on exactly
  // what the tree is made of — so a turn frame, a hover, a rename in flight and
  // a drag in progress all leave it alone.
  const tree = useMemo(
    () =>
      buildChatTree({
        chats: visible.chats,
        folders: visible.folders,
        collapsed,
        shown,
        foldedAway,
        query,
      }),
    [visible, collapsed, shown, foldedAway, query],
  )
  const rows = tree.rows

  // Window the rows: only the visible slice is mounted, so 1000 chats cost what
  // 20 do. scrollRef goes on the overflow-auto container the virtualizer owns.
  const { scrollRef, rowVirtualizer } = useAgentChatListVirtualizer(rows.length)

  // What the drag reads at pointer time. A ref, not a dependency: the pointer
  // handlers subscribe once and must never close over a stale tree, and putting
  // the tree in their dependency list would re-subscribe them on every frame of
  // a scroll.
  const modelRef = useRef({ rows, siblings: tree.siblings, selected, providerIcons })
  modelRef.current = { rows, siblings: tree.siblings, selected, providerIcons }

  // Shared with the New Tab surface's recent-chat rows, so the two can never
  // disagree about what clicking a chat does (open-agent-chat.ts).
  const openChat = useCallback(
    (chatId: string) => openAgentChat(store, wsId, chatId),
    [store, wsId],
  )

  /**
   * A click on a row: ⌘/Ctrl collects it, a plain click opens it and drops the
   * collection.
   *
   * The clearing, not the styling, is what stops several lit rows accumulating.
   */
  const selectChat = useCallback(
    (chatId: string, meta: boolean) => {
      if (meta) {
        setSelected((prev) => {
          const next = new Set(prev)
          if (!next.delete(chatId)) next.add(chatId)
          return next
        })
        return
      }
      setSelected(NO_IDS)
      openChat(chatId)
    },
    [openChat],
  )

  /**
   * Collect a folder into the multiselection, or drop it out again.
   *
   * Only ever the ⌘/Ctrl gesture: a plain click on a folder toggles it, so the
   * row calls this from that branch alone and there is nothing here to guard.
   */
  const selectRow = useCallback((id: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (!next.delete(id)) next.add(id)
      return next
    })
  }, [])

  const toggleRow = useCallback((id: string) => {
    useSidebarStore.getState().toggleChatRow(id)
    // Opening a row hands back whatever it was holding, so a fold-away it is
    // carrying from last time must not survive into the next fold.
    setFoldedAway((prev) => {
      if (!prev.has(id)) return prev
      const next = new Set(prev)
      next.delete(id)
      return next
    })
  }, [])

  /** Let go of the rows a folded parent is keeping on screen. */
  const foldAwayRow = useCallback((id: string) => {
    setFoldedAway((prev) => new Set(prev).add(id))
  }, [])

  /**
   * Start a chat, optionally inside `parentId`.
   *
   * ONE callback for all three surfaces, because they are one operation with one
   * argument: the "New chat" row passes '', a folder's "+" passes the folder,
   * and a chat's "+" (or "New thread" in the menu) passes the CHAT — which is
   * what makes the new chat a thread of it, since a chat's child chat reads its
   * parent's turns. Two functions here would be two chances to file a new chat
   * somewhere its parent did not ask for.
   *
   * The provider is read from the STORE at click time rather than taken as an
   * argument: a row's "+" is drawn whether or not anything can start a chat, and
   * threading a render value through it would give every row a fresh callback
   * each time the provider list arrives.
   */
  const newChat = useCallback(
    (parentId: string) => {
      const provider = store.getState().agentChats.providers.find((p) => p.enabled)
      // Nothing can start a chat — the New-chat row is not even drawn in this
      // state, but a folder's "+" is.
      if (!provider) return
      // Putting something into a row you cannot see is not what "+ in here"
      // means, so open it first.
      if (parentId) useSidebarStore.getState().openChatRow(parentId)
      // ONE call. The parentage rides on the create, so the daemon mints the
      // chat, writes the edge and only then starts the runner — a thread has its
      // lineage before its first CLI exists and gets its ancestors injected on
      // that first session. Create-then-place ran the very turn the user asked
      // the thread for with no lineage at all, and left a window where a failed
      // second call stranded the new chat at the root.
      createChat(wsId, provider.id, parentId)
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

  /**
   * Put the selected rows in a new folder, and open its rename editor.
   *
   * The only way a folder is made here, exactly as in the workspace tree: AROUND
   * a selection, never as a standing "new folder" affordance. The name is a
   * detail typed into a row that is already on screen with its contents in it.
   */
  const groupRows = useCallback(
    (subjects: readonly ChatDragSubject[]) => {
      setSelected(NO_IDS)
      void groupIntoChatFolder(store, wsId, subjects).then((id) => {
        if (id) setRenamingId(id)
      })
    },
    [store, wsId],
  )

  /**
   * What a removal is planned against, read at POINTER time.
   *
   * `.getState()` and a ref rather than subscribed values: this is called from
   * inside a drag, once per pane crossing, and taking the chat list as a
   * dependency would give the drag hook a fresh options object on every frame of
   * the feed.
   */
  const removalInput = useCallback(() => {
    const st = store.getState()
    return {
      wsId,
      // Blank titles normalised HERE too, exactly as the tree normalises them:
      // the veil and the tray row name what is going, and a chat the daemon has
      // not titled yet was leaving both of them saying "Release to remove " with
      // nothing after it.
      chats: st.agentChats.chats.map((c) => (c.title ? c : { ...c, title: UNTITLED_CHAT_LABEL })),
      folders: st.agentChats.folders,
      // The very map the rows draw their glyphs from, so a row held for removal
      // wears the same provider mark it wore in the tree.
      providerIcons: modelRef.current.providerIcons,
      siblings: modelRef.current.siblings,
    }
  }, [store, wsId])

  /**
   * What the editor pane's veil says while these rows are over it.
   *
   * Derived from the SAME plan the release commits, so the veil can never offer
   * a removal the drop then refuses — and so it can tell the truth about the two
   * rules that differ here: a chat takes its threads, a folder does not take its
   * chats.
   */
  const removalText = useCallback(
    (subjects: readonly ChatDragSubject[], armed: boolean) => {
      const drafts = planChatRemoval(subjects, removalInput())
      return drafts.length === 0 ? null : describeChatRemoval(drafts, armed)
    },
    [removalInput],
  )

  /**
   * Hold rows for removal.
   *
   * Nothing here calls a delete. The plan decides which rows go and what goes
   * WITH them — a chat cascades to its threads, a folder promotes its contents —
   * and the tray hides them for eight seconds with an undo beside them. This is
   * the sidebar's removal path, verbatim, and it is reached from exactly the two
   * places the sidebar's is: the right-click menu, and a release over the editor
   * pane.
   */
  const removeRows = useCallback(
    (subjects: readonly ChatDragSubject[]) => {
      const drafts = planChatRemoval(subjects, removalInput())
      setSelected(NO_IDS)
      if (drafts.length === 0) return
      useRemovalTrayStore.getState().hold(drafts)
    },
    [removalInput],
  )

  // Takes the id (not the row) so each row can hold ONE stable callback for its
  // whole life — `(row) => …` per render gives every row a fresh closure and
  // defeats the memo that keeps a turn frame off the rest of the list.
  const confirmRename = useCallback(
    (id: string, title: string) => {
      setRenamingId(null)
      const st = store.getState()
      const chat = st.agentChats.chats.find((c) => c.id === id)
      if (!chat) {
        void renameChatFolderRow(store, wsId, id, title)
        return
      }
      if (title === chat.title) return
      st.upsertAgentChat({ ...chat, title }) // optimistic; the WS title_set frame confirms
      renameChat(wsId, id, title).catch((err: unknown) =>
        console.error('Failed to rename agent chat:', err),
      )
    },
    [store, wsId],
  )

  const startRename = useCallback((id: string) => setRenamingId(id), [])
  const cancelRename = useCallback(() => setRenamingId(null), [])

  // The multiselection, in TREE order — read off the row model rather than off
  // the DOM, because the list is windowed and a selected row past the fold has
  // no element to be read from.
  //
  // KEPT rows included. They are the same rows, drawn somewhere else, and each
  // carries its own real container — so a selection that spans a folded row and
  // what it is holding moves both to where they were asked to go. A row is in
  // this list at most once: the walk never descends into a folded row, so a
  // hoisted row is drawn as a kept row or in its own place, never both.
  const selectionSubjects = useCallback((): ChatDragSubject[] => {
    const model = modelRef.current
    // One pass: this is read at POINTER time — on every drag start, and again on
    // every pane crossing during one — over the whole row model.
    const subjects: ChatDragSubject[] = []
    for (const row of model.rows) {
      if (model.selected.has(row.id)) {
        subjects.push({ kind: row.kind, id: row.id, parentId: row.parentId })
      }
    }
    return subjects
  }, [])

  const onDrop = useCallback(
    (subjects: readonly ChatDragSubject[], target: ResolvedChatDrop) => {
      setSelected(NO_IDS)
      void commitChatDrop(store, wsId, subjects, target, modelRef.current.siblings)
    },
    [store, wsId],
  )

  const drag = useAgentChatsDrag({
    scrollRef,
    subjectsFor: selectionSubjects,
    onDrop,
    onPaneRemove: removeRows,
    removalText,
    onSpringOpen: toggleRow,
  })

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden">
      <AgentChatsSearch value={query} onChange={setQuery} />

      {/* The virtualizer owns this scroll element directly (like the file tree
          beside it). No top padding/border: the rows are positioned from the
          content origin. */}
      <div
        ref={scrollRef}
        id="chat-tree-results"
        data-agent-chat-scroll="true"
        className="min-h-0 flex-1 overflow-auto"
      >
        {/* Windowed region: a spacer of the full list height, with only the
            visible rows absolutely positioned inside it. */}
        <div className="relative" style={{ height: rowVirtualizer.getTotalSize() }}>
          {rowVirtualizer.getVirtualItems().map((virtualItem) => {
            const row = rows[virtualItem.index]
            // The virtualizer is driven by rows.length, but it is a third-party
            // windowing library: if it ever reports an index past the list, drop
            // the row rather than crash the sidebar.
            if (!row) return null
            return (
              <div
                key={row.id}
                className="absolute inset-x-0 top-0 flex flex-col"
                style={{
                  height: AGENT_CHAT_ROW_HEIGHT,
                  transform: `translateY(${virtualItem.start}px)`,
                }}
              >
                <TreeRow
                  row={row}
                  wsId={wsId}
                  providerIcon={providerIcons.get(row.providerId) ?? ''}
                  active={shown.has(row.id)}
                  selected={selected.has(row.id)}
                  nesting={drag.nestTargetId === row.id}
                  dragging={drag.draggingIds.has(row.id)}
                  renaming={renamingId === row.id}
                  query={query}
                  onSelectChat={selectChat}
                  onSelectRow={selectRow}
                  onToggle={toggleRow}
                  onFoldAway={foldAwayRow}
                  onNewChat={newChat}
                  onStartRename={startRename}
                  onConfirmRename={confirmRename}
                  onCancelRename={cancelRename}
                  onPointerDownDrag={drag.onPointerDownDrag}
                />
              </div>
            )
          })}
        </div>

        {/* One make-something row, below every real row and behind a hairline.
            It opens the first ENABLED provider; when none is enabled there is no
            provider to start, so the row gives way to the notice that says so —
            an empty panel that explains nothing was the defect here.

            There is no "New folder" row beside it, and that is deliberate: a
            folder is made AROUND a selection from the right-click menu, which is
            the only way the workspace tree makes one. Two panels inventing
            separate gestures for the same thing is how a sidebar stops reading
            as one place.

            flex-col so the row STRETCHES to the width minus its own mx-1.5 (a
            <button> is shrink-to-fit at display:flex in WebKit). */}
        <div className="flex flex-col">
          {primaryProvider && rows.length > 0 && (
            <div data-new-chat-separator="true" className="mx-3 my-1 border-t border-border/60" />
          )}
          {primaryProvider ? (
            <NewChatRow provider={primaryProvider} onClick={() => newChat('')} />
          ) : (
            <NoProvidersNotice />
          )}
        </div>
      </div>

      {/* Rows on their way out, held where you can still change your mind — the
          sidebar's tray, scoped to this tree's rows. There is no bin in this
          footer any more: a chat is removed by dragging it onto the EDITOR PANE,
          exactly as a workspace is, and this is where it waits afterwards.

          Drawn here as well as at the foot of the workspace tree because the two
          panels are pages of one carousel: a single tray would put the row you
          just dragged away on a page you would have to swipe to find. */}
      <RemovalTray scope="chats" />

      {/* A SIBLING of the tree, listening natively — never a prop on the rows.
          With the menu's open state inside the tree, opening it re-renders every
          row on screen to draw a popup that is not part of the tree at all. */}
      <AgentChatContextMenu
        treeRef={scrollRef}
        selectionSubjects={selectionSubjects}
        onNewThread={newChat}
        onGroup={groupRows}
        onRemove={removeRows}
      />

      {/* The hairline that says BETWEEN, and the rows in the air. Both are
          portalled to the body and positioned in viewport coordinates. */}
      {drag.dragging && <DropIndicator ref={drag.attachDropLine} />}
      {drag.dragging && drag.ghostRows && (
        <DragGhost ref={drag.ghostRef} origin={drag.ghostOrigin}>
          <DragGhostRows rows={drag.ghostRows} />
        </DragGhost>
      )}
    </div>
  )
}

interface TreeRowProps {
  row: ChatRow
  wsId: string
  providerIcon: string
  active: boolean
  selected: boolean
  nesting: boolean
  dragging: boolean
  renaming: boolean
  query: string
  onSelectChat: (chatId: string, meta: boolean) => void
  onSelectRow: (id: string) => void
  onToggle: (id: string) => void
  onFoldAway: (id: string) => void
  /** Start a chat under this row: "in here" on a folder, a THREAD on a chat. */
  onNewChat: (parentId: string) => void
  onStartRename: (id: string) => void
  onConfirmRename: (id: string, value: string) => void
  onCancelRename: () => void
  onPointerDownDrag: (subject: ChatDragSubject, e: React.PointerEvent) => void
}

/**
 * One row of the tree, chat or folder.
 *
 * A plain switch, and nothing else: every prop it hands down is a primitive off
 * the row model or a callback that outlives the render. That is what lets the
 * memo on each row component hold — this wrapper re-renders whenever the panel
 * does, and the rows underneath it do not.
 */
function TreeRow({
  row,
  wsId,
  providerIcon,
  active,
  selected,
  nesting,
  dragging,
  renaming,
  query,
  onSelectChat,
  onSelectRow,
  onToggle,
  onFoldAway,
  onNewChat,
  onStartRename,
  onConfirmRename,
  onCancelRename,
  onPointerDownDrag,
}: TreeRowProps) {
  if (row.kind === 'chatFolder') {
    return (
      <AgentChatFolderRow
        id={row.id}
        name={row.title}
        depth={row.depth}
        parentId={row.parentId}
        path={row.path}
        expanded={row.expanded}
        hasChildren={row.hasChildren}
        holding={row.holding}
        renaming={renaming}
        dragging={dragging}
        nesting={nesting}
        selected={selected}
        ctx={row.ctx}
        query={query}
        onSelect={onSelectRow}
        onToggle={onToggle}
        onFoldAway={onFoldAway}
        onStartRename={onStartRename}
        onConfirmRename={onConfirmRename}
        onCancelRename={onCancelRename}
        onAddChat={onNewChat}
        onPointerDownDrag={onPointerDownDrag}
      />
    )
  }

  return (
    <AgentChatRow
      wsId={wsId}
      chatId={row.id}
      title={row.title}
      providerIcon={providerIcon}
      active={active}
      selected={selected}
      nesting={nesting}
      dragging={dragging}
      renaming={renaming}
      depth={row.depth}
      parentId={row.parentId}
      path={row.path}
      expanded={row.expanded}
      hasChildren={row.hasChildren}
      holding={row.holding}
      ctx={row.ctx}
      query={query}
      onSelect={onSelectChat}
      onToggle={onToggle}
      onFoldAway={onFoldAway}
      onStartRename={onStartRename}
      onConfirmRename={onConfirmRename}
      onCancelRename={onCancelRename}
      onNewThread={onNewChat}
      onPointerDownDrag={onPointerDownDrag}
    />
  )
}

// What the panel says instead of the New rows when NOTHING can start a chat.
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
// (the provider it opens is the first enabled one, matching the New Tab surface).
// A real <button> — no nested interactive children, unlike the tree's rows.
function NewChatRow({ provider, onClick }: { provider: AgentProvider; onClick: () => void }) {
  return (
    <button
      type="button"
      data-new-chat={provider.id}
      // No `w-full`: ROW_BASE is `display:flex` (block-level), so the button
      // already fills the row MINUS its `mx-1.5`. `w-full` forces width:100% of
      // the parent AND keeps the 6px side margins, overflowing the sidebar by
      // 6px → a stray horizontal scrollbar in the Chats panel.
      className={cn(ROW_BASE, ROW_INACTIVE, 'text-muted-foreground hover:text-foreground')}
      onClick={onClick}
    >
      <span
        aria-hidden="true"
        className="flex size-4 shrink-0 items-center justify-center [&>svg]:size-full"
        dangerouslySetInnerHTML={{ __html: provider.icon }}
      />
      {/* text-left counters the <button>'s UA text-align:center (same fix as
          workspace-tree's "Import project" and "New" rows). */}
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
