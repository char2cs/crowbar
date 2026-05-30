import { useEffect, useState } from 'react'
import { hydrateFromIDB } from '@/lib/persistence/hydrate'

interface HydrationGateProps {
  workspaceId: string
  children: React.ReactNode
}

export function HydrationGate({ workspaceId, children }: HydrationGateProps) {
  const [hydrated, setHydrated] = useState(false)

  useEffect(() => {
    hydrateFromIDB(workspaceId).then(() => setHydrated(true))
  }, [workspaceId])

  if (!hydrated) return null

  return <>{children}</>
}
