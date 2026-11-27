import { useEffect, useState, useSyncExternalStore } from 'react'
import { getIdentity, type IdentityDTO } from '@/features/git/api/identity-api'
import { report } from '@/lib/errors/report'
import { useHostResolved } from '@/lib/host-resolve'
import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'

const cache = new Map<string, IdentityDTO>()
const inFlight = new Map<string, Promise<IdentityDTO | null>>()

export async function resolveIdentity(wsId: string): Promise<IdentityDTO | null> {
  const cached = cache.get(wsId)
  if (cached) return cached
  const pending = inFlight.get(wsId)
  if (pending) return pending
  const promise = getIdentity(wsId)
    .then((result) => {
      cache.set(wsId, result)
      inFlight.delete(wsId)
      return result
    })
    .catch((err: unknown) => {
      inFlight.delete(wsId)
      report(err, { action: 'Load your Git identity' })
      return null
    })
  inFlight.set(wsId, promise)
  return promise
}

export function useCurrentIdentity(wsId: string): IdentityDTO | null {
  const [identity, setIdentity] = useState<IdentityDTO | null>(() => cache.get(wsId) ?? null)
  const owningChatId = useSyncExternalStore(
    (onChange) => subscribeToWorkspaceScope(wsId, onChange),
    () => getOwningChatId(wsId),
  )

  const hostResolved = useHostResolved({ wsId })

  useEffect(() => {
    if (owningChatId === null || !hostResolved) return
    let cancelled = false
    void resolveIdentity(wsId).then((result) => {
      if (!cancelled && result) setIdentity(result)
    })
    return () => {
      cancelled = true
    }
  }, [wsId, owningChatId, hostResolved])

  return identity
}
