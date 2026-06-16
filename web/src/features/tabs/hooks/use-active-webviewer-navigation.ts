import { useActiveWorkspaceBuffer } from '@/features/workspace/hooks/use-active-workspace-buffer'
import {
  useWebViewerNavigationStore,
  type WebViewerNavEntry,
} from '@/features/web-viewer/stores/web-viewer-navigation-store'

/**
 * Derives jump-navigation inputs for the globally-active buffer: whether it is
 * a web viewer, and its live navigation entry (if so). Feeds `useJumpNavigation`
 * from the global sidebar header.
 */
export function useActiveWebViewerNavigation(): {
  usesWebViewerNavigation: boolean
  activeWebViewerNavigation: WebViewerNavEntry | undefined
} {
  const activeBuffer = useActiveWorkspaceBuffer()
  const usesWebViewerNavigation = activeBuffer?.type === 'webViewer'
  const activeWebViewerNavigation = useWebViewerNavigationStore((s) =>
    usesWebViewerNavigation && activeBuffer ? s.navigationByBufferId[activeBuffer.id] : undefined,
  )
  return { usesWebViewerNavigation, activeWebViewerNavigation }
}
