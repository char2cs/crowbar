import { useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useSidebarStore } from '@/lib/store/sidebar'
import { markEnd, markStart } from '@/lib/perf/instrumentation'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '../stores/workspace-store-registry'
import { WorkspaceStoreContext } from '../stores/workspace-context'
import { WorkspaceLayoutRoot } from './workspace-layout-root'
import { subscribeWorkspaceEviction } from '../lib/workspace-eviction-request'
import { planRetention, RETENTION_CAP } from '../lib/keep-alive-policy'
import { workspaceSlotStyling } from '../lib/workspace-slot-style'
import { WorkspaceView } from './workspace-view'

// Stable default so omitting `homeWsIds`/`paneWsIds` never produces a new
// array identity per render (avoids a spurious *WsIdsKey recompute).
const EMPTY_WS_IDS: string[] = []

// Membership-key delimiter: workspace ids can never contain NUL, so ids with
// spaces (or any other printable character) can't split the key wrongly.
const ID_DELIM = '\u0000'

/**
 * Retention manager for the workspace surface — "in memory iff in a view"
 * (`keep-alive-policy.ts`'s new rule; there is no more configurable
 * keep-alive window).
 *
 * Instead of unmounting the whole workspace subtree (and destroying its store)
 * on every switch, WorkspaceHost keeps every workspace with a view mounted but
 * hidden (`display:none` + `inert`) so switching back is instant. A single
 * pure policy (`planRetention`) decides what to keep. The active workspace is
 * always retained; every other one is retained only while `viewWsIds` still
 * names it. There is no timer: whether a workspace has a view-chat changes
 * synchronously with view/chat state, so reconcile runs off that state
 * directly rather than off a clock.
 *
 * A workspace is destroyed when it is evicted (no longer has a view and isn't
 * active, or was pushed out by the hard cap) or when it no longer exists
 * (closed / deleted — pruned against the sidebar's live workspace set).
 * Destruction is deferred until AFTER the subtree has unmounted: the
 * components living over the store (Monaco panes, terminal slots) must never
 * render against a destroyed store, so reconcile only removes the id from the
 * mounted set and a post-commit effect destroys the store once React has torn
 * the subtree down.
 */
