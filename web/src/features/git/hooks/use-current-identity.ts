import { useEffect, useRef, useState } from 'react'
import { getIdentity, type IdentityDTO } from '@/features/git/api/identity-api'

// Per-workspace in-memory cache so re-renders and sibling mounts do not
// re-fetch. The cache lives for the lifetime of the page — a full navigation
// (new project load) will naturally create a fresh hook instance.
const cache = new Map<string, IdentityDTO>()

/**
 * Fetches the GitHub/GitLab identity for the given workspace once and caches
 * the result in memory. Returns `null` while loading or if the fetch failed.
 */
export function useCurrentIdentity(wsId: string): IdentityDTO | null {
  const [identity, setIdentity] = useState<IdentityDTO | null>(() => cache.get(wsId) ?? null)
  const fetchedRef = useRef<string | null>(null)

  useEffect(() => {
    // Already cached — nothing to do.
    if (cache.has(wsId)) {
      setIdentity(cache.get(wsId)!)
      return
    }

    // Prevent double-fetch if the same wsId is already in-flight.
    if (fetchedRef.current === wsId) return
    fetchedRef.current = wsId

    let cancelled = false
    getIdentity(wsId)
      .then((result) => {
        if (cancelled) return
        cache.set(wsId, result)
        setIdentity(result)
      })
      .catch(() => {
        // Leave identity as null; callers can degrade gracefully.
      })

    return () => {
      cancelled = true
    }
  }, [wsId])

  return identity
}
