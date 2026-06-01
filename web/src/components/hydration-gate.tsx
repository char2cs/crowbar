import { useEffect, useState } from 'react'
import type { ReactNode } from 'react'
import { hydratePreferences, hydrateSidebar } from '@/lib/persistence/hydrate'

interface HydrationGateProps {
  children: ReactNode
}

export function HydrationGate({ children }: HydrationGateProps) {
  const [hydrated, setHydrated] = useState(false)

  useEffect(() => {
    Promise.all([hydratePreferences(), hydrateSidebar()])
      .then(() => setHydrated(true))
      .catch(() => setHydrated(true))
  }, [])

  if (!hydrated) return null

  return <>{children}</>
}
