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

const EMPTY_WS_IDS: string[] = []

const ID_DELIM = '\u0000'

export function WorkspaceHost({
  activeWsId,
  homeWsIds = EMPTY_WS_IDS,
  paneWsIds = EMPTY_WS_IDS,
  viewWsIds = EMPTY_WS_IDS,
}: {
  activeWsId: string | null
  paneWsIds?: string[]
  homeWsIds?: string[]
  viewWsIds?: string[]
}) {
  const existingIdsKey = useSelect((s) =>
    selectRepos(s)
      .flatMap((r) => [
        ...(r.defaultWorkspaceId ? [r.defaultWorkspaceId] : []),
        ...r.workspaces.map((w) => w.id),
      ])
      .sort()
      .join(ID_DELIM),
  )
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

  const [mountedIds, setMountedIds] = useState<string[]>(EMPTY_WS_IDS)
  const retainedRef = useRef<Set<string> | null>(null)
  if (retainedRef.current === null) retainedRef.current = new Set()
  const pendingDestroyRef = useRef<string[]>([])

  const activeWsIdRef = useRef(activeWsId)
  activeWsIdRef.current = activeWsId
  const existingIdsRef = useRef(existingIds)
  existingIdsRef.current = existingIds
  const paneWsIdsRef = useRef(paneWsIds)
  paneWsIdsRef.current = paneWsIds
  const viewWsIdsRef = useRef(viewWsIdsSet)
  viewWsIdsRef.current = viewWsIdsSet

  const reconcileRef = useRef<() => void>(() => {})
  reconcileRef.current = () => {
    const map = retainedRef.current!
    const active = activeWsIdRef.current

    if (active) map.add(active)
    for (const id of paneWsIdsRef.current) {
      if (id !== active) map.add(id)
    }

    const existing = existingIdsRef.current
    if (existing.size > 0) {
      for (const id of [...map]) {
        if (id !== active && !existing.has(id)) {
          map.delete(id)
          pendingDestroyRef.current.push(id)
        }
      }
    }

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

    setActiveWorkspaceId(active)
    for (const id of plan.retain) getOrCreateWorkspaceStore(id)

    setMountedIds(plan.retain)
  }

  const spanPrevActiveRef = useRef<string | null>(activeWsId)
  useLayoutEffect(() => {
    const prev = spanPrevActiveRef.current
    spanPrevActiveRef.current = activeWsId
    if (!activeWsId || activeWsId === prev) return
    const warm = retainedRef.current!.has(activeWsId)
    if (!warm) return
    markStart('workspace.switch')
    const raf = requestAnimationFrame(() => markEnd('workspace.switch'))
    return () => cancelAnimationFrame(raf)
  }, [activeWsId])

  useLayoutEffect(() => {
    reconcileRef.current()
  }, [activeWsId, existingIds, paneWsIdsKey, viewWsIdsKey])

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

  useEffect(() => {
    if (pendingDestroyRef.current.length === 0) return
    const batch = pendingDestroyRef.current
    pendingDestroyRef.current = []
    for (const id of batch) {
      if (retainedRef.current!.has(id)) continue
      destroyWorkspaceStore(id)
    }
  }, [mountedIds])

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

function WindowPaneSurface({ mountedIds }: { mountedIds: readonly string[] }) {
  const ambientWsId = mountedIds[0]
  const store = ambientWsId ? getWorkspaceStore(ambientWsId) : undefined
  if (!store) return null
  return (
    <WorkspaceStoreContext.Provider value={store}>
      <WorkspaceLayoutRoot />
    </WorkspaceStoreContext.Provider>
  )
}