export function WorkspaceHost({
  activeWsId,
  homeWsIds = EMPTY_WS_IDS,
  paneWsIds = EMPTY_WS_IDS,
  viewWsIds = EMPTY_WS_IDS,
}: {
  activeWsId: string | null
  /**
   * Every workspace id some PANE currently holds a chat for (see
   * `use-chat-workspace-id.ts`'s `usePaneWorkspaceIds`) — not just the active
   * one. A split can show chats from workspaces this host never mounted
   * (never routed to, never clicked into): with no real store to read,
   * `PaneContainer`'s `chatStore` fell back to whichever workspace happened
   * to be AMBIENT, so an unclicked pane rendered blank/wrong and then
   * appeared to "switch chat" the instant some OTHER pane's click changed
   * what the fallback resolved to — caught live, a two-repo split. Force-
   * mounting every one of these (below, alongside `activeWsId`'s own
   * blank-frame guard) gives every visible pane a real store from the start,
   * so the ambient fallback is never reached for one that actually exists.
   */
  paneWsIds?: string[]
  /**
   * Home-workspace ids (any project) resolved so far this session — see
   * home-workspace-resolver.ts. Home is a project-level concept, not a repo
   * workspace, so it never appears in the sidebar's repo/workspace id set
   * this host otherwise prunes retained ids against; without this, a home
   * workspace would look "closed" the instant it goes hidden and get
   * destroyed instead of retained like any other workspace.
   */
  homeWsIds?: string[]
  /**
   * Every workspace id that currently owns >=1 chat present in some Recents
   * entry — see `use-chat-workspace-id.ts`'s `useViewWorkspaceIds`. THE
   * retention test: a non-active workspace is kept mounted only while this
   * still names it, and is evicted the moment it drops out (a view closed,
   * its last chat removed) — no grace period.
   */
  viewWsIds?: string[]
}) {
  // A stable, membership-only projection of every existing workspace id. Keyed
  // as a delimited string so this only changes identity when a workspace is
  // actually added or removed — not on every unrelated sidebar mutation (git
  // status frames, rename, etc.). The DEFAULT (main-worktree) workspace is not
  // a tree row — it exists only as repo.defaultWorkspaceId — so it must be
  // included explicitly, or leaving it while any child workspace exists would
  // prune-and-destroy it as "closed".
  const existingIdsKey = useSidebarStore((s) =>
    s.repos
      .flatMap((r) => [
        ...(r.defaultWorkspaceId ? [r.defaultWorkspaceId] : []),
        ...r.workspaces.map((w) => w.id),
      ])
      .sort()
      .join(ID_DELIM),
  )
  // Stable key for the home ids too (same "sort + join" trick as
  // existingIdsKey) so the memo below only recomputes when the actual set of
  // known home ids changes, not on every render `homeWsIds` is passed a fresh
  // array literal.
  const homeWsIdsKey = homeWsIds.length ? [...homeWsIds].sort().join(ID_DELIM) : ''
  // Same trick again for the pane ids — stable across renders that pass a
  // fresh array literal with the same membership, so it only ever changes
  // identity when a pane actually starts or stops naming a NEW workspace.
  const paneWsIdsKey = paneWsIds.length ? [...paneWsIds].sort().join(ID_DELIM) : ''
  const existingIds = useMemo(() => {
    const ids = new Set(existingIdsKey ? existingIdsKey.split(ID_DELIM) : [])
    if (homeWsIdsKey) {
      for (const id of homeWsIdsKey.split(ID_DELIM)) ids.add(id)
    }
    return ids
  }, [existingIdsKey, homeWsIdsKey])

  // Same trick again for the view ids (workspaces Recents currently tracks a
  // chat for) — the new retention test itself.
  const viewWsIdsKey = viewWsIds.length ? [...viewWsIds].sort().join(ID_DELIM) : ''
  const viewWsIdsSet = useMemo(
    () => new Set(viewWsIdsKey ? viewWsIdsKey.split(ID_DELIM) : []),
    [viewWsIdsKey],
  )

  // The list of mounted workspace ids drives rendering. `lastActiveRef` is
  // kept only as an LRU tie-break signal for the hard cap (see
  // keep-alive-policy.ts) — never read during render. activeWsId is null on
  // the project-home route (no workspace in view); the host still stays
  // mounted so its retention survives the home transit.
  const initialIds = activeWsId ? [...new Set([activeWsId, ...paneWsIds])] : [...new Set(paneWsIds)]
  const [mountedIds, setMountedIds] = useState<string[]>(initialIds)
  // Lazy ref init (null-guarded): the Map only needs to be built once, at
  // mount, not as a throwaway useRef() arg re-evaluated on every render.
  const lastActiveRef = useRef<Map<string, number> | null>(null)
  if (lastActiveRef.current === null) {
    const now = Date.now()
    lastActiveRef.current = new Map(initialIds.map((id) => [id, now]))
  }
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

  // "Latest callback in a ref" — same shape the old timer-based version used,
  // kept even though nothing re-arms itself any more, so every call site below
  // (the effect, forced eviction) shares one reconcile without a stale closure.
  const reconcileRef = useRef<() => void>(() => {})
  reconcileRef.current = () => {
    const now = Date.now()
    const map = lastActiveRef.current!
    const active = activeWsIdRef.current

    // Track the active workspace so it participates in cap bookkeeping (it is
    // always retained regardless — see planRetention — this is purely so it
    // has an entry to report through `retain`/`evict`).
    if (active) map.set(active, now)

    // Track every workspace a PANE currently holds a chat for, the same way
    // `active` is tracked above. A pane's chat is by construction held by a
    // view record, so `viewWsIds` already covers WHY these are retained — this just makes
    // sure they're in the map at all, so they're evaluated (and, once no pane
    // or view names them any longer, actually evicted) instead of rendering
    // forever through the force-mount guard below with no store ever
    // destroyed.
    for (const id of paneWsIdsRef.current) {
      if (id !== active) map.set(id, now)
    }

    // Prune workspaces that no longer exist (closed / deleted). Never the
    // active one, and only once the sidebar has actually loaded, so a transient
    // empty tree can't wipe live workspaces.
    const existing = existingIdsRef.current
    if (existing.size > 0) {
      for (const id of [...map.keys()]) {
        if (id !== active && !existing.has(id)) {
          map.delete(id)
          pendingDestroyRef.current.push(id)
        }
      }
    }

    const viewIds = viewWsIdsRef.current
    const plan = planRetention(
      [...map].map(([wsId, lastActiveAt]) => ({
        wsId,
        hasViewChat: viewIds.has(wsId),
        lastActiveAt,
      })),
      active,
      RETENTION_CAP,
    )
    for (const id of plan.evict) {
      map.delete(id)
      pendingDestroyRef.current.push(id)
    }

    // Unmount first — the post-commit effect below destroys the pending stores
    // once React has torn their subtrees down. Always a fresh array, so the
    // effect re-fires (and flushes) after every reconcile.
    setMountedIds(plan.retain)
  }

  // Re-plan whenever the active workspace, the set of existing workspaces, the
  // set of pane-referenced workspaces, or the set of view-tracked workspaces
  // changes — no more timer: a workspace's view-chat either exists right now
  // or it doesn't, and every one of these signals updates synchronously with
  // the state that could flip it.
  useEffect(() => {
    reconcileRef.current()
  }, [activeWsId, existingIds, paneWsIdsKey, viewWsIdsKey])

  // FORCED EVICTION — the close path asking for a workspace to go NOW,
  // outside the retention window entirely (see workspace-eviction-request.ts).
  // Retention exists to keep a workspace warm for a switch BACK; a workspace
  // whose last view the user just closed is the opposite case, and aging it
  // out over the keep-alive window would hold its whole live surface (chats
  // stream, LSP, terminal transports, file watcher, Monaco registry) resident
  // for something nobody can see.
  //
  // The request is honoured through the SAME unmount-then-destroy path an
  // ordinary eviction takes — dropped from the retention map, queued for the
  // post-commit destroy effect below, then reconciled so React tears the
  // subtree down first. It never destroys a store directly, and the ACTIVE
  // workspace is skipped outright: it is the route, mounted, and would be
  // re-created the instant it went away.
  useEffect(
    () =>
      subscribeWorkspaceEviction((wsId) => {
        if (wsId === activeWsIdRef.current) return
        if (!lastActiveRef.current!.delete(wsId)) return
        pendingDestroyRef.current.push(wsId)
        reconcileRef.current()
      }),
    [],
  )

  // WARM-SWITCH span (M4 warm). When the active id changes to a workspace that
  // was ALREADY retained (kept mounted + hidden), becoming active is a display
  // flip + terminal/editor refit — no cold mount, no re-hydrate. WorkspaceView
  // spans its own COLD mount (hydrate→paint); a warm activation leaves no fresh
  // mount to span, so the switch was previously invisible to instrumentation.
  // Bracket it here across one paint so warm switches (the common case, and the
  // whole point of keep-alive) are permanently measurable under the same
  // `workspace.switch` name. This runs as a LAYOUT effect — before the reconcile
  // passive effect appends a cold id, so `lastActiveRef` still distinguishes a
  // retained (warm) target from a brand-new (cold) one — and before paint, so
  // the span covers the flip's reflow through the next frame.
  const spanPrevActiveRef = useRef<string | null>(activeWsId)
  useLayoutEffect(() => {
    const prev = spanPrevActiveRef.current
    spanPrevActiveRef.current = activeWsId
    if (!activeWsId || activeWsId === prev) return
    // The incoming id is in the retention map iff this is a WARM activation (the
    // target was kept mounted + hidden). A cold target isn't in the map yet —
    // WorkspaceView owns its own cold-mount span — so only bracket warm switches.
    const warm = lastActiveRef.current!.has(activeWsId)
    if (!warm) return
    markStart('workspace.switch')
    const raf = requestAnimationFrame(() => markEnd('workspace.switch'))
    return () => cancelAnimationFrame(raf)
  }, [activeWsId])

  // DESTROY AFTER UNMOUNT. This effect runs post-commit, after React has torn
  // down the subtrees removed from `mountedIds` (deleted children's cleanup
  // runs before the parent's passive effects for the same commit) — so no
  // Monaco pane or terminal slot is ever live over a destroyed store. An id
  // that was re-activated between eviction and flush is back in the retained
  // map (re-stamped by the later reconcile); its store must survive.
  useEffect(() => {
    if (pendingDestroyRef.current.length === 0) return
    const batch = pendingDestroyRef.current
    pendingDestroyRef.current = []
    for (const id of batch) {
      if (lastActiveRef.current!.has(id)) continue
      destroyWorkspaceStore(id)
    }
  }, [mountedIds])

  // Own the lifecycle of every workspace this host mounted: on unmount (the
  // route left the workspace surface entirely, e.g. to project home) destroy
  // the retained stores — plus any eviction still pending — so they don't
  // leak. Switching BETWEEN workspaces never unmounts the host — only the
  // activeWsId prop changes — so warm switches keep their stores.
  useEffect(
    () => () => {
      const doomed = new Set([...lastActiveRef.current!.keys(), ...pendingDestroyRef.current])
      lastActiveRef.current!.clear()
      pendingDestroyRef.current = []
      for (const id of doomed) {
        destroyWorkspaceStore(id)
      }
    },
    [],
  )

  // Always render the active workspace AND every pane-referenced one even
  // before the reconcile effect commits the mounted set for a brand-new id,
  // so there is never a blank frame — same guard as `activeWsId`'s own,
  // extended to the whole set a split can name at once. On the home route
  // activeWsId is null — nothing is force-appended for it, and every
  // retained slot renders hidden while home (rendered by the Outlet) is in
  // view; a pane can still be showing there (spec: panes are window-level),
  // so `paneWsIds` force-appends regardless of the route.
  const forced = activeWsId ? [activeWsId, ...paneWsIds] : paneWsIds
  const mountedIdSet = new Set(mountedIds)
  const missing = forced.filter((id) => !mountedIdSet.has(id))
  const renderIds = missing.length ? [...mountedIds, ...new Set(missing)] : mountedIds

  return (
    <>
      {renderIds.map((wsId) => {
        const isActive = wsId === activeWsId
        // The ONLY place the retention strategy touches the DOM: the active
        // workspace paints (display:contents), retained ones are hidden and inert
        // (display:none — dropped from the render tree, store still live).
        const { style, inert } = workspaceSlotStyling(isActive)
        return (
          <div
            key={wsId}
            data-workspace-slot={wsId}
            data-active={isActive || undefined}
            style={style}
            inert={inert}
          >
            <WorkspaceView wsId={wsId} active={isActive} />
          </div>
        )
      })}
      <WindowPaneSurface activeWsId={activeWsId} />
    </>
  )
}

