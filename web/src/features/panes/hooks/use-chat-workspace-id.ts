import { useCallback, useMemo, useSyncExternalStore } from 'react'
import {
  getAllActiveWorkspaceIds,
  getWorkspaceStore,
  subscribeWorkspaceStores,
} from '@/features/workspace/stores/workspace-store-registry'
import { workspacesWithViewChat } from '@/features/workspace/lib/keep-alive-policy'
import { resolveChatWorkspaceId } from '@/features/panes/lib/pane-chat-workspace'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { Repo } from '@/lib/store/sidebar'

// Ids can never contain NUL (workspace-host.tsx's own guarantee, reused
// verbatim here) — safe as a join delimiter for the stable keys below.
const ID_DELIM = '\x00'

/**
 * {@link resolveChatWorkspaceId} in the render path — the workspace `chatId`
 * belongs to, re-resolved whenever that could start (or stop) being knowable.
 *
 * A pane mounts before any store has been seeded with the chat it holds, so a
 * one-shot lookup at mount answers null and stays there for the session. The
 * subscription is registry-wide for the same reason the resolver's scan is:
 * WHICH store will turn out to hold the chat is precisely what the caller does
 * not know yet.
 *
 * `hint` is {@link resolveChatWorkspaceId}'s own second argument, threaded
 * straight through — a caller that already has a claim (a `SidebarRow`'s
 * `workspaceId`, live off the sidebar tree) should pass it, since it is the
 * one source that answers before any workspace store has mounted at all.
 * Without one, a chat belonging to a repo the ROUTE has never visited this
 * session (a split merged in from elsewhere) resolves to null until
 * something else happens to mount it — caught live: a two-repo split left
 * the file explorer stuck on whichever repo the route had actually opened,
 * un-reactive to switching the active pane to the other one.
 *
 * Returns a plain string (or null), so a store write that leaves the answer
 * unchanged — every one of them, in the steady state — re-renders nothing.
 */
export function useChatWorkspaceId(chatId: string | null, hint?: string | null): string | null {
  const subscribe = useCallback(
    (onChange: () => void) => (chatId ? subscribeWorkspaceStores(onChange) : () => {}),
    [chatId],
  )
  const snapshot = useCallback(
    () => (chatId ? resolveChatWorkspaceId(chatId, hint) : null),
    [chatId, hint],
  )
  return useSyncExternalStore(subscribe, snapshot, snapshot)
}

/**
 * {@link useChatWorkspaceId}'s own `hint` argument, off the live sidebar
 * tree — the ONE source that answers synchronously, on the very first
 * render, before any workspace store has mounted (or even been asked to).
 *
 * Every caller of `useChatWorkspaceId` was meant to pass this (that hook's
 * own doc says so), but two of the three real call sites didn't:
 * `PaneContainer` and `TabBar`'s `ChatHead` wrapper both called
 * `useChatWorkspaceId(chatId)` bare. Without a hint, first resolution can
 * only come from the REGISTRY — populated by `WorkspaceHost`'s reconcile
 * effect, which runs AFTER the click that made this chat's pane active, not
 * synchronously with it. The result was a one-(or more-)frame flash of the
 * WRONG (ambient) title/content on every click into a pane whose workspace
 * wasn't already warm, self-correcting only once the registry caught up —
 * caught live as "the chat head changes the instant I click another pane."
 * A hint answers in that same render, so there is nothing left to correct.
 *
 * Extracted once here rather than re-implemented at each call site (a third,
 * near-identical copy already lived in `ide-shell.tsx` before this) — the
 * three-way duplication was itself how the two bare calls above went
 * unnoticed as duplicates of a pattern nobody had named yet.
 */
export function useChatWorkspaceHint(chatId: string | null): string | null {
  return useSidebarStore((s) => chatWorkspaceHintIn(s.repos, chatId))
}

function chatWorkspaceHintIn(repos: Repo[], chatId: string | null): string | null {
  if (!chatId) return null
  for (const repo of repos) {
    const chat = repo.chats?.find((c) => c.id === chatId)
    if (chat?.workspaceId) return chat.workspaceId
  }
  return null
}

