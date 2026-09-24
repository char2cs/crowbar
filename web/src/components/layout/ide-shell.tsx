import { useCallback, useEffect, useRef, useState } from 'react'
import { useStore } from 'zustand'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { SidebarProvider } from '@/components/ui/sidebar'
import { SidebarProjectHeader } from './sidebar-project-header'
import { useNavigationHistory } from '@/features/tabs/hooks/use-navigation-history'
import { SidebarCarousel } from './sidebar-carousel'
import { SidebarTreeSurface } from './sidebar-tree-surface'
import { SidebarFooter } from './sidebar-footer'
import { useSpaceSwitcherKeyboard } from '@/features/keymaps/hooks/use-space-switcher-keyboard'
import {
  useProjectStore,
  useProjectDataStore,
  EMPTY_PROJECTS,
  importProjectAndSync,
} from '@/lib/store/projects'
import type { Project } from '@/lib/types'
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
import { dataOf } from '@/lib/loadable'
import {
  ensureHomeWorkspaceResolved,
  useHomeWorkspaceState,
} from '@/features/workspace/lib/home-workspace-resolver'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { selectIsShowingEmptyStage } from '@/features/panes/lib/view-selectors'
import { useIdeShellWorkspaceRetention } from './use-ide-shell-workspace-retention'
import { IdeShellWorkspaceContent } from './ide-shell-workspace-content'
import { useMeasuredHeight } from './use-measured-height'
import { useRedirectIfProjectMissing } from './use-redirect-if-project-missing'
import { useMacTrafficLightSync } from '@/features/tabs/hooks/use-mac-traffic-light-sync'

