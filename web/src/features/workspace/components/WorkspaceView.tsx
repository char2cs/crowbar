import { useLayoutEffect } from 'react'
import { WorkspaceStoreContext } from '../stores/workspace-context'
import { getOrCreateWorkspaceStore } from '../stores/workspace-store-registry'
import { setActiveWorkspaceStoreRef } from '../stores/workspace-store-ref'
import { WorkspaceLayoutRoot } from './WorkspaceLayoutRoot'
import { useWorkspaceEffects } from '../stores/hooks/use-workspace-effects'

interface WorkspaceViewProps {
  wsId: string
  label?: string
}

export function WorkspaceView({ wsId, label }: WorkspaceViewProps) {
  const store = getOrCreateWorkspaceStore(wsId)

  useLayoutEffect(() => {
    setActiveWorkspaceStoreRef(store)
    return () => { setActiveWorkspaceStoreRef(null) }
  }, [store])

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
