import { WorkspaceStoreContext } from '../stores/workspace-context'
import { getOrCreateWorkspaceStore } from '../stores/workspace-store-registry'
import { WorkspaceLayoutRoot } from './WorkspaceLayoutRoot'
import { useWorkspaceEffects } from '../stores/hooks/use-workspace-effects'

interface WorkspaceViewProps {
  wsId: string
  label?: string
}

export function WorkspaceView({ wsId, label }: WorkspaceViewProps) {
  const store = getOrCreateWorkspaceStore(wsId)

  return (
    <WorkspaceStoreContext.Provider value={store}>
      <WorkspaceViewInner wsId={wsId} label={label} />
    </WorkspaceStoreContext.Provider>
  )
}

function WorkspaceViewInner({ wsId, label }: WorkspaceViewProps) {
  useWorkspaceEffects(wsId, label)
  return <WorkspaceLayoutRoot />
}