export function IDEShell() {
  const routerState = useRouterState()
  const pathname = routerState.location.pathname
  const navigate = useNavigate()
  const isSettingsOpen = useUIState((s) => s.isSettingsOpen)
  const sidebarPosition = useSettingsStore((state) => state.settings.sidebarPosition)
  const sidebarSide = sidebarPosition === 'right' ? 'right' : 'left'
  const theme = useSettingsStore((state) => state.settings.theme)
  const themeMode = useSettingsStore((state) => state.settings.themeMode)
  // macOS only, and only meaningful once the pane/sidebar chrome below is on
  // screen to measure — see the hook's own doc for the geometry it re-derives.
  // themeKey re-runs the sync after every theme switch: applying a theme pins
  // the native vibrancy view's appearance, which resets the traffic lights as
  // a side effect (see the hook's own doc).
  useMacTrafficLightSync(sidebarSide, `${theme}:${themeMode}`)
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
  // Not gated to `homeRouteMatch`: retention's sidebar-path lookup needs this project's home id/path off the home route too (a split can mix a home chat's pane with a repo route), or it falls through to the repo scan and dead-ends on "No folder open".
  const homeProjectId = activeProjectIdFromRoute
  const {
    wsId: homeWorkspaceId,
    owningChatId: homeOwningChatId,
    localPath: homeWorkspacePath,
  } = useHomeWorkspaceState(homeProjectId ?? null)
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
  // Every workspace-id fact WorkspaceHost's retention and the sidebar's
  // file-explorer path need, resolved off the active pane/route — see the
  // hook's own doc for why this lives outside IDEShell's body.
  const allProjects = useProjectDataStore((s) => dataOf(s.data) ?? EMPTY_PROJECTS)
  const projectPath = allProjects.find((p) => p.id === activeProjectIdFromRoute)?.path ?? ''
  const { effectiveActiveWorkspaceId, paneWsIds, viewWsIds } = useIdeShellWorkspaceRetention(
    activeWorkspaceId,
    homeWorkspaceId,
    activeProjectIdFromRoute,
    activeRepoIdFromRoute,
    Boolean(homeRouteMatch),
    homeWorkspacePath,
    projectPath,
  )
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
  //
  // useCallback'd (stable across renders unless `navigate` itself changes —
  // TanStack Router's own hook already returns a stable reference) so the
  // memoized SidebarTreeSurface/SidebarFooter below don't see a fresh prop
  // identity, and therefore don't re-render, on an IDEShell render this
  // handler had no part in.
  const handleSelectProject = useCallback(
    (projectId: string) => {
      setCreatingSpace(false)
      void navigate({ to: '/ide/$projectId/home', params: { projectId } })
    },
    [navigate],
  )
  // Same reason as `handleSelectProject` above — stable identities so
  // memoized SidebarTreeSurface/SidebarFooter can actually skip a render
  // that doesn't touch space-creation state, instead of always seeing a
  // fresh inline-arrow prop.
  const handleCancelCreateSpace = useCallback(() => setCreatingSpace(false), [])
  const handleAddProject = useCallback(() => setCreatingSpace(true), [])
  // Browser-tab convention: Cmd/Ctrl+1-9 jump straight to a space by
  // position (9 always the last one) — the same shortcut `develop` wired for
  // sidebar context switching, now driving spaces instead.
  //
  // useCallback'd (same reasoning as `handleSelectProject` above) — this is
  // the hook's own `onSelect`, an effect dependency INSIDE
  // useSpaceSwitcherKeyboard: an inline arrow here was a fresh identity every
  // render, so that effect tore down and re-added its window keydown
  // listener on every IDEShell render instead of just when `allProjects` or
  // `handleSelectProject` actually change.
  const handleSpaceSwitcherSelect = useCallback(
    (index: number) => {
      const project = allProjects[index]
      if (project) handleSelectProject(project.id)
    },
    [allProjects, handleSelectProject],
  )
  useSpaceSwitcherKeyboard(allProjects.length, handleSpaceSwitcherSelect)
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
  // IS this column. `sidebarRailRef` sits on the whole sidebar column
  // (header/tree/floating card/footer/toasts) and stays the
  // height-measurement target even though SidebarFooter now sits below the
  // card as a true flow sibling — the card's own bottom anchor resolves
  // against a narrower `relative flex-1` wrapper around just the tree +
  // carousel instead (see sidebarContent below), so the footer's height
  // never eats into the card's own.
  const sidebarRailRef = useRef<HTMLDivElement>(null)
  const sidebarRailHeight = useMeasuredHeight(sidebarRailRef)
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
  //
  // THE ONE WRITER of the pane store's own `activeProjectId` too (the
  // project-scoped panes design's trap 4): the route is already the one place
  // a project change becomes state, and a second writer — a click handler
  // racing this effect — is exactly how the sidebar's own project-switch bugs
  // happened. `setActiveProject` no-ops on an unchanged id, so calling it
  // unconditionally here is free; parking the space you left and bringing this
  // one's own last view forward is all downstream of that single write.
  const workspaceProjectId = activeProjectIdFromRoute
  useEffect(() => {
    if (!workspaceProjectId) return
    if (useProjectStore.getState().activeProjectId !== workspaceProjectId) {
      useProjectStore.getState().setActiveProject(workspaceProjectId)
    }
    windowPaneStore.getState().paneActions.setActiveProject(workspaceProjectId)
  }, [workspaceProjectId])

  // See the hook's own doc for the "viewing a project that no longer exists"
  // invariant this enforces.
  useRedirectIfProjectMissing(activeProjectIdFromRoute, allProjects, navigate)

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
              onCancelCreateSpace={handleCancelCreateSpace}
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
              <SidebarCarousel sidebarHeight={sidebarRailHeight} railRef={sidebarRailRef} />
            </ErrorBoundary>
          )}
        </div>
        <SidebarFooter
          projects={allProjects}
          activeProjectId={activeProjectIdFromRoute}
          onSelectProject={handleSelectProject}
          onAddProject={handleAddProject}
        />
        <SidebarToastOverlay sidebarOpen={sidebarOpen} sidebarSide={sidebarSide} />
      </div>
    </SidebarPeek>
  )

  // See ide-shell-workspace-content.tsx's own doc for why WorkspaceHost/
  // Outlet live in their own component rather than inline here — this
  // subtree only ever needs the three resolved workspace-id sets below, none
  // of the sidebar/route state the rest of this shell carries.
  const contentEl = (
    <IdeShellWorkspaceContent
      effectiveActiveWorkspaceId={effectiveActiveWorkspaceId}
      paneWsIds={paneWsIds}
      viewWsIds={viewWsIds}
    />
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
