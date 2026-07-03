import type { Workspace } from '@/lib/store/sidebar'
import { placeholderReason } from '@/lib/workspace/placeholder'
import { retryProvision } from '@/lib/api/workspace'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'
import { toast } from '@/features/window/stores/toast-store'

// The inline surface for a placeholder row (spec §3.3): a reconstructed reason
// plus Retry and Detach… actions. Retry re-provisions in place; Detach… opens the
// consent modal (only when a holder path is known — an unknown holder can only be
// retried). Reason + gating are derived from heldByPath; there is no persisted
// lastError (spec §4/B7).
export function PlaceholderRowActions({ workspace }: { workspace: Workspace }) {
  const openDetach = useDetachModalStore((s) => s.open)

  const onRetry = (e: React.MouseEvent) => {
    e.stopPropagation()
    retryProvision(workspace.id).catch((err: unknown) => {
      toast.show({
        message: `Couldn't retry ${workspace.branch}`,
        description: err instanceof Error ? err.message : String(err),
        type: 'error',
        key: `retry-${workspace.id}`,
      })
    })
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
    <div className="flex flex-col gap-1.5">
      <p className="text-xs leading-relaxed text-muted-foreground">
        {placeholderReason(workspace)}
      </p>
      <div className="flex gap-1.5">
        <button
          type="button"
          className="rounded-lg border border-border/60 px-3 py-1.5 text-[13px] text-foreground/80 hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
          onClick={onRetry}
          onPointerDown={(e) => e.stopPropagation()}
        >
          Retry
        </button>
        {workspace.heldByPath ? (
          <button
            type="button"
            className="rounded-lg border border-border/60 px-3 py-1.5 text-[13px] text-foreground/80 hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
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
