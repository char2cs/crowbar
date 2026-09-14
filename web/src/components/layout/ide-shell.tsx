import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { Outlet, useNavigate, useRouterState } from '@tanstack/react-router'
import { SidebarProvider } from '@/components/ui/sidebar'
import { SidebarProjectHeader } from './sidebar-project-header'
import { useNavigationHistory } from '@/features/tabs/hooks/use-navigation-history'
import { SidebarCarousel } from './sidebar-carousel'
import { SidebarTreeSurface } from './sidebar-tree-surface'
import { SidebarFooter } from './sidebar-footer'
import { useSpaceSwitcherKeyboard } from '@/features/keymaps/hooks/use-space-switcher-keyboard'
import { useSidebarStore } from '@/lib/store/sidebar'
import {
  useProjectStore,
  useProjectDataStore,
  EMPTY_PROJECTS,
  importProjectAndSync,
} from '@/lib/store/projects'
import type { Project } from '@/lib/types'
import { WorkspaceHost } from '@/features/workspace/components/workspace-host'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { TerminalHost } from '@/features/terminal/components/terminal-host'
import { ErrorBoundary } from '@/components/error-boundary'
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { FontStyleInjector } from '@/features/settings/components/font-style-injector'
import { ConnectionIndicator } from './connection-indicator'
import { FpsOverlay } from './fps-overlay'
import { DetachHolderModal } from './detach-holder-modal'
import { PlaceholderToastWatcher } from './placeholder-toast-watcher'
import { SidebarToastOverlay } from './sidebar-toast-overlay'
import { SidebarPeek } from './sidebar-peek'
import { useSidebarPanel, SIDEBAR_MIN_PX, SIDEBAR_MAX_PX } from './use-sidebar-panel'
import { SidebarSplitPane } from './sidebar-split-pane'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { recordWorkspaceScopeFromPath, setWorkspaceScope } from '@/lib/workspace-scope'
import { useWorkspaceProviderStream } from '@/features/workspace/stores/hooks/use-workspace-provider-stream'
import { dataOf } from '@/lib/loadable'
import {
  ensureHomeWorkspaceResolved,
  getKnownHomeWorkspaceIds,
  useHomeWorkspaceState,
} from '@/features/workspace/lib/home-workspace-resolver'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { selectIsShowingEmptyStage } from '@/features/panes/stores/slices/pane-slice'
import {
  useActivePaneWorkspaceId,
  usePaneWorkspaceIds,
  useViewWorkspaceIds,
} from '@/features/panes/hooks/use-chat-workspace-id'
import { useMacTrafficLightSync } from '@/features/tabs/hooks/use-mac-traffic-light-sync'

// Ids can never contain NUL/SOH (workspace-host.tsx's own NUL guarantee,
// extended here with a second delimiter for a chatId/wsId pair within one
// entry) — safe join/split delimiters for the stable keys below.
const PANE_ENTRY_DELIM = '\x00'
const PANE_PAIR_DELIM = '\x01'

