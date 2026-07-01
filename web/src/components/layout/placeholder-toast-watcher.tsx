import { useEffect, useRef } from 'react'
import { useSidebarStore } from '@/lib/store/sidebar'
import { toast } from '@/features/window/stores/toast-store'
import { isPlaceholderWorkspace, placeholderReason } from '@/lib/workspace/placeholder'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'

// Watches sidebar state and fires ONE error toast per newly-observed placeholder
// protected workspace (spec §3.6). Per CLAUDE.md the toast is fired from a
// component watching store state, never a store/backend. Uses toast.show (the
// only variant carrying an action + a dedup key); the Fix… action opens the
// detach modal.
export function PlaceholderToastWatcher() {
  const repos = useSidebarStore((s) => s.repos)
  const openDetach = useDetachModalStore((s) => s.open)
  const seen = useRef(new Set<string>())

  useEffect(() => {
    for (const repo of repos) {
      for (const ws of repo.workspaces) {
        if (!isPlaceholderWorkspace(ws)) continue
        if (seen.current.has(ws.id)) continue
        seen.current.add(ws.id)
        toast.show({
          message: `Couldn't set up ${ws.branch}`,
          description: placeholderReason(ws),
          type: 'error',
          key: ws.id,
          action: {
            label: 'Fix…',
            onClick: () =>
              openDetach({ wsId: ws.id, branch: ws.branch, heldByPath: ws.heldByPath ?? '' }),
          },
        })
      }
    }
  }, [repos, openDetach])

  return null
}
