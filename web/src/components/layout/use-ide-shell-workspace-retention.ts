import { useMemo } from 'react'
import { useStore } from 'zustand'
import { useSidebarStore } from '@/lib/store/sidebar'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import {
  useActivePaneWorkspaceId,
  usePaneEditorWorkspaceIds,
  usePaneWorkspaceIds,
  useViewWorkspaceIds,
} from '@/features/panes/hooks/use-chat-workspace-id'
import { useWorkspaceProviderStream } from '@/features/workspace/stores/hooks/use-workspace-provider-stream'

// Ids can never contain NUL/SOH (workspace-host.tsx's own NUL guarantee,
// extended here with a second delimiter for a chatId/wsId pair within one
// entry) — safe join/split delimiters for the stable keys below.
const PANE_ENTRY_DELIM = '\x00'
const PANE_PAIR_DELIM = '\x01'

export interface IdeShellWorkspaceRetention {
  /** The workspace WorkspaceHost should treat as "active" — see field doc
   *  below at the computation site. */
  effectiveActiveWorkspaceId: string | null
  /** Every workspace some pane currently holds a chat OR editor tab for —
   *  already unioned/deduped, `WorkspaceHost`'s own `paneWsIds` retention
   *  input. */
  paneWsIds: string[]
  /** Every workspace Recents currently tracks a chat for — `WorkspaceHost`'s
   *  `viewWsIds` retention input. */
  viewWsIds: string[]
  /** The active workspace's resolved filesystem path, for the sidebar's
   *  file-explorer card. */
  sidebarWorkspacePath: string
}

/**
 * Every workspace-id fact `WorkspaceHost`'s retention (`activeWsId`,
 * `paneWsIds`, `viewWsIds`) and the sidebar's file-explorer path need,
 * resolved off the active pane/route — pulled out of `IDEShell` itself,
 * which otherwise threaded ~10 intermediate values through its own body just
 * to reach the one component (`WorkspaceHost`) and the one selector
 * (`sidebarWorkspacePath`) that actually read them. Mirrors how
 * use-chat-workspace-id.ts already isolates each SINGLE-source resolution
 * this hook composes.
 */
