import { useEffect, useState, useSyncExternalStore } from 'react'
import { getIdentity, type IdentityDTO } from '@/features/git/api/identity-api'
import { report } from '@/lib/errors/report'
import { useHostResolved } from '@/lib/host-resolve'
import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'

// Per-workspace cache for the page's lifetime, so re-renders and sibling mounts don't refetch.
const cache = new Map<string, IdentityDTO>()
// In-flight fetches so concurrent resolveIdentity() callers share one request.
const inFlight = new Map<string, Promise<IdentityDTO | null>>()

/** Resolves the identity, awaiting the fetch if uncached. Event handlers (posting a comment) use
 *  this, not the hook, which is null while loading and would post with an empty author. */
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

/** The workspace's forge identity, fetched once and cached; null while loading or on failure. */
export function useCurrentIdentity(wsId: string): IdentityDTO | null {
  const [identity, setIdentity] = useState<IdentityDTO | null>(() => cache.get(wsId) ?? null)
  // The identity route needs the owning chat id, which the sidebar may record
  // after this mounts; subscribing re-runs the effect the moment it lands.
  const owningChatId = useSyncExternalStore(
    (onChange) => subscribeToWorkspaceScope(wsId, onChange),
    () => getOwningChatId(wsId),
  )

  const hostResolved = useHostResolved({ wsId })

  useEffect(() => {
    // Nothing to fetch yet — wait for owningChatId and the workspace's host
    // rather than firing a call that can only resolve to null.
    if (owningChatId === null || !hostResolved) return
    let cancelled = false
    // Share the cache + in-flight request used by resolveIdentity, so the identity always lands
    // here even when a sibling (e.g. a comment submit) populated it first.
    void resolveIdentity(wsId).then((result) => {
      if (!cancelled && result) setIdentity(result)
    })
    return () => {
      cancelled = true
    }
  }, [wsId, owningChatId, hostResolved])

  return identity
}
