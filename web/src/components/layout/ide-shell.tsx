import { useCallback } from 'react'
import { SidebarProvider } from '@/components/ui/sidebar'
import { SidebarProjectHeader } from './sidebar-project-header'
import { useNavigationHistory } from '@/features/tabs/hooks/use-navigation-history'
import { SidebarBody } from './sidebar-body'
import { SidebarFoot } from './sidebar-foot'
import { SidebarProjectSwitcher } from './sidebar-project-switcher'
import { RemovalTray } from './removal-tray'
import { SidebarTreeSurface } from './sidebar-tree-surface'
import { useSpaceSwitcherKeyboard } from '@/features/keymaps/hooks/use-space-switcher-keyboard'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { FontStyleInjector } from '@/features/settings/components/font-style-injector'
import { ConnectionIndicator } from './connection-indicator'
import { DetachHolderModal } from './detach-holder-modal'
import { PlaceholderToastWatcher } from './placeholder-toast-watcher'
import { SidebarToastOverlay } from './sidebar-toast-overlay'
import { SidebarPeek } from './sidebar-peek'
import { useSidebarPanel, SIDEBAR_MIN_PX, SIDEBAR_MAX_PX } from './use-sidebar-panel'
import { useSidebarFocusMode } from './use-sidebar-focus-mode'
import { SidebarSplitPane } from './sidebar-split-pane'
import { useSidebarNavStore } from '@/features/layout/stores/sidebar-nav'
import { IdeShellWorkspaceContent } from './ide-shell-workspace-content'
import { useShellRouteState } from './use-shell-route-state'
import { ConsoleDock } from '@/features/console/components/console-dock'
import { useMacTrafficLightSync } from '@/features/tabs/hooks/use-mac-traffic-light-sync'

/** The shell inside the console's frame, so the console can cover or shrink the whole window. */
export function IDEShell() {
  return (
    <ConsoleDock>
      <IdeShellBody />
    </ConsoleDock>
  )
}

function IdeShellBody() {
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
  useSidebarFocusMode()
  const {
    activeProjectId: activeProjectIdFromRoute,
    allProjects,
    effectiveActiveWorkspaceId,
    paneWsIds,
    viewWsIds,
    creatingSpace,
    handleCreateSpace,
    handleSelectProject,
    handleCancelCreateSpace,
    handleAddProject,
  } = useShellRouteState()
  // Browser-tab convention: Cmd/Ctrl+1-9 jump straight to a space by position
  // (9 always the last one). Stable, as it is the hook's effect dependency.
  const handleSpaceSwitcherSelect = useCallback(
    (index: number) => {
      const project = allProjects[index]
      if (project) handleSelectProject(project.id)
    },
    [allProjects, handleSelectProject],
  )
  useSpaceSwitcherKeyboard(allProjects.length, handleSpaceSwitcherSelect)
  const hasNavScreen = useSidebarNavStore((s) => s.stack.length > 0)

  // SidebarPeek is a wrapper, not a branch: it renders in every state and only
  // restyles itself, so hiding the sidebar never rebuilds the subtree below it.
  const sidebarContent = (
    <SidebarPeek hidden={!sidebarOpen} side={sidebarSide} width={preferredWidth}>
      <div className="relative flex h-full min-h-0 flex-col overflow-hidden bg-transparent select-none">
        {!hasNavScreen && <SidebarProjectHeader />}
        <SidebarBody
          projectSwitcher={
            <SidebarProjectSwitcher
              projects={allProjects}
              activeProjectId={activeProjectIdFromRoute}
              onSelectProject={handleSelectProject}
              onAddProject={handleAddProject}
            />
          }
          chatsPanel={
            !hasNavScreen && (
              <SidebarTreeSurface
                projects={allProjects}
                activeProjectId={activeProjectIdFromRoute}
                onActiveProjectChange={handleSelectProject}
                creatingSpace={creatingSpace}
                onCreateSpace={handleCreateSpace}
                onCancelCreateSpace={handleCancelCreateSpace}
              />
            )
          }
        />
        <SidebarFoot />
        {/* The removal service, mounted where NOTHING unmounts it.
            It draws no held row of its own (those transform in place in the
            tree, sidebar-row.tsx) — it owns the 8s commit clock, the seconds
            numerals, the pagehide flush and `RemovalConfirmDialog`. It cannot
            be gated on anything: a gated tray never sends its DELETE. */}
        <RemovalTray />
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
      className="h-full min-h-0 flex-1 bg-transparent text-foreground"
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
      <FontStyleInjector />
      <ConnectionIndicator />
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
 * which the jump list's retarget handshake depends on (the marker is
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
