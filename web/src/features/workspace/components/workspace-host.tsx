import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { selectRepos, useSelect } from '@/core/entities/select'
import { markEnd, markStart } from '@/lib/perf/instrumentation'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
  getWorkspaceStore,
  setActiveWorkspaceId,
} from '../stores/workspace-store-registry'
import { WorkspaceStoreContext } from '../stores/workspace-context'
import { WorkspaceLayoutRoot } from './workspace-layout-root'
import { subscribeWorkspaceEviction } from '../lib/workspace-eviction-request'
import { planRetention } from '../lib/keep-alive-policy'
import { workspaceSlotStyling } from '../lib/workspace-slot-style'
import { WorkspaceView } from './workspace-view'

// Stable default so omitting `homeWsIds`/`paneWsIds` never produces a new
// array identity per render (avoids a spurious *WsIdsKey recompute).
const EMPTY_WS_IDS: string[] = []

// Membership-key delimiter: workspace ids can never contain NUL, so ids with
// spaces (or any other printable character) can't split the key wrongly.
const ID_DELIM = '\u0000'

// Sole lifecycle owner of the workspace-store registry: a workspace stays mounted (hidden, inert)
// iff it is active or in a view; reconcile is the only place a store is minted and the
// post-commit effect, after its subtree unmounted, the only place one is destroyed.
export function WorkspaceHost({
  activeWsId,
  homeWsIds = EMPTY_WS_IDS,
  paneWsIds = EMPTY_WS_IDS,
  viewWsIds = EMPTY_WS_IDS,
}: {
  activeWsId: string | null
  /** Workspaces some pane holds a chat or editor tab for; mounted so no pane reads the ambient
   *  fallback store for a workspace that exists. */
  paneWsIds?: string[]
  /** Home-workspace ids resolved this session. They are not sidebar rows, so without this they
   *  would be pruned as "closed" the moment they go hidden. */
  homeWsIds?: string[]
  /** Workspaces owning a chat in some Recents entry: the retention test. A non-active workspace
   *  is evicted the moment it drops out of this set, with no grace period. */
  viewWsIds?: string[]
}) {
  // Membership-only key, so sidebar churn (git status, renames) doesn't re-plan. The default
  // workspace is not a tree row (only repo.defaultWorkspaceId), so it is added explicitly or it
  // would be pruned as "closed".
  const existingIdsKey = useSelect((s) =>
    selectRepos(s)
      .flatMap((r) => [
        ...(r.defaultWorkspaceId ? [r.defaultWorkspaceId] : []),
        ...r.workspaces.map((w) => w.id),
      ])
      .sort()
      .join(ID_DELIM),
  )
  // Sorted-join keys for the id props: a fresh array literal with the same membership must not
  // re-plan.
  const homeWsIdsKey = homeWsIds.length ? [...homeWsIds].sort().join(ID_DELIM) : ''
  const paneWsIdsKey = paneWsIds.length ? [...paneWsIds].sort().join(ID_DELIM) : ''
  const existingIds = useMemo(() => {
    const ids = new Set(existingIdsKey ? existingIdsKey.split(ID_DELIM) : [])
    if (homeWsIdsKey) {
      for (const id of homeWsIdsKey.split(ID_DELIM)) ids.add(id)
    }
    return ids
  }, [existingIdsKey, homeWsIdsKey])

  const viewWsIdsKey = viewWsIds.length ? [...viewWsIds].sort().join(ID_DELIM) : ''
  const viewWsIdsSet = useMemo(
    () => new Set(viewWsIdsKey ? viewWsIdsKey.split(ID_DELIM) : []),
    [viewWsIdsKey],
  )

  // `mountedIds` drives rendering; `retainedRef` is what reconcile plans over, never read in
  // render. Reconcile is a layout effect so the stores it mints render before the first paint.
  const [mountedIds, setMountedIds] = useState<string[]>(EMPTY_WS_IDS)
  const retainedRef = useRef<Set<string> | null>(null)
  if (retainedRef.current === null) retainedRef.current = new Set()
  // Stores awaiting destruction: removed from the mounted set by a reconcile,
  // destroyed by the post-commit effect below once their subtree has unmounted.
  const pendingDestroyRef = useRef<string[]>([])

  // Latest inputs mirrored into refs so `reconcile` always runs against fresh
  // values without itself being re-created on every change.
  const activeWsIdRef = useRef(activeWsId)
  activeWsIdRef.current = activeWsId
  const existingIdsRef = useRef(existingIds)
  existingIdsRef.current = existingIds
  const paneWsIdsRef = useRef(paneWsIds)
  paneWsIdsRef.current = paneWsIds
  const viewWsIdsRef = useRef(viewWsIdsSet)
  viewWsIdsRef.current = viewWsIdsSet

  // Latest callback in a ref: the layout effect and forced eviction share one reconcile without
  // a stale closure.
  const reconcileRef = useRef<() => void>(() => {})
  reconcileRef.current = () => {
    const map = retainedRef.current!
    const active = activeWsIdRef.current

    // Active and pane-held workspaces enter the map so the plan evaluates them, and evicts a
    // pane's once nothing names it.
    if (active) map.add(active)
    for (const id of paneWsIdsRef.current) {
      if (id !== active) map.add(id)
    }

    // Prune closed/deleted workspaces, never the active one, and only once the sidebar has
    // loaded so a transient empty tree can't wipe live workspaces.
    const existing = existingIdsRef.current
    if (existing.size > 0) {
      for (const id of [...map]) {
        if (id !== active && !existing.has(id)) {
          map.delete(id)
          pendingDestroyRef.current.push(id)
        }
      }
    }

    // In use: a view member's workspace, or one some pane's editor tab reads.
    const viewIds = viewWsIdsRef.current
    const paneIds = new Set(paneWsIdsRef.current)
    const plan = planRetention(
      [...map].map((wsId) => ({ wsId, hasViewChat: viewIds.has(wsId) || paneIds.has(wsId) })),
      active,
    )
    for (const id of plan.evict) {
      map.delete(id)
      pendingDestroyRef.current.push(id)
    }

    // The one place a store is minted — the registry's keys are the plan.
    setActiveWorkspaceId(active)
    for (const id of plan.retain) getOrCreateWorkspaceStore(id)

    // Unmount first; the post-commit effect destroys pending stores. Always a fresh array, so
    // that effect flushes after every reconcile.
    setMountedIds(plan.retain)
  }

  // Warm-switch span: activating an already-retained workspace mounts nothing, so WorkspaceView's
  // cold-mount span can't see it. Declared before reconcile's layout effect so it reads the
  // retained set from before reconcile adds the new active workspace.
  const spanPrevActiveRef = useRef<string | null>(activeWsId)
  useLayoutEffect(() => {
    const prev = spanPrevActiveRef.current
    spanPrevActiveRef.current = activeWsId
    if (!activeWsId || activeWsId === prev) return
    // Only warm (already retained) targets; WorkspaceView spans cold mounts itself.
    const warm = retainedRef.current!.has(activeWsId)
    if (!warm) return
    markStart('workspace.switch')
    const raf = requestAnimationFrame(() => markEnd('workspace.switch'))
    return () => cancelAnimationFrame(raf)
  }, [activeWsId])

  // Every input that can flip retention updates synchronously with its state, so no timer.
  useLayoutEffect(() => {
    reconcileRef.current()
  }, [activeWsId, existingIds, paneWsIdsKey, viewWsIdsKey])

  // Forced eviction from the close path takes the same unmount-then-destroy route as a planned
  // one, never destroying a store directly. The active workspace is skipped: it is the route
  // and would be re-created at once.
  useEffect(
    () =>
      subscribeWorkspaceEviction((wsId) => {
        if (wsId === activeWsIdRef.current) return
        if (!retainedRef.current!.delete(wsId)) return
        pendingDestroyRef.current.push(wsId)
        reconcileRef.current()
      }),
    [],
  )

  // Destroy after unmount: deleted children clean up before this passive effect, so no slot is
  // live over a destroyed store. An id re-retained before the flush keeps its store.
  useEffect(() => {
    if (pendingDestroyRef.current.length === 0) return
    const batch = pendingDestroyRef.current
    pendingDestroyRef.current = []
    for (const id of batch) {
      if (retainedRef.current!.has(id)) continue
      destroyWorkspaceStore(id)
    }
  }, [mountedIds])

  // The host unmounts only when the route leaves the workspace surface (switches just change
  // activeWsId), so destroy every retained and pending store then.
  useEffect(
    () => () => {
      const doomed = new Set([...retainedRef.current!, ...pendingDestroyRef.current])
      retainedRef.current!.clear()
      pendingDestroyRef.current = []
      setActiveWorkspaceId(null)
      for (const id of doomed) {
        destroyWorkspaceStore(id)
      }
    },
    [],
  )

  return (
    <>
      {mountedIds.map((wsId) => {
        const store = getWorkspaceStore(wsId)
        if (!store) return null
        const isActive = wsId === activeWsId
        // The only place retention touches the DOM: active paints, retained slots are hidden + inert.
        const { style, inert } = workspaceSlotStyling(isActive)
        return (
          <div
            key={wsId}
            data-workspace-slot={wsId}
            data-active={isActive || undefined}
            style={style}
            inert={inert}
          >
            <WorkspaceView wsId={wsId} store={store} active={isActive} />
          </div>
        )
      })}
      <WindowPaneSurface mountedIds={mountedIds} />
    </>
  )
}

// The window's single pane tree, a sibling of the slots (layout-identical: an active slot is
// display:contents), so a workspace switch moves no pane DOM and hidden slots hold no copies.
function WindowPaneSurface({ mountedIds }: { mountedIds: readonly string[] }) {
  // Ambient fallback store, deliberately not the active one: following it would re-render every
  // pane on each cross-workspace focus. Panes provide their own store (pane-container.tsx).
  const ambientWsId = mountedIds[0]
  const store = ambientWsId ? getWorkspaceStore(ambientWsId) : undefined
  if (!store) return null
  return (
    <WorkspaceStoreContext.Provider value={store}>
      <WorkspaceLayoutRoot />
    </WorkspaceStoreContext.Provider>
  )
}