/**
 * THE WINDOW'S PANE TREE — one of it, for the whole window.
 *
 * Panes became window-level in Task 26 (`windowPaneStore`: one pane map, one
 * layout, shared by every workspace), but the tree that RENDERS them stayed
 * inside `WorkspaceView` — so every retained workspace drew its own complete
 * copy of it, all but one of them inside a `display: none` slot. With four
 * workspaces warm and a three-pane split that is twelve `PaneContainer`s, twelve
 * chat surfaces and sixteen Plate editors live at once, eleven-twelfths of them
 * invisible, and each one a real subscriber to the shared pane store: marking a
 * chat active re-rendered all of them. It was also the visible flash. A chat's
 * workspace is the active one, so clicking between two chats switches slots,
 * and the incoming slot's copy had never been laid out — revealing it forced a
 * full layout and paint of every chat, editor and terminal in it, refired every
 * ResizeObserver inside (xterm refit, `usePaneViewPresentation` snapping from
 * "unmeasured" to its real arrangement) and cost 150-400ms on a click that
 * should cost nothing. Rendered once, a workspace switch moves no pane DOM at
 * all.
 *
 * It sits OUTSIDE the slots, as their sibling — layout-identical, because an
 * active slot is `display: contents` and so contributes no box of its own — and
 * carries the ACTIVE workspace's store as the ambient `WorkspaceStoreContext`,
 * which is exactly what the active slot used to give it. A pane holding another
 * workspace's chat re-provides its own store anyway (pane-container.tsx), so
 * the ambient one is only ever the documented fallback.
 */
function WindowPaneSurface({ activeWsId }: { activeWsId: string | null }) {
  // Never creates a store the host hasn't agreed to mount: `activeWsId` is
  // force-appended to `renderIds` above, so its own WorkspaceView creates the
  // very same store in the very same render pass.
  const store = activeWsId ? getOrCreateWorkspaceStore(activeWsId) : null
  if (!store) return null
  return (
    <WorkspaceStoreContext.Provider value={store}>
      <WorkspaceLayoutRoot />
    </WorkspaceStoreContext.Provider>
  )
}
