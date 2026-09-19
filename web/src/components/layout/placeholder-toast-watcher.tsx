import { useEffect, useRef } from 'react'
import { useSidebarStore } from '@/lib/store/sidebar'
import { toast } from '@/features/window/stores/toast-store'
import { placeholderKind, placeholderReason } from '@/lib/workspace/placeholder'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'

// Watches sidebar state and fires ONE error toast per newly-observed
// UNPROVISIONED workspace (spec §3.6). Per CLAUDE.md the toast is fired from a
// component watching store state, never a store/backend. Uses toast.show (the
// only variant carrying an action + a dedup key); the Fix… action opens the
// detach modal.
//
// 'own-checkout' is deliberately silent: the repo's own main folder holding its
// own default branch is the resting state of every import, not a failure, so
// toasting it put a red "Couldn't set up main" on screen for every repo, every
// session, forever. That branch's Detach… lives on its row instead
// (sidebar-row-actions.tsx), where it waits to be wanted rather than
// interrupting.
export function PlaceholderToastWatcher() {
  const repos = useSidebarStore((s) => s.repos)
  const openDetach = useDetachModalStore((s) => s.open)
  const seen = useRef(new Set<string>())

  useEffect(() => {
    for (const repo of repos) {
      for (const ws of repo.workspaces) {
        if (placeholderKind(ws, repo.defaultBranch) !== 'unprovisioned') continue
        if (seen.current.has(ws.id)) continue
        seen.current.add(ws.id)
        toast.show({
          message: `Couldn't set up ${ws.branch}`,
          description: placeholderReason(ws, 'unprovisioned'),
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
