import { apiFetch } from '@/lib/api/fetch'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'
import { daemonStateFromHealth, reportMigrating } from './connection-store'

// One attempt: the socket that failed is already retrying, and each failed
// retry asks again.
const ONCE = { attempts: 1, baseDelayMs: 0, maxDelayMs: 0 }

let inFlight: Promise<void> | null = null

async function readHealth(): Promise<void> {
  const state = daemonStateFromHealth(await apiFetch<unknown>('/v0/health', undefined, ONCE))
  if (state?.kind === 'migrating') reportMigrating(state.migrating)
  if (state?.kind === 'halted') useDaemonHaltStore.getState().halt(state.message)
}

/** Asks the daemon why a socket was refused. A read that fails says nothing
 *  new, so it leaves the state as it is. */
export function probeDaemon(): Promise<void> {
  inFlight ??= readHealth()
    .catch((err: unknown) => {
      if (import.meta.env.DEV) console.warn('daemon health probe failed:', err)
    })
    .finally(() => {
      inFlight = null
    })
  return inFlight
}
