import { useNativeOverlayActive } from '@/features/browser/hooks/use-native-overlay-active'
import { useConsoleStore } from '@/features/console/stores/console-store'
import { useFullscreenPaneId } from '@/features/workspace/stores/hooks/use-pane-store'
import { useSidebarPeekStore } from '@/features/window/stores/sidebar-peek-store'

/**
 * Whether something with a higher stacking order than this pane's native
 * webview is on screen: a body-level popup, the peeked sidebar card, an
 * overlay-mode console, or another pane fullscreen over this one.
 */
export function useBrowserOccluded(paneId: string): boolean {
  const overlay = useNativeOverlayActive()
  const peeking = useSidebarPeekStore((s) => s.occupied)
  const consoleOverlay = useConsoleStore((s) => s.open && s.mode === 'overlay')
  const fullscreenPaneId = useFullscreenPaneId()
  return (
    overlay ||
    peeking ||
    consoleOverlay ||
    (fullscreenPaneId !== null && fullscreenPaneId !== paneId)
  )
}