export function useIdeShellWorkspaceRetention(
  activeWorkspaceId: string | undefined,
  homeWorkspaceId: string | null,
  activeProjectIdFromRoute: string | undefined,
  activeRepoIdFromRoute: string | undefined,
  isHomeRoute: boolean,
): IdeShellWorkspaceRetention {
  // The chat the ACTIVE PANE is showing, and the workspace that chat belongs
  // to — resolved before `effectiveActiveWorkspaceId` below, which now leans
  // on it. A split can merge chats from different workspaces into one view
  // (spec §8.2's drag-to-merge), and everything downstream that means "the
  // workspace you're sitting in" — the file-explorer card, but also
  // `WorkspaceHost`'s own single "active" slot, which is what actually
  // mounts `WorkspaceActiveEffects` (use-workspace-effects.ts: git store,
  // file-system store, save/pane keyboard) for exactly one workspace at a
  // time — needs to agree with whichever pane you actually clicked into, not
  // just the URL. Pinned to the route alone, changing the active PANE inside
  // a split never fired any of that: the file tree kept showing the pane you
  // had left, because the global file-system store is written only by the
  // workspace `WorkspaceHost` currently calls "active" — caught live,
  // clicking between two panes on different repos left the file explorer
  // stuck on the first one.
  //
  // Resolved by ONE hook rather than the three steps this used to spell out
  // (active pane → its chat id → that chat's sidebar hint → the resolver):
  // each intermediate moves on every click from one chat to another, while the
  // ANSWER only moves when the two chats belong to different workspaces — and
  // a re-render of IDEShell is a re-render of the whole application, every
  // sidebar row and the whole pane tree included.
  // `useActivePaneWorkspaceId` subscribes to all three sources and yields the
  // resolved id alone, so clicking between two chats of the same workspace now
  // re-renders nothing here.
  const activePaneWorkspaceId = useActivePaneWorkspaceId()
  // Every chat ANY pane currently holds, not just the active one — stable,
  // deduped key so this only changes identity when a pane actually starts or
  // stops naming a NEW chat, not on every unrelated pane-store write.
  const paneChatIdsKey = useStore(windowPaneStore, (s) => {
    const ids = new Set<string>()
    for (const pane of Object.values(s.panes)) if (pane.chatId) ids.add(pane.chatId)
    return [...ids].sort().join(PANE_ENTRY_DELIM)
  })
  const paneChatIds = useMemo(
    () => (paneChatIdsKey ? paneChatIdsKey.split(PANE_ENTRY_DELIM) : []),
    [paneChatIdsKey],
  )
  // Same hint lookup as `activePaneWorkspaceId` above, generalized to every
  // pane's chat rather than just the active one's.
  const paneChatHintsKey = useSidebarStore((s) => {
    const parts: string[] = []
    for (const chatId of paneChatIds) {
      for (const repo of s.repos) {
        const chat = repo.chats?.find((c) => c.id === chatId)
        if (chat?.workspaceId) {
          parts.push(`${chatId}${PANE_PAIR_DELIM}${chat.workspaceId}`)
          break
        }
      }
    }
    return parts.join(PANE_ENTRY_DELIM)
  })
  const paneChatEntries = useMemo<Array<[string, string | null]>>(() => {
    const hints = new Map<string, string>()
    if (paneChatHintsKey) {
      for (const part of paneChatHintsKey.split(PANE_ENTRY_DELIM)) {
        const [chatId, wsId] = part.split(PANE_PAIR_DELIM)
        hints.set(chatId, wsId)
      }
    }
    return paneChatIds.map((chatId) => [chatId, hints.get(chatId) ?? null])
  }, [paneChatIds, paneChatHintsKey])
  // Every workspace SOME pane holds a chat for — fed into WorkspaceHost below
  // so each one gets a real, mounted store instead of silently falling back
  // to whichever workspace happens to be ambient (see usePaneWorkspaceIds'
  // own doc for the "clicking one pane switches the other's chat" bug this
  // closes).
  // Every workspace some pane's EDITOR TABS reference, via each open buffer's
  // own workspaceId — an editor-only pane (chatId: null) names no chat, so
  // it is invisible to paneWorkspaceIds above; without this, WorkspaceHost's
  // retention could evict a workspace still displaying an open file/terminal
  // split the instant its chat (if any) dropped out of Recents (see the
  // hook's own doc — "Editor failed to load" was this).
  const paneEditorWorkspaceIds = usePaneEditorWorkspaceIds()
  const paneWorkspaceIds = usePaneWorkspaceIds(paneChatEntries)
  // Every workspace Recents currently tracks a chat for (live, working, set,
  // or dormant) — fed into WorkspaceHost below as `viewWsIds`, its new "in a
  // view" retention test (workspaceKeepAliveMinutes and its time-window
  // policy are gone; see keep-alive-policy.ts).
  const viewWorkspaceIds = useViewWorkspaceIds()
  // The workspace WorkspaceHost should treat as "active": the active pane's
  // own workspace first (see above), then the routed workspace, then — on
  // project home — the resolved home workspace once known.
  const effectiveActiveWorkspaceId =
    activePaneWorkspaceId ?? activeWorkspaceId ?? homeWorkspaceId ?? null
  // Open the per-:wsId workspace WS stream for the viewed workspace. Beyond data,
  // this is what starts the daemon's per-connection provider poll so a branch with
  // an open PR flips to the green pr-open icon (the list stream never starts it).
  useWorkspaceProviderStream(activeProjectIdFromRoute, activeRepoIdFromRoute, activeWorkspaceId)
  // The shell only needs one scalar from the sidebar tree. Subscribing to the
  // whole repos array made every live status/count frame rebuild the complete
  // IDE shell — sidebar provider, carousel, offscreen panels and workspace host
  // included. Returning the resolved path lets Zustand bail out unless the
  // active workspace's actual filesystem scope changed.
  const sidebarWorkspaceId = activePaneWorkspaceId ?? activeWorkspaceId
  // For the home route there is no repoId, so fall back to any repo under the
  // active project, then to the project's own path (the home workspace root).
  const sidebarWorkspacePath = useSidebarStore((s) => {
    if (sidebarWorkspaceId) {
      for (const repo of s.repos) {
        if (repo.defaultWorkspaceId === sidebarWorkspaceId) return repo.localPath ?? ''
        const ws = repo.workspaces.find((w) => w.id === sidebarWorkspaceId)
        if (ws) return ws.localPath || repo.localPath || ''
      }
    }
    if (!isHomeRoute) return ''
    return s.repos.find((r) => r.projectId === activeProjectIdFromRoute)?.localPath ?? ''
  })

  return {
    effectiveActiveWorkspaceId,
    paneWsIds: [...new Set([...paneWorkspaceIds, ...paneEditorWorkspaceIds])],
    viewWsIds: viewWorkspaceIds,
    sidebarWorkspacePath,
  }
}
