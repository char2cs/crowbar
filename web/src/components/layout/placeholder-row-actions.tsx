import { Button } from '@/components/ui/button'
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
//
// Oracle anchors (native/oracle/ANCHORS.md): `data-oracle-content-sized="true"`
// is on both `<Button>`s — `size="sm"` authors no width, so each button's used
// width is its own label's, the same v1.5 declaration `inline-error.tsx`'s
// retry control carries for the identical reason. Neither carries
// `data-oracle-line-sized`: `size="sm"` authors `h-8 sm:h-7`, so the box is not
// derived from the label's line box (`badge`/`inline-error`'s rule). Both ids
// are renamed away from `button`'s own default (v1.8, `git-row-badge`'s
// precedent) into this call site's own namespace, so no `oracleSurfaceScope`
// entry is needed — the port composes these boxes from `button`'s public
// values rather than nesting a second `Button::render`, so there is no
// foreign content left to filter.
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
    <div className="flex flex-col gap-1.5" data-oracle-id="placeholder-row-actions">
      <p
        className="text-xs leading-relaxed text-muted-foreground"
        data-oracle-id="placeholder-row-actions-reason"
      >
        {placeholderReason(workspace)}
      </p>
      <div className="flex justify-end gap-1.5" data-oracle-id="placeholder-row-actions-actions">
        <Button
          variant="outline"
          size="sm"
          onClick={onRetry}
          onPointerDown={(e) => e.stopPropagation()}
          data-oracle-id="placeholder-row-actions-retry"
          data-oracle-content-sized="true"
        >
          Retry
        </Button>
        {workspace.heldByPath ? (
          <Button
            size="sm"
            onClick={onDetach}
            onPointerDown={(e) => e.stopPropagation()}
            data-oracle-id="placeholder-row-actions-detach"
            data-oracle-content-sized="true"
          >
            Detach…
          </Button>
        ) : null}
      </div>
    </div>
  )
}
