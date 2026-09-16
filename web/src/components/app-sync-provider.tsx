import type { ReactNode } from 'react'
import { useAppSyncEngine, SUBSCRIPTION_GRACE_MS } from './app-sync-engine'

export { SUBSCRIPTION_GRACE_MS }

/**
 * Mounts the §7 sync engine at the app root (see `app-sync-engine.ts`'s own
 * doc comment for the full startup/subscription-by-visibility mechanism) and
 * renders `children` straight through — this component owns no state or DOM
 * of its own; it exists only to run `useAppSyncEngine` for the app's lifetime.
 */
export function AppSyncProvider({ children }: { children: ReactNode }) {
  useAppSyncEngine()
  return <>{children}</>
}
