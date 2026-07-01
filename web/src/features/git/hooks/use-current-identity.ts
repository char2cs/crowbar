import { useEffect, useState } from 'react'
import { getIdentity, type IdentityDTO } from '@/features/git/api/identity-api'

// Per-workspace in-memory cache so re-renders and sibling mounts do not
// re-fetch. The cache lives for the lifetime of the page — a full navigation
// (new project load) will naturally create a fresh hook instance.
const cache = new Map<string, IdentityDTO>()
// In-flight fetches so concurrent resolveIdentity() callers share one request.
const inFlight = new Map<string, Promise<IdentityDTO | null>>()

/**
 * Eagerly resolve the workspace identity, awaiting the network fetch if it is
 * not cached yet. Use this in event handlers (e.g. posting a comment) so the
 * author is ALWAYS the real GitHub login — the hook returns `null` while the
 * fetch is in flight, which previously caused comments posted before identity
 * loaded to be stored with an empty author.
 */
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
    .catch(() => {
      inFlight.delete(wsId)
      return null
    })
  inFlight.set(wsId, promise)
  return promise
}

/**
 * Fetches the GitHub/GitLab identity for the given workspace once and caches
 * the result in memory. Returns `null` while loading or if the fetch failed.
 */
export function useCurrentIdentity(wsId: string): IdentityDTO | null {
  const [identity, setIdentity] = useState<IdentityDTO | null>(() => cache.get(wsId) ?? null)

  useEffect(() => {
    let cancelled = false
    // Share the cache + in-flight request used by resolveIdentity, so the
    // identity always lands here even when a sibling (e.g. a comment submit)
    // populated it first.
    void resolveIdentity(wsId).then((result) => {
      if (!cancelled && result) setIdentity(result)
    })
    return () => {
      cancelled = true
    }
  }, [wsId])

  return identity
}
