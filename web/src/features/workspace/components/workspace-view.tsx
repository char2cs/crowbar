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
  /** Mounted by `WorkspaceHost` before this renders; never minted here. */
  store: WorkspaceStore
  /** Hidden retained workspaces stay mounted; only the active one owns the keyboard handlers
   *  and the file/git watchers. */
  active: boolean
}

// A workspace's lifecycle (store, streams, active-only watchers); it renders no surface, since
// the pane tree is window-level in `WorkspaceHost`. Memoized: `IDEShell` re-renders often and
// only the two slots whose `active` flips on a switch need to run.
export const WorkspaceView = memo(function WorkspaceView({
  wsId,
  store,
  active,
}: WorkspaceViewProps) {
  // Mounted, not just active: panes holding a hidden workspace's chat read it.
  useChatProviders(wsId)

  // A workspace mounts when it first becomes active: this cold switch's span
  // closes after its first paint (M4). A warm switch is only an `active` flip,
  // and WorkspaceHost, which alone knows the target was retained, owns its span.
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

// Each workspace's tree and git slice stay loaded and subscribed while it is
// retained, so a switch changes which store the UI reads and nothing else.
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