/**
 * The workspace the ACTIVE PANE's chat belongs to — and nothing else.
 *
 * `useChatWorkspaceId(useActivePaneChatId(), hint)` computes the same answer,
 * and that is exactly how `IDEShell` used to spell it: one subscription
 * yielding the active pane's CHAT id, a second yielding that chat's sidebar
 * hint, then the resolver. Both intermediates change whenever the user clicks
 * from one chat to another — including between two chats of the SAME workspace,
 * where the answer this shell actually consumes does not move at all — so the
 * app's ROOT component re-rendered on every such click. Nothing below IDEShell
 * is memoized (see its own note about the sidebar rows), so that one render
 * walked the entire application: the sidebar, the file tree, the settings
 * dialog, and every retained workspace's own copy of the window-level pane
 * tree. Measured live in the Tauri app, 3-pane split, 4 retained workspaces:
 * ~4,600 fibers and two 140-190ms frames per click — the one-frame flash across
 * every chat on screen, and a drop from 120fps to well under 30.
 *
 * Collapsing all three steps behind ONE `useSyncExternalStore` whose snapshot is
 * the resolved workspace id keeps the intermediates out of the render path
 * entirely: a click that lands on a chat of the same workspace re-renders
 * nothing. Same convention (and same reason) as `useChatWorkspaceId`'s own
 * "returns a plain string, so a store write that leaves the answer unchanged
 * re-renders nothing".
 */
export function useActivePaneWorkspaceId(): string | null {
  const subscribe = useCallback((onChange: () => void) => {
    const unsubs = [
      windowPaneStore.subscribe(onChange),
      useSidebarStore.subscribe(onChange),
      subscribeWorkspaceStores(onChange),
    ]
    return () => {
      for (const unsub of unsubs) unsub()
    }
  }, [])
  const snapshot = useCallback(() => {
    const panes = windowPaneStore.getState()
    const chatId = panes.panes[panes.activePaneId]?.chatId ?? null
    if (!chatId) return null
    return resolveChatWorkspaceId(
      chatId,
      chatWorkspaceHintIn(useSidebarStore.getState().repos, chatId),
    )
  }, [])
  return useSyncExternalStore(subscribe, snapshot, snapshot)
}

/**
 * {@link resolveChatWorkspaceId} for EVERY pane at once, not just the active
 * one — every workspace id currently held by ANY pane in the window.
 *
 * `entries` is `[chatId, hint]` pairs, one per pane's chat (the same `hint`
 * {@link useChatWorkspaceId} takes) — resolved by the caller off the live
 * sidebar tree, since this hook has no reason to depend on `lib/store/sidebar`
 * itself. Deduplicated and order-stable (sorted), so a caller feeding this
 * straight into a dependency array or another stable key doesn't thrash.
 *
 * Built for `WorkspaceHost`'s own `paneWsIds` prop: a pane can hold a chat
 * from a workspace `WorkspaceHost` has never mounted (never routed to, never
 * clicked into) — `getWorkspaceStore` mints nothing for one, so
 * `PaneContainer`'s `chatStore` falls back to whichever workspace happens to
 * be AMBIENT (the active one). That fallback is a documented last resort for
 * a chat NOTHING can name a workspace for — it is not meant to stand in for
 * "this workspace's store just doesn't exist yet", but it silently did,
 * which is what made a still-unmounted pane render blank/wrong, and then
 * appear to "switch chat" the instant some OTHER pane's click changed which
 * workspace the ambient fallback resolves to — caught live, in a two-repo
 * split. Feeding every pane's real workspace id into `WorkspaceHost`'s
 * retention set closes the gap at its source: once mounted, a pane's own
 * `chatWsId` resolves a REAL store and the ambient fallback is never reached
 * for it at all.
 */
export function usePaneWorkspaceIds(
  entries: ReadonlyArray<readonly [string, string | null]>,
): string[] {
  const key = [...entries.map(([id]) => id)].sort().join(ID_DELIM)
  const subscribe = useCallback(
    (onChange: () => void) => (key ? subscribeWorkspaceStores(onChange) : () => {}),
    [key],
  )
  const snapshot = useCallback(() => {
    const ids = new Set<string>()
    for (const [chatId, hint] of entries) {
      const wsId = resolveChatWorkspaceId(chatId, hint)
      if (wsId) ids.add(wsId)
    }
    return [...ids].sort().join(ID_DELIM)
  }, [entries])
  const resolvedKey = useSyncExternalStore(subscribe, snapshot, snapshot)
  return useMemo(() => (resolvedKey ? resolvedKey.split(ID_DELIM) : []), [resolvedKey])
}

