import { useStore } from 'zustand'
import { PaneEditorViewContent } from '@/features/panes/components/pane-editor-view-content'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import type { PaneGroup } from '@/features/panes/types/pane'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import { useRegisteredWorkspaceStore } from '@/features/workspace/stores/hooks/use-workspace-store-by-id'
import { useEffect } from 'react'
import { CaretLeftIcon, FolderSimpleIcon } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { hasNativeChrome, onNativeChromeIntent } from '@/lib/native-chrome'
import { MobileFilesScreen } from './mobile-files-screen'
import { useNativeFiles } from '@/features/window/hooks/use-native-files'
import { useNativeTabs } from '@/features/window/hooks/use-native-tabs'

// In the phone app the IDE's whole header row (Back, tabs, +, Files) is native.
const NATIVE_CHROME = hasNativeChrome()
const back = () => useMobileScreenStore.getState().back()
const openFiles = () => useMobileScreenStore.getState().setFilesOpen(true)
const closeFiles = () => useMobileScreenStore.getState().setFilesOpen(false)

/**
 * The phone's IDE: the focused chat's own editor tabs (terminals, branch review) full screen.
 * Kept mounted while hidden, so a terminal keeps its screen between visits.
 */
export function MobileIdeScreen({ hidden }: { hidden: boolean }) {
  const pane = useStore(windowPaneStore, (s) => s.panes[s.activePaneId] ?? null)
  if (!pane?.chatId || !pane.workspaceId) return null
  return (
    <PaneIde
      key={pane.id}
      pane={pane}
      wsId={pane.workspaceId}
      chatId={pane.chatId}
      hidden={hidden}
    />
  )
}

function PaneIde({
  pane,
  wsId,
  chatId,
  hidden,
}: {
  pane: PaneGroup
  wsId: string
  chatId: string
  hidden: boolean
}) {
  const store = useRegisteredWorkspaceStore(wsId)
  const filesOpen = useMobileScreenStore((s) => s.filesOpen)
  useEffect(
    () =>
      onNativeChromeIntent((intent) => {
        if (intent.type === 'openFiles') openFiles()
      }),
    [],
  )
  return (
    <section
      aria-label="IDE"
      hidden={hidden}
      inert={hidden}
      className="fixed inset-0 z-40 flex flex-col bg-background pb-[env(safe-area-inset-bottom)]"
    >
      <WorkspaceStoreContext.Provider value={store}>
        {NATIVE_CHROME ? (
          <NativeIdeFeed paneId={pane.id} wsId={wsId} chatId={chatId} />
        ) : (
          <header className="flex shrink-0 items-center justify-between border-b px-2 pt-[env(safe-area-inset-top)]">
            <Button variant="ghost" size="icon" aria-label="Back" onClick={back}>
              <CaretLeftIcon size={20} />
            </Button>
            <Button variant="ghost" size="icon" aria-label="Files" onClick={openFiles}>
              <FolderSimpleIcon size={20} />
            </Button>
          </header>
        )}
        {/* Under the native header row: its 44pt below the safe area. */}
        <div
          className="relative min-h-0 flex-1 data-[native=true]:mt-[calc(env(safe-area-inset-top)+44px)]"
          data-native={NATIVE_CHROME}
        >
          {!NATIVE_CHROME && filesOpen && <MobileFilesScreen onFileOpened={closeFiles} />}
          <PaneEditorViewContent
            paneId={pane.id}
            wsId={wsId}
            editorTabIds={pane.editorTabIds}
            activeEditorTabId={pane.activeEditorTabId}
            editorViewHidden={false}
            isActivePane={!hidden}
            showing={!hidden}
          />
        </div>
      </WorkspaceStoreContext.Provider>
    </section>
  )
}

/** The native tab strip's and file drawer's feeds; the drawer is the `#/mobile-files` webview. */
function NativeIdeFeed({ paneId, wsId, chatId }: { paneId: string; wsId: string; chatId: string }) {
  useNativeTabs(paneId, wsId)
  useNativeFiles(wsId, chatId)
  return null
}
