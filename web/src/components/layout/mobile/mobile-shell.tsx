import { useEffect, useRef } from 'react'
import { useStore } from 'zustand'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { TerminalWindowIcon } from '@phosphor-icons/react'
import { SidebarProvider } from '@/components/ui/sidebar'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { FontStyleInjector } from '@/features/settings/components/font-style-injector'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { useMobileNativeChrome } from '@/features/window/hooks/use-mobile-native-chrome'
import { useNativeSidebar } from '@/features/window/hooks/use-native-sidebar'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { hasNativeChrome } from '@/lib/native-chrome'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { NativeAgentsSettingsFeed } from '@/features/settings/components/native-agents-settings-feed'
import { NativeHostsSettingsFeed } from '@/features/settings/components/native-hosts-settings-feed'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { IdeShellWorkspaceContent } from '../ide-shell-workspace-content'
import { DetachHolderModal } from '../detach-holder-modal'
import { PlaceholderToastWatcher } from '../placeholder-toast-watcher'
import { RemovalTray } from '../removal-tray'
import { RowBranchImportDialog } from '../sidebar-tree-chrome'
import { SidebarToastOverlay } from '../sidebar-toast-overlay'
import { useShellRouteState } from '../use-shell-route-state'
import { MobileIdeScreen } from './mobile-ide-screen'
import { MobileTopBar } from './mobile-top-bar'
import { useLaunchDraft } from './use-launch-draft'

const IDE = {
  label: 'Open IDE',
  icon: <TerminalWindowIcon size={20} />,
  onClick: () => useMobileScreenStore.getState().open('ide'),
}

/**
 * The phone's shell: the focused chat, and its IDE (terminals, branch review) pushed full
 * screen over it with the chat staying mounted beneath. The chat list is native, fed from here.
 */
export function MobileShell() {
  const route = useShellRouteState()
  const navigate = useNavigate()
  const screen = useMobileScreenStore((s) => s.screen)
  const isSettingsOpen = useUIState((s) => s.isSettingsOpen)
  // The native sheet's Agents page shows the active project's host: the phone has no local daemon.
  const agentsHostId = route.allProjects.find((p) => p.id === route.activeProjectId)?.hostId
  const wsId = route.effectiveActiveWorkspaceId
  const hasChat = useStore(windowPaneStore, (s) => Boolean(s.panes[s.activePaneId]?.chatId))
  useLaunchDraft(route.activeProjectId, route.homeWorkspaceId, navigate)
  useCloseChatListOnNavigation()
  useMobileNativeChrome({ ide: hasChat })
  const { importRepoRowId, closeImport } = useNativeSidebar({
    activeProjectId: route.activeProjectId,
    projects: route.allProjects,
    onSelectProject: route.handleSelectProject,
  })

  return (
    // The desktop surfaces reused here read a sidebar context; the phone's list is native.
    <SidebarProvider className="h-dvh min-h-0 flex-1 text-foreground" open={false}>
      <div className="flex h-full w-full flex-col bg-background">
        <MobileTopBar trailing={{ ...IDE, disabled: !hasChat }} />
        <div className="flex min-h-0 flex-1 flex-col pb-[env(safe-area-inset-bottom)]">
          <IdeShellWorkspaceContent
            effectiveActiveWorkspaceId={wsId}
            paneWsIds={route.paneWsIds}
            viewWsIds={route.viewWsIds}
            showViewTabs={false}
          />
        </div>
      </div>
      <MobileIdeScreen hidden={screen !== 'ide'} />
      {isSettingsOpen && hasNativeChrome() && agentsHostId && (
        <AgentsSettingsFeed key={agentsHostId} hostId={agentsHostId} />
      )}
      {isSettingsOpen && hasNativeChrome() && <NativeHostsSettingsFeed />}
      <SettingsDialog
        // The phone app's own sheet presents Settings off the same `isSettingsOpen`.
        isOpen={isSettingsOpen && !hasNativeChrome()}
        onClose={() => useUIState.getState().setIsSettingsDialogVisible(false)}
      />
      <FontStyleInjector />
      <DetachHolderModal />
      <ImportDialog repoRowId={importRepoRowId} onClose={closeImport} />
      <PlaceholderToastWatcher />
      {/* The removal service and the app's one toast viewport, as in the desktop shell. */}
      <RemovalTray />
      <SidebarToastOverlay sidebarOpen={false} sidebarSide="left" />
    </SidebarProvider>
  )
}

function AgentsSettingsFeed({ hostId }: { hostId: string }) {
  const hostName = useHostConfigStore((s) => s.hosts.find((h) => h.id === hostId)?.name ?? '')
  return <NativeAgentsSettingsFeed hostId={hostId} hostName={hostName} />
}

function ImportDialog({ repoRowId, onClose }: { repoRowId: string | null; onClose: () => void }) {
  const repos = useSidebarStore((s) => s.repos)
  return <RowBranchImportDialog repoRowId={repoRowId} repos={repos} onClose={onClose} />
}

/** Opening a chat from the list (a new route, or a new chat in the one pane) puts it away. */
function useCloseChatListOnNavigation() {
  const pathname = useRouterState({ select: (state) => state.location.pathname })
  const chatId = useStore(windowPaneStore, (s) => s.panes[s.activePaneId]?.chatId ?? null)
  const shown = `${pathname}\n${chatId}`
  const lastShown = useRef(shown)
  const lastChat = useRef(chatId)
  useEffect(() => {
    if (lastShown.current === shown) return
    const left = lastChat.current
    lastShown.current = shown
    lastChat.current = chatId
    // Leaving a chat that is being removed is not opening another: the list stays up.
    if (left && useRemovalTrayStore.getState().hiddenIds.has(left)) return
    useMobileScreenStore.getState().setChatListOpen(false)
  }, [shown, chatId])
}