export function IDEShell() {
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const navigate = useNavigate()
  const isSettingsOpen = useUIState((s) => s.isSettingsOpen)
  const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition)
  const sidebarSide = sidebarPosition === 'right' ? 'right' : 'left'
  // macOS only, and only meaningful once the pane/sidebar chrome below is on
  // screen to measure — see the hook's own doc for the geometry it re-derives.
  useMacTrafficLightSync(sidebarSide)
  const { sidebarOpen, setSidebarOpen, preferredWidth, commitPreferredWidth } = useSidebarPanel()

  // §7: the TanStack /ide/:projectId/:repoId/:wsId route params are the
  // canonical source for the active project/repo/workspace — read them directly
  // rather than scanning the sidebar store (which lags the route on cold start).
  // Recording the scope here, SYNCHRONOUSLY during render, is load-bearing: the
  // WorkspaceView subtree (rendered below) builds workspace-scoped URLs via
  // workspaceBase() during its own render, so the scope must exist before then —
  // recording it only in the route component's post-render effect threw on first
  // paint and tripped the ErrorBoundary (§14 add-repo regression).
  const routeScope = recordWorkspaceScopeFromPath(pathname)
  const homeRouteMatch = routeScope ? null : pathname.match(/\/ide\/([^/]+)\/home$/)
  const activeProjectIdFromRoute = routeScope?.projectId ?? homeRouteMatch?.[1]
  const activeRepoIdFromRoute = routeScope?.repoId
  const activeWorkspaceId = routeScope?.wsId

  // Resolve the home workspace id ONCE per project (cached for the session —
  // see home-workspace-resolver.ts) instead of HomeRoute re-fetching and
  // cold-mounting a fresh WorkspaceView on every single visit to project
  // home. Kicking the fetch off here (not in HomeRoute) means WorkspaceHost
  // below can keep the resulting workspace mounted-but-hidden via its normal
  // keep-alive retention, so a repeat visit is a warm slot reveal.
  const homeProjectId = homeRouteMatch ? activeProjectIdFromRoute : undefined
  const { wsId: homeWorkspaceId, owningChatId: homeOwningChatId } = useHomeWorkspaceState(
    homeProjectId ?? null,
  )
  useEffect(() => {
    if (homeProjectId) ensureHomeWorkspaceResolved(homeProjectId)
  }, [homeProjectId])
  // Recorded SYNCHRONOUSLY during render (matches recordWorkspaceScopeFromPath
  // above) — WorkspaceHost, rendered below, mounts this workspace's
  // WorkspaceView in the same render pass, and workspaceBase() needs the
  // scope to exist before that.
  if (homeRouteMatch && homeProjectId && homeWorkspaceId) {
    // owningChatId travels with it: home is project-level, so it never appears in
    // the sidebar's repo tree that records every other workspace's owning chat,
    // and a chat-scoped URL (a terminal's) could not be built for it otherwise.
    setWorkspaceScope({
      projectId: homeProjectId,
      repoId: '',
      wsId: homeWorkspaceId,
      owningChatId: homeOwningChatId ?? undefined,
    })
  }
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
  // a re-render of this component is a re-render of the whole application,
  // every sidebar row and the whole pane tree included (see the
  // `--card-bottom-inset` note further down for the same lesson).
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
  // Same hint lookup as `activePaneWorkspaceHint` above, generalized to every
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
  const paneWorkspaceIds = usePaneWorkspaceIds(paneChatEntries)
  // Every workspace Recents currently tracks a chat for (live, working, set,
  // or dormant) — fed into WorkspaceHost below as `viewWsIds`, its new "in a
  // view" retention test (workspaceKeepAliveMinutes and its time-window
  // policy are gone; see keep-alive-policy.ts).
  const viewWorkspaceIds = useViewWorkspaceIds()
  // The workspace WorkspaceHost should treat as "active": the active pane's
  // own workspace first (see above), then the routed workspace, then — on
  // project home — the resolved home workspace once known.
  const effectiveActiveWorkspaceId = activePaneWorkspaceId ?? activeWorkspaceId ?? homeWorkspaceId ?? null
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
  const sidebarWorkspacePath = useSidebarStore((s) => {
    if (sidebarWorkspaceId) {
      for (const repo of s.repos) {
        if (repo.defaultWorkspaceId === sidebarWorkspaceId) return repo.localPath ?? ''
        const ws = repo.workspaces.find((w) => w.id === sidebarWorkspaceId)
        if (ws) return ws.localPath || repo.localPath || ''
      }
    }
    if (!homeRouteMatch) return ''
    return s.repos.find((r) => r.projectId === activeProjectIdFromRoute)?.localPath ?? ''
  })
  // For the home route there is no repoId, so fall back to any repo under the
  // active project, then to the project's own path (the home workspace root).
  const allProjects = useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)
  // The tree's only entry point for a SECOND space (spec §3 ruling): a
  // trailing `+` mark alongside the space marks in SidebarFooter. Swaps
  // SpaceScroller for CreateSpacePanel in place (sidebar-tree-surface.tsx)
  // rather than opening a modal — Zen's own "Create a Space" sheet lives in
  // the sidebar itself. Lifted here, alongside `allProjects`, so both the
  // footer (the marks) and the surface below can reach it.
  const [creatingSpace, setCreatingSpace] = useState(false)
  const handleCreateSpace = useCallback((project: Project) => {
    importProjectAndSync(project)
    setCreatingSpace(false)
  }, [])
  // Space marks (spec §4.1): same navigation the workspace switcher's own
  // project-home rows already use (workspace-switcher.tsx's `select`).
  // Shared verbatim with SidebarTreeSurface's SpaceScroller below
  // (`onActiveProjectChange`) — one "switch to project" action, not two.
  // Also cancels an in-progress space creation: picking an existing space
  // while the create form is open has to replace it, not leave it stranded
  // behind the newly-routed content.
  const handleSelectProject = (projectId: string) => {
    setCreatingSpace(false)
    void navigate({ to: '/ide/$projectId/home', params: { projectId } })
  }
  // Browser-tab convention: Cmd/Ctrl+1-9 jump straight to a space by
  // position (9 always the last one) — the same shortcut `develop` wired for
  // sidebar context switching, now driving spaces instead.
  useSpaceSwitcherKeyboard(allProjects.length, (index) => {
    const project = allProjects[index]
    if (project) handleSelectProject(project.id)
  })
  const projectFallbackPath = homeRouteMatch
    ? (allProjects.find((p) => p.id === activeProjectIdFromRoute)?.path ?? '')
    : ''
  const activeWorkspaceRepoPath = sidebarWorkspacePath || projectFallbackPath

  const hasNavScreen = useSidebarNavStore((s) => s.stack.length > 0)
  // The file-explorer card only makes sense alongside a real view — with
  // nothing open (the empty-stage fallback pane, spec §5.4's tumbling
  // wordmark) there is no workspace in particular the Files/Git tabs would
  // even be showing. `selectIsShowingEmptyStage` reads the exact same fact
  // the pane store itself uses to decide whether the showing tree is worth
  // parking at all — not a fresh guess at "empty" here.
  const isShowingEmptyStage = useStore(windowPaneStore, selectIsShowingEmptyStage)

  // The floating file-explorer card's own rail (spec §6: it "opens at one
  // third of the sidebar's height" and "height is kept as a proportion of
  // the rail"). Measured here, not inside SidebarCarousel, because the rail
  // IS this column — measure synchronously on mount (mirrors
  // use-tab-bar-scroll.ts's own layout-effect + ResizeObserver pattern) so
  // the card opens at the right height on first paint, not one frame late.
  // `sidebarRailRef` sits on the whole sidebar column (header/tree/floating
  // card/footer/toasts) and stays the height-measurement target even though
  // SidebarFooter now sits below the card as a true flow sibling — the
  // card's own bottom anchor resolves against a narrower `relative flex-1`
  // wrapper around just the tree + carousel instead (see sidebarContent
  // below), so the footer's height never eats into the card's own.
  const sidebarRailRef = useRef<HTMLDivElement>(null)
  const [sidebarRailHeight, setSidebarRailHeight] = useState(0)
  useLayoutEffect(() => {
    const el = sidebarRailRef.current
    if (!el) return
    const measure = () => setSidebarRailHeight(el.getBoundingClientRect().height)
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  // The tree's own bottom inset (spec §6) no longer flows through React
  // state or props here: SidebarCarousel writes its live height straight
  // onto `sidebarRailRef` as a `--card-bottom-inset` CSS custom property
  // (via the `railRef` prop below), and `space-scroller.tsx`'s `SpacePanel`
  // reads it back through plain CSS inheritance. A per-frame React state
  // update here previously re-rendered this whole shell — and, since none
  // of SidebarTreeSurface/SpaceScroller/SpacePanel/SidebarTree/SidebarRow
  // are memoized, every visible row — on every frame of a resize drag.

  // BUG-003: when landing directly on a workspace route, the header project
  // button showed "Select project" — the active project was never derived from
  // the route. Keep the active project in sync with the route's projectId.
  const workspaceProjectId = activeProjectIdFromRoute
  useEffect(() => {
    if (!workspaceProjectId) return
    if (useProjectStore.getState().activeProjectId !== workspaceProjectId) {
      useProjectStore.getState().setActiveProject(workspaceProjectId)
    }
  }, [workspaceProjectId])

  // The one invariant that makes "viewing a project that no longer exists"
  // unreachable, regardless of what took it: this client's own space delete,
  // a teammate's, anything — `allProjects` is kept live by the `/v0/projects`
  // WS stream (projects.ts's own §6 note), so a tombstone lands here the same
  // render pass it lands anywhere else. `removal-commit.ts`'s `leaveIfRemoved`
  // tries to pick a good target BEFORE its own delete request fires, but its
  // `hiddenIds` check only ever matches a 'workspace'/'folder'/'chat' removal
  // — a 'repo'/'project' hold's `hiddenIds` names repo/project ids, never a
  // workspace id, so it silently does nothing for exactly the two removals
  // that can strand the user on project home with nothing left to render
  // (caught live: deleting the space you're sitting on left "Project Home
  // unavailable" on screen, with the now-gone project's sidebar still drawn
  // behind it). `/` is `_shell/index.tsx`'s own router: it already redirects
  // to a project that DOES exist, or to `/oobe` once none are left — the
  // legitimate "no more views" default this falls back to.
  //
  // Gated on `status === 'success'`: a `staleData` snapshot mid-refetch, or
  // the store's own idle instant before its first fetch resolves, must never
  // read as "this project is gone" and bounce a perfectly live route.
  const projectsStatus = useProjectDataStore((s) => s.data.status)
  useEffect(() => {
    if (projectsStatus !== 'success') return
    if (!activeProjectIdFromRoute) return
    if (allProjects.some((p) => p.id === activeProjectIdFromRoute)) return
    void navigate({ to: '/' })
  }, [projectsStatus, activeProjectIdFromRoute, allProjects, navigate])

  // SidebarPeek is a wrapper, not a branch: it renders in every state and only
  // restyles itself, so hiding the sidebar never rebuilds the subtree below it.
  //
  // The project marks (SidebarFooter) mount here as their own row, below the
  // tree/card area and above the toast overlay — the sidebar's true last
  // content element — per the product owner's explicit placement call, not
  // squeezed into SidebarProjectHeader's window-chrome row any more. The tree
  // and the floating card are wrapped in their own `relative flex-1` box so
  // the card's `absolute`/`bottom-2` anchor (sidebar-carousel.tsx) resolves
  // against THAT box's bottom edge, right above the footer, rather than
  // against `sidebarRailRef`'s — which now extends past the footer too.
  const sidebarContent = (
    <SidebarPeek hidden={!sidebarOpen} side={sidebarSide} width={preferredWidth}>
      <div
        ref={sidebarRailRef}
        className="relative flex h-full min-h-0 flex-col overflow-hidden bg-transparent select-none"
      >
        {!hasNavScreen && <SidebarProjectHeader />}
        <div className="relative flex min-h-0 flex-1 flex-col overflow-hidden">
          {!hasNavScreen && (
            <SidebarTreeSurface
              projects={allProjects}
              activeProjectId={activeProjectIdFromRoute}
              onActiveProjectChange={handleSelectProject}
              creatingSpace={creatingSpace}
              onCreateSpace={handleCreateSpace}
              onCancelCreateSpace={() => setCreatingSpace(false)}
            />
          )}
          {/* Floats absolutely over whatever SidebarTreeSurface renders
              (its own doc: "never splits layout with it") — over the create-
              space form that means it covers the Cancel button, not just the
              tree. Hidden while creating a space rather than made to respect
              a prop of its own: a rare, deliberate excursion, and its height/
              fold state already round-trips through localStorage, so nothing
              is lost by unmounting for it. Hidden with nothing open too
              (`isShowingEmptyStage`): a Files/Git card for no view in
              particular read as chrome left over from before the last view
              closed — caught live, sitting there beside the empty-stage
              wordmark with no workspace of its own to speak for. */}
          {!creatingSpace && !isShowingEmptyStage && (
            <ErrorBoundary>
              <SidebarCarousel
                activeWorkspaceRepoPath={activeWorkspaceRepoPath}
                sidebarHeight={sidebarRailHeight}
                railRef={sidebarRailRef}
              />
            </ErrorBoundary>
          )}
        </div>
        <SidebarFooter
          projects={allProjects}
          activeProjectId={activeProjectIdFromRoute}
          onSelectProject={handleSelectProject}
          onAddProject={() => setCreatingSpace(true)}
        />
        <SidebarToastOverlay sidebarOpen={sidebarOpen} sidebarSide={sidebarSide} />
      </div>
    </SidebarPeek>
  )

  // Every pane in here is its OWN drop target now (PaneContainer spreads
  // `PANE_DROP_ATTR` per pane, keyed by pane id) — spec §8.1's four-target
  // table, not the one whole-content "drop anywhere in here to remove it"
  // zone this div used to be (Task 22 deleted that dwell-to-remove gesture
  // along with `editor-removal-overlay.tsx`).
  const contentEl = (
    <div className="relative z-[1] flex h-full min-w-0 flex-col bg-transparent">
      <ErrorBoundary>
        {/* WorkspaceHost stays mounted for the whole IDE session — including on
            the project-home route. Unmounting the host on every home visit
            destroyed all keep-alive retention (stores, terminals, Monaco
            models) — so returning to a workspace was a full COLD re-mount
            every time. Keeping the host mounted lets it retain
            recently-visited workspaces (all hidden) across home transits, so
            the return is warm.

            On the home route, `effectiveActiveWorkspaceId` is the resolved
            home workspace (once known) — the host renders ITS WorkspaceView
            too, as just another retained slot, instead of HomeRoute
            cold-mounting a fresh one on every visit (that used to be ~2x the
            frame cost of a normal warm switch; see
            home-workspace-resolver.ts). `homeWsIds` protects every home
            workspace resolved so far this session from the existence-prune —
            home is a project-level concept, not in the sidebar's repo/
            workspace id set, so without this it would look "closed" the
            instant it goes hidden and get destroyed instead of retained.
            `paneWsIds` (see its own doc, use-chat-workspace-id.ts) does the
            same for every workspace a PANE currently holds a chat for — not
            just the one that's "active" — so a split's other pane(s) always
            get a real store instead of falling back to the wrong ambient
            one. `viewWsIds` (`useViewWorkspaceIds`) is the host's actual
            retention test now: every workspace with a chat somewhere in
            Recents stays mounted, and dropping out of `viewWsIds` is what
            gets a workspace evicted — no more time-based keep-alive window.
            HomeRoute itself renders null (or the error state); the
            Outlet still stays mounted so workspace-route components'
            route-level guards keep running. */}
        <WorkspaceHost
          activeWsId={effectiveActiveWorkspaceId}
          homeWsIds={getKnownHomeWorkspaceIds()}
          paneWsIds={paneWorkspaceIds}
          viewWsIds={viewWorkspaceIds}
        />
        <Outlet />
      </ErrorBoundary>
    </div>
  )

  return (
    <SidebarProvider
      className="h-screen bg-transparent text-foreground"
      open={sidebarOpen}
      onOpenChange={setSidebarOpen}
    >
      {/* Grid areas move the two already-mounted regions when the side changes;
          neither the sidebar tree nor WorkspaceHost is reconciled into a new
          position. The separator owns pointer tracking only while it is being
          dragged, so ordinary pointer movement over either region has no split
          layout work to do. */}
      <SidebarSplitPane
        side={sidebarSide}
        open={sidebarOpen}
        preferredWidth={preferredWidth}
        minWidth={SIDEBAR_MIN_PX}
        maxWidth={SIDEBAR_MAX_PX}
        sidebar={sidebarContent}
        onOpenChange={setSidebarOpen}
        onWidthCommit={commitPreferredWidth}
      >
        {contentEl}
      </SidebarSplitPane>
      <SettingsDialog
        isOpen={isSettingsOpen}
        onClose={() => useUIState.getState().setIsSettingsDialogVisible(false)}
      />
      <TerminalHost />
      <FontStyleInjector />
      <ConnectionIndicator />
      <FpsOverlay />
      <DetachHolderModal />
      <PlaceholderToastWatcher />
      <NavigationHistoryRecorder />
    </SidebarProvider>
  )
}

/**
 * `useNavigationHistory` as a LEAF, not as a call in `IDEShell`'s own body.
 *
 * The hook returns void — it renders nothing, it only records into the jump
 * list — but it subscribes to `panes[activePaneId].activeEditorTabId`, which
 * moves every time focus crosses between a pane holding an editor tab and one
 * that doesn't. Called directly in `IDEShell`, that put a WINDOW-WIDE value in
 * the app root's render path, and nothing below `IDEShell` is memoized: each
 * such click walked ~810 fibers — the whole sidebar, every row's Tooltip and
 * Dropdown, the file-explorer card, the settings dialog — for a render whose
 * output was identical. Measured live in the Tauri app, three panes tiled in
 * ONE workspace view (so no workspace switch is involved, unlike the
 * `useActivePaneWorkspaceId` fix this sits beside), 15 focus clicks: 9,159
 * rendered fibers and 78fps, against 120fps idle.
 *
 * Isolating it here leaves the subscription — and its React-effect timing,
 * which `navigateToJumpEntry`'s retarget handshake depends on (the marker is
 * moved onto the reopened buffer's brand-new id AFTER the pane-store write
 * that reveals it, so a recorder running synchronously with that write would
 * record the jump as a fresh navigation and truncate the forward branch) —
 * exactly as it was, while confining the re-render to this one childless
 * fiber. Still mounted at shell level rather than in `SidebarProjectHeader`,
 * whose back/forward arrows it feeds, so history keeps accruing while that
 * header is hidden behind a nav screen and survives it unmounting.
 */
function NavigationHistoryRecorder(): null {
  useNavigationHistory()
  return null
}
