import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { hydrateFromIDB } from '@/lib/persistence/hydrate'

interface HydrationGateProps {
  workspaceId: string
  children: ReactNode
}

export function HydrationGate({ workspaceId, children }: HydrationGateProps) {
  const [hydrated, setHydrated] = useState(false)

  useEffect(() => {
    if (!workspaceId) {
      setHydrated(true)
      return
    }
    hydrateFromIDB(workspaceId).then(() => setHydrated(true)).catch(() => setHydrated(true))
  }, [workspaceId])

  if (!hydrated) return null

  return <>{children}</>
}