/**
 * Every workspace id some pane's EDITOR TABS reference — files, terminals,
 * diffs, previews — via each open buffer's own `workspaceId`
 * (`EditorTabBase.workspaceId`, pane-content.ts).
 *
 * `usePaneWorkspaceIds` above only resolves a pane's CHAT — but a pane can
 * hold editor tabs with `chatId: null` (an editor-only split, e.g. a file
 * opened beside a chat pane). Such a pane names no chat at all, so it was
 * invisible to `WorkspaceHost`'s retention set (`paneWsIds`): unless its
 * workspace also happened to be the single active one, or own a chat
 * Recents was tracking, `planRetention` (keep-alive-policy.ts) could
 * legitimately evict it — destroying the workspace's store, and with it
 * `EditorSurface`'s `editorManager` — while the pane displaying its file was
 * still on screen. Live-reported as "Editor failed to load. Try closing and
 * reopening this file.": the split's OTHER pane (a chat) switched the
 * active workspace elsewhere, its own workspace had no chat left in
 * Recents, and the next render's `getWorkspaceStore(workspaceId)!.editorManager`
 * threw on the now-destroyed store.
 */
export function usePaneEditorWorkspaceIds(): string[] {
  const subscribe = useCallback((onChange: () => void) => windowPaneStore.subscribe(onChange), [])
  const snapshot = useCallback(() => {
    const { panes, buffers } = windowPaneStore.getState()
    const bufferWorkspace = new Map(buffers.map((b) => [b.id, b.workspaceId]))
    const ids = new Set<string>()
    for (const pane of Object.values(panes)) {
      for (const tabId of pane.editorTabIds) {
        const wsId = bufferWorkspace.get(tabId)
        if (wsId) ids.add(wsId)
      }
    }
    return [...ids].sort().join(ID_DELIM)
  }, [])
  const key = useSyncExternalStore(subscribe, snapshot, snapshot)
  return useMemo(() => (key ? key.split(ID_DELIM) : []), [key])
}

/**
 * Every workspace id that currently owns at least one chat present in some
 * Recents entry (live, working, set, or dormant) — "in a view" per
 * `keep-alive-policy.ts`'s new retention rule. Built for `WorkspaceHost`'s
 * own `viewWsIds` prop: it needs this to decide what stays mounted, and this
 * hook is where the "which workspaces does Recents currently track"
 * question already gets answered generically (see `workspacesWithViewChat`),
 * the same way `usePaneWorkspaceIds` above answers the narrower "which
 * workspaces does some PANE currently name" one.
 *
 * Scans every currently-registered workspace store (`getAllActiveWorkspaceIds`)
 * for its own `agentChats.chats`/`agentChats.working` — same "only a live
 * store can say who owns a chat" scoping `recents-for-project.ts` uses,
 * generalized across every project rather than one. Subscribes to the one
 * window-level pane store (a view opening/closing/merging) AND the workspace
 * registry (a chat's working flag flipping, or a chat being deleted) —
 * whichever changes first, this recomputes; `useSyncExternalStore`'s
 * `Object.is` check on the returned string key means a change that doesn't
 * actually move any workspace in or out of Recents re-renders nothing.
 */
export function useViewWorkspaceIds(): string[] {
  const subscribe = useCallback((onChange: () => void) => {
    const unsubs = [windowPaneStore.subscribe(onChange), subscribeWorkspaceStores(onChange)]
    return () => {
      for (const unsub of unsubs) unsub()
    }
  }, [])
  const snapshot = useCallback(() => {
    const { panes, dormantArrangements } = windowPaneStore.getState()
    const working: Record<string, boolean> = {}
    const chatOwner = new Map<string, string>()
    for (const wsId of getAllActiveWorkspaceIds()) {
      const store = getWorkspaceStore(wsId)
      if (!store) continue
      const { agentChats } = store.getState()
      Object.assign(working, agentChats.working)
      for (const chat of agentChats.chats) chatOwner.set(chat.id, chat.workspaceId || wsId)
    }
    const owners = workspacesWithViewChat(
      Object.values(panes),
      working,
      dormantArrangements,
      chatOwner,
    )
    return [...owners].sort().join(ID_DELIM)
  }, [])
  const key = useSyncExternalStore(subscribe, snapshot, snapshot)
  return useMemo(() => (key ? key.split(ID_DELIM) : []), [key])
}
