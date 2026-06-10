import { useEffect, useLayoutEffect, useState } from 'react'
import { WorkspaceStoreContext } from '../stores/workspace-context'
import { getOrCreateWorkspaceStore, setActiveWorkspaceId } from '../stores/workspace-store-registry'
import { setActiveWorkspaceStoreRef } from '../stores/workspace-store-ref'
import { hydrateWorkspace } from '@/lib/persistence/hydrate'
import { WorkspaceLayoutRoot } from './workspace-layout-root'
import { useWorkspaceEffects } from '../stores/hooks/use-workspace-effects'
import { BrowserPaneEventListener } from '@/features/web-viewer/components/browser-pane-event-listener'
import { useSaveKeyboard } from '@/features/keymaps/hooks/use-save-keyboard'
import { usePaneKeyboard } from '@/features/panes/hooks/use-pane-keyboard'

interface WorkspaceViewProps {
  wsId: string
}

export function WorkspaceView({ wsId }: WorkspaceViewProps) {
  const store = getOrCreateWorkspaceStore(wsId)
  const [hydratedWsId, setHydratedWsId] = useState<string | null>(null)

  useLayoutEffect(() => {
    setActiveWorkspaceStoreRef(store)
    return () => {
      setActiveWorkspaceStoreRef(null)
    }
  }, [store])

  useEffect(() => {
    setActiveWorkspaceId(wsId)
  }, [wsId])

  useEffect(() => {
    let cancelled = false
    hydrateWorkspace(wsId)
      .then(() => {
        if (!cancelled) setHydratedWsId(wsId)
      })
      .catch(() => {
        if (!cancelled) setHydratedWsId(wsId)
      })
    return () => {
      cancelled = true
    }
  }, [wsId])

  if (hydratedWsId !== wsId) return null

  return (
    <WorkspaceStoreContext.Provider value={store}>
      <WorkspaceViewInner wsId={wsId} />
    </WorkspaceStoreContext.Provider>
  )
}

function WorkspaceViewInner({ wsId }: Pick<WorkspaceViewProps, 'wsId'>) {
  useWorkspaceEffects(wsId)
  useSaveKeyboard()
  usePaneKeyboard()
  return (
    <>
      <BrowserPaneEventListener />
      <WorkspaceLayoutRoot />
    </>
  )
}
