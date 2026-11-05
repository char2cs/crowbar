import { lazy, Suspense, useCallback } from 'react'
import { SidebarProvider } from '@/components/ui/sidebar'
import { SidebarProjectHeader } from './sidebar-project-header'
import { useNavigationHistory } from '@/features/tabs/hooks/use-navigation-history'
import { SidebarBody } from './sidebar-body'
import { SidebarFoot } from './sidebar-foot'
import { SidebarProjectSwitcher } from './sidebar-project-switcher'
import { RemovalTray } from './removal-tray'
import { SidebarTreeSurface } from './sidebar-tree-surface'
import { useSpaceSwitcherKeyboard } from '@/features/keymaps/hooks/use-space-switcher-keyboard'
import { useSettingsStore } from '@/features/settings/store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { FontStyleInjector } from '@/features/settings/components/font-style-injector'
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

// Opened on demand: the dialog and every settings tab stay out of the boot bundle.
const SettingsDialog = lazy(() => import('@/features/settings/components/settings-dialog'))

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
  // macOS only. The theme key re-runs the sync after a theme switch, which pins the vibrancy
  // view's appearance and resets the traffic lights as a side effect.
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
        {/* The removal service (commit clock, pagehide flush, confirm dialog), mounted where nothing
            unmounts it: a gated tray never sends its DELETE. */}
        <RemovalTray />
        <SidebarToastOverlay sidebarOpen={sidebarOpen} sidebarSide={sidebarSide} />
      </div>
    </SidebarPeek>
  )

  // Its own component: it needs only the resolved workspace-id sets, none of the shell's state.
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
      {/* Grid areas move the two mounted regions on a side change, so neither is reconciled anew;
          the separator tracks the pointer only while dragged. */}
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
      {isSettingsOpen && (
        <Suspense fallback={null}>
          <SettingsDialog
            isOpen
            onClose={() => useUIState.getState().setIsSettingsDialogVisible(false)}
          />
        </Suspense>
      )}
      <FontStyleInjector />
      <DetachHolderModal />
      <PlaceholderToastWatcher />
      <NavigationHistoryRecorder />
    </SidebarProvider>
  )
}

// A childless leaf for `useNavigationHistory`: its active-editor-tab subscription would otherwise
// re-render the unmemoized shell on every pane-focus change. Effect timing is unchanged (the jump
// list's retarget depends on it); shell-level so history accrues while its header is hidden.
function NavigationHistoryRecorder(): null {
  useNavigationHistory()
  return null
}
