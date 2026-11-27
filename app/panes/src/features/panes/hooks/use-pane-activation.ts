import { useEffect, type RefObject } from 'react'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'

export function usePaneActivation(paneId: string, containerRef: RefObject<HTMLElement | null>) {
  useEffect(() => {
    const node = containerRef.current
    if (!node) return
    const onMouseDownCapture = (e: MouseEvent) => {
      const target = e.target as HTMLElement
      const isTypingSurface =
        target.classList?.contains('inputarea') ||
        target.classList?.contains('xterm-helper-textarea')
      const isDirectInteractiveHit = target.matches?.(
        "button, input, textarea, [role='button'], [role='menu']",
      )
      if (!isTypingSurface && isDirectInteractiveHit) return
      if (windowPaneStore.getState().activePaneId !== paneId) {
        windowPaneStore.getState().paneActions.setActivePane(paneId)
      }
    }
    node.addEventListener('mousedown', onMouseDownCapture, true)
    return () => node.removeEventListener('mousedown', onMouseDownCapture, true)
  }, [paneId, containerRef])
}
