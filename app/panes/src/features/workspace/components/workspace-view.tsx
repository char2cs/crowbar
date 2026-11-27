import { memo, useEffect } from 'react'
import { WorkspaceStoreContext } from '../stores/workspace-context'
import type { WorkspaceStore } from '../stores/workspace-store'
import { markStart, markEnd } from '@/lib/perf/instrumentation'
import { useWorkspaceEffects } from '../stores/hooks/use-workspace-effects'
import { useChatProviders } from '@/features/agent/hooks/use-chat-providers'
import { useSaveKeyboard } from '@/features/keymaps/hooks/use-save-keyboard'
import { usePaneKeyboard } from '@/features/panes/hooks/use-pane-keyboard'
import { useSidebarTabKeyboard } from '@/features/keymaps/hooks/use-sidebar-tab-keyboard'
import { useZoomKeyboard } from '@/features/keymaps/hooks/use-zoom-keyboard'

interface WorkspaceViewProps {
  wsId: string
  store: WorkspaceStore
  active: boolean
}

export const WorkspaceView = memo(function WorkspaceView({
  wsId,
  store,
  active,
}: WorkspaceViewProps) {
  useChatProviders(wsId)

  useEffect(() => {
    markStart('workspace.switch')
    const raf = requestAnimationFrame(() => markEnd('workspace.switch'))
    return () => cancelAnimationFrame(raf)
  }, [wsId])

  return (
    <WorkspaceStoreContext.Provider value={store}>
      <WorkspaceDataFeeds wsId={wsId} active={active} />
      {active && <WorkspaceKeyboard />}
    </WorkspaceStoreContext.Provider>
  )
})

function WorkspaceDataFeeds({ wsId, active }: Pick<WorkspaceViewProps, 'wsId' | 'active'>) {
  useWorkspaceEffects(wsId, active)
  return null
}

function WorkspaceKeyboard() {
  useSaveKeyboard()
  usePaneKeyboard()
  useSidebarTabKeyboard()
  useZoomKeyboard()
  return null
}
