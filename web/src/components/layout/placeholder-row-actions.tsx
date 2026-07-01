import type { Workspace } from '@/lib/store/sidebar'
import { placeholderReason } from '@/lib/workspace/placeholder'
import { retryProvision } from '@/lib/api/workspace'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'

// The inline surface for a placeholder row (spec §3.3): a reconstructed reason
// plus Retry and Detach… actions. Retry re-provisions in place; Detach… opens the
// consent modal (only when a holder path is known — an unknown holder can only be
// retried). Reason + gating are derived from heldByPath; there is no persisted
// lastError (spec §4/B7).
export function PlaceholderRowActions({ workspace }: { workspace: Workspace }) {
  const openDetach = useDetachModalStore((s) => s.open)

  const onRetry = (e: React.MouseEvent) => {
    e.stopPropagation()
    void retryProvision(workspace.id)
  }

  const onDetach = (e: React.MouseEvent) => {
    e.stopPropagation()
    openDetach({
      wsId: workspace.id,
      branch: workspace.branch,
      heldByPath: workspace.heldByPath ?? '',
    })
  }

  return (
    <div className="flex flex-col gap-1 pl-6 pr-2 pb-1">
      <p className="text-xs text-muted-foreground">{placeholderReason(workspace)}</p>
      <div className="flex gap-2">
        <button
          type="button"
          className="rounded-md px-2 py-0.5 text-xs text-foreground/70 hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          onClick={onRetry}
          onPointerDown={(e) => e.stopPropagation()}
        >
          Retry
        </button>
        {workspace.heldByPath ? (
          <button
            type="button"
            className="rounded-md px-2 py-0.5 text-xs text-foreground/70 hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
            onClick={onDetach}
            onPointerDown={(e) => e.stopPropagation()}
          >
            Detach…
          </button>
        ) : null}
      </div>
    </div>
  )
}
