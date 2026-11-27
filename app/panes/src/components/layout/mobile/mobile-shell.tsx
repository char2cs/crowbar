import { useEffect, useRef } from 'react'
import { useStore } from 'zustand'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { CodeIcon } from '@phosphor-icons/react'
import { SidebarProvider } from '@/components/ui/sidebar'
import SettingsDialog from '@/features/settings/components/settings-dialog'
import { FontStyleInjector } from '@/features/settings/components/font-style-injector'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { useMobileNativeChrome } from '@/features/window/hooks/use-mobile-native-chrome'
import { useNativeSidebar } from '@/features/window/hooks/use-native-sidebar'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { hasNativeChrome } from '@/lib/native-chrome'
import { selectRepos, useSelect } from '@/core/entities/select'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'
import { NativeAgentsSettingsFeed } from '@/features/settings/components/native-agents-settings-feed'
import { NativeHostsSettingsFeed } from '@/features/settings/components/native-hosts-settings-feed'
import { NativeHostPickerFeed } from '@/features/hosts/components/native-host-picker-feed'
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
  icon: <CodeIcon size={20} />,
  onClick: () => useMobileScreenStore.getState().open('ide'),
}

export function MobileShell() {
  const route = useShellRouteState()
  const navigate = useNavigate()
  const screen = useMobileScreenStore((s) => s.screen)
  const isSettingsOpen = useUIState((s) => s.isSettingsOpen)
  const isHostPickerOpen = useUIState((s) => s.isHostPickerOpen)
  const agentsHostId = route.allProjects.find((p) => p.id === route.activeProjectId)?.hostId
  const wsId = route.effectiveActiveWorkspaceId
  const hasChat = useStore(windowPaneStore, (s) => Boolean(s.panes[s.activePaneId]?.chatId))
  useLaunchDraft(route.activeProjectId, route.homeWorkspaceId, navigate)
  useCloseChatListOnNavigation()
  const { importRepoRowId, closeImport, shownHost } = useNativeSidebar({
    activeProjectId: route.activeProjectId,
    projects: route.allProjects,
    onSelectProject: route.handleSelectProject,
  })
  useMobileNativeChrome({ ide: hasChat, shownHost })

  return (
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
      {isSettingsOpen && hasNativeChrome() && <NativeHostsSettingsFeed shownHost={shownHost} />}
      {isHostPickerOpen && hasNativeChrome() && <NativeHostPickerFeed shownHost={shownHost} />}
      <SettingsDialog
        isOpen={isSettingsOpen && !hasNativeChrome()}
        onClose={() => useUIState.getState().setIsSettingsDialogVisible(false)}
      />
      <FontStyleInjector />
      <DetachHolderModal />
      <ImportDialog repoRowId={importRepoRowId} onClose={closeImport} />
      <PlaceholderToastWatcher />
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
  const repos = useSelect(selectRepos)
  return <RowBranchImportDialog repoRowId={repoRowId} repos={repos} onClose={onClose} />
}

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
    if (left && treeUiStore.getState().hiddenIds.has(left)) return
    useMobileScreenStore.getState().setChatListOpen(false)
  }, [shown, chatId])
}
