import { useEffect, useState, useSyncExternalStore } from 'react'
import { getReviewOutline, type FileOutline } from '@/features/git/api/review-window-api'
import { useHostResolved } from '@/lib/host-resolve'
import { getOwningChatId, subscribeToWorkspaceScope } from '@/lib/workspace-scope'
import { onGitStatusChanged } from '@/features/git/stores/git-refresh'

export interface UseReviewOutlineResult {
  outline: FileOutline[]
  loaded: boolean
}

const EMPTY_OUTLINE: FileOutline[] = []

export function useReviewOutline(wsId: string | null, commit?: string): UseReviewOutlineResult {
  const [outline, setOutline] = useState<FileOutline[]>(EMPTY_OUTLINE)
  const [loaded, setLoaded] = useState(false)

  const scopeKey = `${wsId ?? ''}\u0000${commit ?? ''}`
  const [prevScope, setPrevScope] = useState(scopeKey)
  if (prevScope !== scopeKey) {
    setPrevScope(scopeKey)
    setOutline(EMPTY_OUTLINE)
    setLoaded(false)
  }

  const owningChatId = useSyncExternalStore(
    (onChange) => (wsId ? subscribeToWorkspaceScope(wsId, onChange) : () => {}),
    () => (wsId ? getOwningChatId(wsId) : null),
  )

  const hostResolved = useHostResolved({ wsId: wsId ?? '' })

  useEffect(() => {
    if (!wsId) return
    if (owningChatId === null || !hostResolved) return

    let cancelled = false

    const fetchOutline = async () => {
      try {
        const next = await getReviewOutline({ wsId, commit })
        if (cancelled) return
        setOutline(next)
        setLoaded(true)
      } catch {
        return
      }
    }

    void fetchOutline()

    if (commit)
      return () => {
        cancelled = true
      }

    const stop = onGitStatusChanged(wsId, () => void fetchOutline())

    return () => {
      cancelled = true
      stop()
    }
  }, [wsId, commit, owningChatId, hostResolved])

  return { outline, loaded }
}
