import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { useNavigate, useRouter } from '@tanstack/react-router'
import { DragGhost, DRAG_GHOST_OFFSET_X, DRAG_GHOST_OFFSET_Y } from './drag-ghost'
import { getPostDeleteNavigationTarget, useSidebarStore } from '@/lib/store/sidebar'
import { reparentWorkspace } from '@/lib/api/workspace'
import { postWorkspace } from '@/lib/api'
import {
  projectIdForRepo,
  performDeleteWorkspace,
  performRenameWorkspaceBranch,
  performImportBranches,
} from './workspace-tree-actions'

interface CreatingState {
  repoId: string
  parentId: string
}

interface DraggingState {
  id: string
  repoId: string
  label: string
}

export interface PendingCreate {
  repoId: string
  parentId: string
  branch: string
  error?: string
}

/**
 * The slow-changing slice: action callbacks (stable identities) plus the
 * create/rename UI state, which only flips on explicit user intent. Split out
 * from the drag slice so that a drag (which fires `setHoverTargetId` on every
 * drop-boundary crossing) does not recreate this value and re-render every row
 * that only needs the actions.
 */
interface WorkspaceTreeActionsContextValue {
  // Create
  creatingChildOf: CreatingState | null
  startCreating: (repoId: string, parentId: string) => void
  confirmCreate: (branch: string) => void
  cancelCreate: () => void
  // Pending creates (in-flight / errored)
  pendingCreates: Map<string, PendingCreate>
  clearPendingCreate: (tempId: string) => void
  // Import: optimistically add a spinner row per branch, then fire the batch import
  startImport: (repoId: string, branches: string[]) => void
  // Rename
  renamingId: string | null
  startRenaming: (wsId: string) => void
  confirmRename: (branch: string) => void
  cancelRename: () => void
  // Drag start (pointer-based) — stable callback, lives with the actions
  onPointerDownDrag: (wsId: string, repoId: string, label: string, e: React.PointerEvent) => void
}

/**
 * The fast-changing slice: the live drag state. `hoverTargetId` updates on every
 * boundary crossing during a drag; keeping it in its own context means only the
 * subscribers that actually read drag state re-render on those updates.
 */
interface WorkspaceTreeDragContextValue {
  draggingWs: DraggingState | null
  hoverTargetId: string | null
  movingWsId: string | null // wsId of item currently being moved (API in-flight)
}

const WorkspaceTreeActionsContext = createContext<WorkspaceTreeActionsContextValue | null>(null)
const WorkspaceTreeDragContext = createContext<WorkspaceTreeDragContextValue | null>(null)

export function useWorkspaceTreeActions() {
  const ctx = useContext(WorkspaceTreeActionsContext)
  if (!ctx) throw new Error('useWorkspaceTreeActions must be used inside WorkspaceTreeProvider')
  return ctx
}

export function useWorkspaceTreeDrag() {
  const ctx = useContext(WorkspaceTreeDragContext)
  if (!ctx) throw new Error('useWorkspaceTreeDrag must be used inside WorkspaceTreeProvider')
  return ctx
}

// A completed drag still produces a click on the captured row (pointerdown +
// pointerup on the same element), which would select/navigate into the
// dragged workspace. Swallow that one click in the capture phase.
function suppressNextClick(): void {
  const swallow = (e: MouseEvent) => {
    e.stopPropagation()
    e.preventDefault()
  }
  window.addEventListener('click', swallow, { capture: true, once: true })
  // The click (if any) fires synchronously after pointerup; drop the trap
  // right after so a later real click is never swallowed.
  setTimeout(() => window.removeEventListener('click', swallow, { capture: true }), 0)
}

// Skips the dragging workspace itself to prevent self-drop flicker
function findDropTarget(x: number, y: number, draggingId: string | null): string | null {
  const els = document.elementsFromPoint(x, y)
  for (const el of els) {
    if (!(el instanceof Element)) continue
    if (el.getAttribute('data-trash-drop') !== null) return 'trash'
    const wsId = el.getAttribute('data-ws-drop')
    if (wsId !== null && wsId !== draggingId) return `ws:${wsId}`
    const repoId = el.getAttribute('data-repo-drop')
    if (repoId !== null) return `repo:${repoId}`
  }
  return null
}

