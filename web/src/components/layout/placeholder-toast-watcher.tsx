import { useEffect, useRef } from 'react'
import { selectRepos, useSelect } from '@/core/entities/select'
import { toast } from '@/features/window/stores/toast-store'
import { placeholderKind, placeholderReason } from '@/lib/workspace/placeholder'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'
import { useShallow } from 'zustand/react/shallow'
import {
  newestProblem,
  selectEntityBusy,
  selectFailedOps,
  useOperations,
  operationsState,
} from '@/core/entities/operations'

// ONE error toast (with a Fix… detach action) per newly seen UNPROVISIONED workspace (§3.6).
// 'own-checkout' stays silent: it is every import's resting state; its Detach… lives on the row.
// A placeholder still being made, or whose operation failed, is the operations watcher's to toast.
export function PlaceholderToastWatcher() {
  const repos = useSelect(selectRepos)
  const openDetach = useDetachModalStore((s) => s.open)
  const failedOps = useOperations(useShallow(selectFailedOps))
  const seen = useRef(new Set<string>())

  useEffect(() => {
    for (const repo of repos) {
      for (const ws of repo.workspaces) {
        if (placeholderKind(ws, repo.defaultBranch) !== 'unprovisioned') continue
        if (seen.current.has(ws.id)) continue
        const named = [ws.id, ws.owningChatId ?? '']
        const ops = operationsState()
        if (named.some((id) => id && selectEntityBusy(ops, id))) continue
        seen.current.add(ws.id)
        if (newestProblem(failedOps, named)) continue
        toast.show({
          message: `Couldn't set up ${ws.branch}`,
          description: placeholderReason(ws, 'unprovisioned', null),
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
  }, [repos, openDetach, failedOps])

  return null
}
