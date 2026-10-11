import { useCallback, useEffect, useRef, useState } from 'react'
import { browseDirectory } from '@/features/hosts/lib/browse'
import type { Listing } from '@/features/hosts/lib/folder-rows'
import { reportHostFailure } from '@/features/hosts/lib/host-failures'
import { onHostUp } from '@/features/hosts/lib/host-status'

export const ROOT_PATH = '/'

interface FolderListings {
  listings: Readonly<Record<string, Listing>>
  expanded: ReadonlySet<string>
  expand: (path: string) => void
  collapse: (path: string) => void
  /** Lists `path` again if its listing failed. */
  retry: (path: string) => void
}

/**
 * The directories of `hostId` listed so far — the owner is keyed by `hostId`, so no listing
 * outlives its host. Each directory is fetched when first shown (the root, or a folder being
 * expanded) and only while `canAsk`; when the host is up again every shown directory without
 * a listing is fetched.
 */
export function useFolderListings(hostId: string, canAsk: boolean): FolderListings {
  const [listings, setListings] = useState<Record<string, Listing>>({})
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set())
  const latest = useRef({ listings, expanded, canAsk })
  latest.current = { listings, expanded, canAsk }
  // A listing is applied only by its latest request, and only while mounted.
  const inFlight = useRef(new Map<string, number>())
  const nextRequest = useRef(0)
  const mounted = useRef(false)

  const fetchListing = useCallback(
    (path: string) => {
      const request = ++nextRequest.current
      inFlight.current.set(path, request)
      setListings((prev) => ({ ...prev, [path]: { status: 'loading' } }))
      const settle = (listing: Listing): boolean => {
        if (!mounted.current || inFlight.current.get(path) !== request) return false
        inFlight.current.delete(path)
        setListings((prev) => ({ ...prev, [path]: listing }))
        return true
      }
      browseDirectory(hostId, path).then(
        (entries) => settle({ status: 'loaded', entries }),
        (error: unknown) => {
          if (settle({ status: 'failed', error })) reportHostFailure(error)
        },
      )
    },
    [hostId],
  )

  const listMissing = useCallback(
    (paths: Iterable<string>) => {
      for (const path of paths) {
        const listed = latest.current.listings[path]?.status === 'loaded'
        if (!listed && !inFlight.current.has(path)) fetchListing(path)
      }
    },
    [fetchListing],
  )

  const ensureListed = useCallback(
    (paths: Iterable<string>) => {
      if (latest.current.canAsk) listMissing(paths)
    },
    [listMissing],
  )

  useEffect(() => {
    const requests = inFlight.current
    mounted.current = true
    ensureListed([ROOT_PATH])
    // Called from the status change itself, before this component re-renders with it.
    const stopWatching = onHostUp(hostId, () =>
      listMissing([ROOT_PATH, ...latest.current.expanded]),
    )
    return () => {
      mounted.current = false
      requests.clear()
      stopWatching()
    }
  }, [hostId, ensureListed, listMissing])

  const expand = useCallback(
    (path: string) => {
      setExpanded((prev) => (prev.has(path) ? prev : new Set(prev).add(path)))
      ensureListed([path])
    },
    [ensureListed],
  )

  const collapse = useCallback((path: string) => {
    setExpanded((prev) => {
      if (!prev.has(path)) return prev
      const next = new Set(prev)
      next.delete(path)
      return next
    })
  }, [])

  return { listings, expanded, expand, collapse, retry: (path) => ensureListed([path]) }
}
