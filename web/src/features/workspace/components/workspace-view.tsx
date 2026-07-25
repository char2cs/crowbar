import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { WorkspaceStoreContext } from '../stores/workspace-context'
import { getOrCreateWorkspaceStore, setActiveWorkspaceId } from '../stores/workspace-store-registry'
import { setActiveWorkspaceStoreRef } from '../stores/workspace-store-ref'
import { hydrateWorkspace, reconcileWorkspaceBuffersWithDisk } from '@/lib/persistence/hydrate'
import { markStart, markEnd } from '@/lib/perf/instrumentation'
import { resetWorkspaceScopedStores } from '../lib/reset-workspace-scoped-stores'
import { markWorkspaceDeactivated } from '../lib/activation-freshness'
import { WorkspaceLayoutRoot } from './workspace-layout-root'
import { useWorkspaceEffects } from '../stores/hooks/use-workspace-effects'
import { useOpenOnNewTab } from '../stores/hooks/use-open-on-new-tab'
import { useSaveKeyboard } from '@/features/keymaps/hooks/use-save-keyboard'
import { usePaneKeyboard } from '@/features/panes/hooks/use-pane-keyboard'
import { useSidebarTabKeyboard } from '@/features/keymaps/hooks/use-sidebar-tab-keyboard'

interface WorkspaceViewProps {
  wsId: string
  /**
   * Whether this workspace is the one currently in view. WorkspaceHost keeps
   * recently-visited workspaces mounted (hidden via `display:none`) so switching
   * back is instant; only the active one owns the global active-store ref, the
   * active-workspace id, the keyboard handlers, and the file/git watchers.
   */
  active: boolean
}

export function WorkspaceView({ wsId, active }: WorkspaceViewProps) {
  const store = getOrCreateWorkspaceStore(wsId)
  // wsId is stable for a given WorkspaceView instance — WorkspaceHost keys each
  // retained workspace by id — so this hydrates exactly once per mount and never
  // re-hydrates on a warm re-activation.
  const [hydrated, setHydrated] = useState(false)

  // Only the active workspace publishes itself as THE active store / id. Hidden
  // workspaces stay mounted but must not steal the ref (imperative non-React
  // access resolves the active workspace) or the active-id used by scoped URLs.
  useLayoutEffect(() => {
    if (!active) return
    setActiveWorkspaceStoreRef(store)
    return () => {
      setActiveWorkspaceStoreRef(null)
    }
  }, [store, active])

  useEffect(() => {
    if (!active) return
    setActiveWorkspaceId(wsId)
  }, [wsId, active])

  // Clear the GLOBAL file-tree / git stores the instant this workspace becomes
  // active — synchronously, BEFORE the browser paints and BEFORE the active-only
  // watchers (WorkspaceActiveEffects, a passive effect) mount and refetch. Those
  // stores are keyed to the single visible workspace, so without this the file
  // explorer / git panel would paint the OUTGOING workspace's tree on the
  // activation frame (a workspace with no data of its own — e.g. home — or a slow
  // fetch would leave it on screen). Layout effects run before paint and before
  // passive effects, so the wrong workspace's tree is never shown and the clear
  // cannot race the incoming refetch. No-op when the stores already hold this
  // workspace's data (a warm return that kept its own tree isn't wiped).
  useLayoutEffect(() => {
    if (!active) return
    resetWorkspaceScopedStores(wsId)
  }, [wsId, active])

  // Stamp the moment this workspace goes hidden so its next warm return can keep
  // the already-loaded tree/git instead of re-seeding — provided it was hidden
  // only briefly and nothing clobbered the global stores meanwhile (the fast
  // path lives in use-workspace-effects; see activation-freshness). Correctness
  // is unaffected if the stamp is missed: the seed effects then re-fetch.
  useEffect(() => {
    if (active) return
    markWorkspaceDeactivated(wsId)
  }, [wsId, active])

  // Cold path: hydrate once on mount. A workspace only mounts when it first
  // becomes active, so this also opens the workspace.switch span for the cold
  // switch that brought it into view (closed below once hydration paints).
  // Destruction is NOT wired here anymore — WorkspaceHost destroys the store on
  // eviction/close.
  useEffect(() => {
    markStart('workspace.switch')
    let cancelled = false
    hydrateWorkspace(wsId)
      .then(() => {
        if (!cancelled) setHydrated(true)
      })
      .catch(() => {
        if (!cancelled) setHydrated(true)
      })
    return () => {
      cancelled = true
    }
  }, [wsId])

  // Cold markEnd: rAF defers past the first paint after hydration lands so the
  // span covers hydrate-to-pixels (M4 cold switch).
  useEffect(() => {
    if (!hydrated) return
    const raf = requestAnimationFrame(() => markEnd('workspace.switch'))
    return () => cancelAnimationFrame(raf)
  }, [hydrated])

  // Warm path (M4 warm switch): a hidden → active flip on an already-hydrated
  // workspace. No re-hydration and no subtree remount happen here. The
  // `workspace.switch` span for a warm activation is owned by WorkspaceHost (it
  // alone knows the target was retained rather than freshly mounted, and brackets
  // the flip before paint); opening it here — from a POST-paint passive effect —
  // measured an empty ~1-frame window after the switch had already painted.
  //
  // This effect's remaining job is disk reconciliation: the file watcher
  // (use-workspace-effects) is gated off while hidden, and agents/terminals keep
  // editing files in hidden worktrees — so a warm return must reconcile the open
  // buffers against disk (clean buffers silently reload, dirty ones get the
  // external-change flag), exactly as the cold path does via hydrateWorkspace's
  // restore-time reconcile.
  const wasActive = useRef(active)
  useEffect(() => {
    const previouslyActive = wasActive.current
    wasActive.current = active
    if (!active || previouslyActive || !hydrated) return
    void reconcileWorkspaceBuffersWithDisk(wsId).catch(() => {})
  }, [active, hydrated, wsId])

  useOpenOnNewTab(store, hydrated)

  if (!hydrated) return null

  return (
    <WorkspaceStoreContext.Provider value={store}>
      <WorkspaceLayoutRoot />
      {/* Watchers + keyboard run ONLY while active: use-workspace-effects writes
          the GLOBAL file-system / git stores (keyed to the single visible
          workspace), so a hidden workspace's WebSocket frames would clobber the
          active one. Mounting this only when active tears those subscriptions
          down on hide and re-seeds them on return — cheap fetches, not a remount
          or re-hydrate. */}
      {active && <WorkspaceActiveEffects wsId={wsId} />}
    </WorkspaceStoreContext.Provider>
  )
}

function WorkspaceActiveEffects({ wsId }: Pick<WorkspaceViewProps, 'wsId'>) {
  useWorkspaceEffects(wsId)
  useSaveKeyboard()
  usePaneKeyboard()
  useSidebarTabKeyboard()
  return null
}