// react-doctor-disable-next-line no-giant-component -- accepted: cohesive context provider — create/rename/delete/drag flows share the same refs, drag ghost and sidebar-store coordination; the size is the provider's wiring, not independent sections.
export function WorkspaceTreeProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate()
  const router = useRouter()
  const [creatingChildOf, setCreatingChildOf] = useState<CreatingState | null>(null)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [draggingWs, setDraggingWs] = useState<DraggingState | null>(null)
  const [hoverTargetId, setHoverTargetId] = useState<string | null>(null)
  const [pendingCreates, setPendingCreates] = useState<Map<string, PendingCreate>>(new Map())
  const [movingWsId, setMovingWsId] = useState<string | null>(null)

  function addPendingCreate(tempId: string, entry: PendingCreate) {
    setPendingCreates((prev) => new Map(prev).set(tempId, entry))
  }
  function setPendingCreateError(tempId: string, error: string) {
    setPendingCreates((prev) => {
      const entry = prev.get(tempId)
      if (!entry) return prev
      return new Map(prev).set(tempId, { ...entry, error })
    })
  }
  function clearPendingCreate(tempId: string) {
    setPendingCreates((prev) => {
      const n = new Map(prev)
      n.delete(tempId)
      return n
    })
  }

  // Ghost div position is updated imperatively in pointermove — no React state,
  // no tree re-renders on every pixel of mouse movement.
  const ghostRef = useRef<HTMLDivElement | null>(null)
  // Tracks the initial position for the ghost's first render (set just before
  // setDraggingWs so the ghost mounts at the correct location).
  const lastDragPosRef = useRef<{ x: number; y: number } | null>(null)
  // Mirrors hoverTargetId for equality checks — avoids state writes when the
  // drop target hasn't actually changed.
  const hoverTargetIdRef = useRef<string | null>(null)

  const pendingRef = useRef<{
    wsId: string
    repoId: string
    label: string
    startX: number
    startY: number
    target: HTMLElement
    pointerId: number
  } | null>(null)
  // Mirrors draggingWs for use inside window event handlers without stale closures.
  // Set synchronously at the point of change, not via useEffect, to avoid one-render lag.
  const draggingRef = useRef<DraggingState | null>(null)

  // Ref so the onPointerUp closure can call setMovingWsId without stale captures.
  const setMovingWsIdRef = useRef(setMovingWsId)
  setMovingWsIdRef.current = setMovingWsId

  const startCreating = useCallback((repoId: string, parentId: string) => {
    setCreatingChildOf({ repoId, parentId })
  }, [])

  const confirmCreate = useCallback(
    (branch: string) => {
      if (!creatingChildOf) return
      const { repoId, parentId } = creatingChildOf
      const tempId = crypto.randomUUID()
      setCreatingChildOf(null) // hide input immediately

      addPendingCreate(tempId, { repoId, parentId, branch })

      // Subscribe to sidebar store: when the real workspace arrives, remove pending
      const unsub = useSidebarStore.subscribe((state) => {
        const repo = state.repos.find((r) => r.id === repoId)
        if (!repo) return
        const found = repo.workspaces.find((w) => w.branch === branch && w.parentId === parentId)
        if (found) {
          clearPendingCreate(tempId)
          unsub()
        }
      })

      // Fire the API
      const projectId = projectIdForRepo(repoId)
      if (!projectId) {
        setPendingCreateError(tempId, 'Unknown project')
        unsub()
        return
      }
      postWorkspace(projectId, repoId, branch, parentId).catch((err) => {
        unsub()
        setPendingCreateError(tempId, err instanceof Error ? err.message : 'Create failed')
      })
    },
    [creatingChildOf],
  )

  const cancelCreate = useCallback(() => setCreatingChildOf(null), [])

  // Batch import: optimistic spinner rows + the 202 import, orchestrated by the
  // testable helper. The row setters are stable functional-setState wrappers.
  const startImport = useCallback((repoId: string, branches: string[]) => {
    performImportBranches(repoId, branches, {
      addPending: addPendingCreate,
      setError: setPendingCreateError,
      clearPending: clearPendingCreate,
    })
  }, [])

  const startRenaming = useCallback((wsId: string) => setRenamingId(wsId), [])

  const confirmRename = useCallback(
    (branch: string) => {
      if (renamingId && branch.trim()) {
        // The renamed WorkspaceDTO arrives on the workspaces WS stream and
        // updates the tree — no optimistic write here, which is what made the
        // rename appear to work while the branch never actually changed.
        void performRenameWorkspaceBranch(renamingId, branch.trim())
      }
      setRenamingId(null)
    },
    [renamingId],
  )

  const cancelRename = useCallback(() => setRenamingId(null), [])

  const onPointerDownDrag = useCallback(
    (wsId: string, repoId: string, label: string, e: React.PointerEvent) => {
      if (e.button !== 0) return
      if (draggingRef.current) return // ignore second pointer mid-drag
      // Don't capture here — deferring setPointerCapture to the pointermove threshold
      // prevents it from swallowing the dblclick event used for rename.
      pendingRef.current = {
        wsId,
        repoId,
        label,
        startX: e.clientX,
        startY: e.clientY,
        target: e.currentTarget as HTMLElement,
        pointerId: e.pointerId,
      }
    },
    [],
  )

  useEffect(() => {
    function onPointerMove(e: PointerEvent) {
      if (pendingRef.current) {
        const { wsId, repoId, label, startX, startY, target, pointerId } = pendingRef.current
        if (Math.hypot(e.clientX - startX, e.clientY - startY) > 5) {
          pendingRef.current = null
          // Capture only now that drag is confirmed — keeps dblclick working.
          if (target.isConnected) target.setPointerCapture(pointerId)
          const ws = { id: wsId, repoId, label }
          draggingRef.current = ws
          // Store position before triggering the React re-render so the ghost
          // div mounts at the correct location on its first render.
          lastDragPosRef.current = { x: e.clientX, y: e.clientY }
          setDraggingWs(ws)
          // Update hover target immediately so highlight appears on the first move
          const initialTarget = findDropTarget(e.clientX, e.clientY, wsId)
          hoverTargetIdRef.current = initialTarget
          setHoverTargetId(initialTarget)
        }
        return
      }
      if (!draggingRef.current) return
      // Move ghost directly — no React state update, no tree re-render.
      if (ghostRef.current) {
        ghostRef.current.style.left = `${e.clientX + DRAG_GHOST_OFFSET_X}px`
        ghostRef.current.style.top = `${e.clientY + DRAG_GHOST_OFFSET_Y}px`
      }
      // Only trigger a React re-render when the drop target actually changes
      // (mouse crosses a boundary), not on every pixel of movement.
      const newTarget = findDropTarget(e.clientX, e.clientY, draggingRef.current.id)
      if (newTarget !== hoverTargetIdRef.current) {
        hoverTargetIdRef.current = newTarget
        setHoverTargetId(newTarget)
      }
    }

    function onPointerUp(e: PointerEvent) {
      pendingRef.current = null
      const ws = draggingRef.current
      if (!ws) return

      // The browser fires a click for this pointerup on the captured row;
      // a drop must never double as a row click (selection/navigation).
      suppressNextClick()

      const target = findDropTarget(e.clientX, e.clientY, ws.id)

      if (target === 'trash') {
        // Resolve the fallback before deletion mutates the store.
        const fallbackWsId = getPostDeleteNavigationTarget(useSidebarStore.getState().repos, ws.id)
        void performDeleteWorkspace(ws.id).then(() => {
          // If the active workspace no longer exists (it was the dragged one
          // or a deleted descendant), leave the dead route: go to the parent
          // / repo base workspace, or the projects page as last resort.
          const pathname = router.state.location.pathname
          const activeId = pathname.match(/\/ide\/[^/]+\/[^/]+\/([^/]+)/)?.[1]
          if (!activeId) return
          const stillExists = useSidebarStore
            .getState()
            .repos.some((r) => r.workspaces.some((w) => w.id === activeId))
          if (stillExists) return
          if (fallbackWsId) {
            const updatedRepos = useSidebarStore.getState().repos
            const fallbackRepo = updatedRepos.find((r) =>
              r.workspaces.some((w) => w.id === fallbackWsId),
            )
            if (fallbackRepo) {
              void navigate({
                to: '/ide/$projectId/$repoId/$wsId',
                params: {
                  projectId: fallbackRepo.projectId ?? '',
                  repoId: fallbackRepo.id,
                  wsId: fallbackWsId,
                },
              })
            } else {
              void navigate({ to: '/' })
            }
          } else {
            void navigate({ to: '/' })
          }
        })
      } else if (target?.startsWith('ws:')) {
        const targetWsId = target.slice(3)
        if (targetWsId !== ws.id) {
          const repos = useSidebarStore.getState().repos
          const targetRepo = repos.find((r) => r.workspaces.some((w) => w.id === targetWsId))
          if (targetRepo?.id === ws.repoId) {
            // Capture original parent before optimistic move
            const originalParentId = repos
              .flatMap((r) => r.workspaces)
              .find((w) => w.id === ws.id)?.parentId

            // Optimistic: move immediately in store
            useSidebarStore.getState().reparentWorkspace(ws.id, targetWsId)
            setMovingWsIdRef.current(ws.id)

            const projectId = projectIdForRepo(ws.repoId)
            if (projectId) {
              reparentWorkspace(projectId, ws.repoId, ws.id, targetWsId)
                .catch(() => {
                  // Snap back on failure
                  useSidebarStore.getState().reparentWorkspace(ws.id, originalParentId)
                })
                .finally(() => {
                  setMovingWsIdRef.current(null)
                })
            } else {
              // Can't move — revert immediately
              useSidebarStore.getState().reparentWorkspace(ws.id, originalParentId)
              setMovingWsIdRef.current(null)
            }
          }
        }
      }

      draggingRef.current = null
      hoverTargetIdRef.current = null
      lastDragPosRef.current = null
      setDraggingWs(null)
      setHoverTargetId(null)
    }

    function onPointerCancel() {
      pendingRef.current = null
      if (draggingRef.current) {
        draggingRef.current = null
        hoverTargetIdRef.current = null
        lastDragPosRef.current = null
        setDraggingWs(null)
        setHoverTargetId(null)
      }
    }

    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', onPointerCancel)
    return () => {
      window.removeEventListener('pointermove', onPointerMove)
      window.removeEventListener('pointerup', onPointerUp)
      window.removeEventListener('pointercancel', onPointerCancel)
    }
  }, [navigate, router])

  // Memoize each slice so the Provider hands out stable values: the actions
  // value only changes when the create/rename state does (its callbacks are
  // stable), and the drag value only changes on a real drag update.
  const actionsValue = useMemo<WorkspaceTreeActionsContextValue>(
    () => ({
      creatingChildOf,
      startCreating,
      confirmCreate,
      cancelCreate,
      pendingCreates,
      clearPendingCreate,
      startImport,
      renamingId,
      startRenaming,
      confirmRename,
      cancelRename,
      onPointerDownDrag,
    }),
    [
      creatingChildOf,
      startCreating,
      confirmCreate,
      cancelCreate,
      pendingCreates,
      startImport,
      renamingId,
      startRenaming,
      confirmRename,
      cancelRename,
      onPointerDownDrag,
    ],
  )

  const dragValue = useMemo<WorkspaceTreeDragContextValue>(
    () => ({ draggingWs, hoverTargetId, movingWsId }),
    [draggingWs, hoverTargetId, movingWsId],
  )

  return (
    <>
      <WorkspaceTreeActionsContext.Provider value={actionsValue}>
        <WorkspaceTreeDragContext.Provider value={dragValue}>
          {children}
        </WorkspaceTreeDragContext.Provider>
      </WorkspaceTreeActionsContext.Provider>
      {draggingWs && (
        <DragGhost
          ref={ghostRef}
          label={draggingWs.label}
          origin={lastDragPosRef.current}
          className="font-mono"
        />
      )}
    </>
  )
}
