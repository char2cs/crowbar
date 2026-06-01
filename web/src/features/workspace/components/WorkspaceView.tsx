import { useEffect, useLayoutEffect, useState } from 'react'
import { WorkspaceStoreContext } from '../stores/workspace-context'
import { getOrCreateWorkspaceStore, setActiveWorkspaceId } from '../stores/workspace-store-registry'
import { setActiveWorkspaceStoreRef } from '../stores/workspace-store-ref'
import { hydrateWorkspace } from '@/lib/persistence/hydrate'
import { WorkspaceLayoutRoot } from './WorkspaceLayoutRoot'
import { useWorkspaceEffects } from '../stores/hooks/use-workspace-effects'

interface WorkspaceViewProps {
  wsId: string
  label?: string
}

export function WorkspaceView({ wsId, label }: WorkspaceViewProps) {
  const store = getOrCreateWorkspaceStore(wsId)
  const [hydratedWsId, setHydratedWsId] = useState<string | null>(null)

  useLayoutEffect(() => {
    setActiveWorkspaceStoreRef(store)
    return () => { setActiveWorkspaceStoreRef(null) }
  }, [store])

  useEffect(() => {
    setActiveWorkspaceId(wsId)
  }, [wsId])

  useEffect(() => {
    let cancelled = false
    hydrateWorkspace(wsId)
      .then(() => { if (!cancelled) setHydratedWsId(wsId) })
      .catch(() => { if (!cancelled) setHydratedWsId(wsId) })
    return () => { cancelled = true }
  }, [wsId])

  if (hydratedWsId !== wsId) return null

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
