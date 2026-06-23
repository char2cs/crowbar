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
import { getPostDeleteNavigationTarget, useSidebarStore } from '@/lib/store/sidebar'
import { reparentWorkspace } from '@/lib/api/workspace'
import { postWorkspace, deleteWorkspace as apiDeleteWorkspace } from '@/lib/api'
import { toast } from '@/components/ui/toast'

/**
 * Resolve the owning project id for a repo from the sidebar tree. Hierarchical
 * mutations need both ids; the tree always carries `projectId` from the §5
 * RepoDTO once the repo has seeded.
 */
function projectIdForRepo(repoId: string): string | undefined {
  return useSidebarStore.getState().repos.find((r) => r.id === repoId)?.projectId
}

/**
 * Fire the hierarchical create mutation (202 Accepted, §3). No optimistic node
 * is added: the WorkspaceDTO arrives over the scoped WS stream (status 'new'
 * then the real status) and the WS-driven cache inserts it. On failure the
 * error is surfaced via toast.
 */
export async function performCreateWorkspace(
  repoId: string,
  branch: string,
  parentId?: string,
): Promise<void> {
  const projectId = projectIdForRepo(repoId)
  if (!projectId) {
    toast.error('Failed to create workspace', 'unknown project for repo')
    return
  }
  try {
    await postWorkspace(projectId, repoId, branch, parentId)
  } catch (err) {
    console.error('Failed to create workspace:', err)
    toast.error('Failed to create workspace', err instanceof Error ? err.message : undefined)
  }
}

/**
 * Fire the hierarchical delete mutation (202 Accepted, §3). Locked workspaces
 * are never deleted. No optimistic removal: the backend owns the cascade and
 * emits one status:'deleted' tombstone per removed id, which the WS-driven
 * cache applies. On failure the error is surfaced via toast.
 */
export async function performDeleteWorkspace(wsId: string): Promise<void> {
  const repo = useSidebarStore
    .getState()
    .repos.find((r) => r.workspaces.some((w) => w.id === wsId))
  const ws = repo?.workspaces.find((w) => w.id === wsId)
  if (!repo || !ws || ws.status === 'locked') return
  const projectId = repo.projectId
  if (!projectId) {
    toast.error('Failed to delete workspace', 'unknown project for repo')
    return
  }
  try {
    await apiDeleteWorkspace(projectId, repo.id, wsId)
  } catch (err) {
    console.error('Failed to delete workspace:', err)
    toast.error('Failed to delete workspace', err instanceof Error ? err.message : undefined)
  }
}

/**
 * Fire the hierarchical reparent mutation (202 Accepted, §3). The backend only
 * accepts a non-empty parent, so a move back to the repo root (undefined) is a
 * no-op for now — the new parentId arrives on the WS WorkspaceDTO and the
 * WS-driven cache reflects it. On failure the error is surfaced via toast.
 */
export async function performReparentWorkspace(
  wsId: string,
  newParentId: string | undefined,
  repoId: string,
): Promise<void> {
  if (newParentId === undefined) return
  const projectId = projectIdForRepo(repoId)
  if (!projectId) {
    toast.error('Failed to reparent workspace', 'unknown project for repo')
    return
  }
  try {
    await reparentWorkspace(projectId, repoId, wsId, newParentId)
    announceReparentOutcome(wsId, newParentId)
  } catch (err) {
    console.error('Failed to reparent workspace:', err)
    toast.error('Failed to reparent workspace', err instanceof Error ? err.message : undefined)
  }
}

/**
 * A reparent is 202-async; the outcome (clean vs conflicting) arrives over the WS
 * broadcast. Watch the moved workspace settle under its new parent — computed
 * fresh, since the parent may have drifted — and tell the user. This answers the
 * "is it clean now?" question on a move-back. One-shot, with a quiet timeout.
 */
function announceReparentOutcome(wsId: string, newParentId: string): void {
  let done = false
  const settle = (announce: () => void): void => {
    if (done) return
    done = true
    unsub()
    clearTimeout(timer)
    announce()
  }
  // Baseline the workspace's current error so we react only to a NEW one raised
  // by THIS reparent. The op is 202-async: a guard rejection (locked/non-leaf/
  // self/bad target) never throws on the request, it surfaces as a fresh
  // lastError on the workspace's broadcast — otherwise the failure is silent.
  let baselineError: string | undefined
  let baselineBranch: string | undefined
  for (const repo of useSidebarStore.getState().repos) {
    for (const w of repo.workspaces) {
      if (w.id === wsId) {
        baselineError = w.lastError
        baselineBranch = w.branch
      }
    }
  }
  const check = (): void => {
    let moved:
      | { branch?: string; parentId?: string; mergeConflicts?: boolean; lastError?: string }
      | undefined
    let parentBranch: string | undefined
    for (const repo of useSidebarStore.getState().repos) {
      for (const w of repo.workspaces) {
        if (w.id === wsId) moved = w
        if (w.id === newParentId) parentBranch = w.branch
      }
    }
    if (!moved) return
    // Failure: a fresh error appeared and the move did not land under the target.
    if (moved.lastError && moved.lastError !== baselineError && moved.parentId !== newParentId) {
      const name = moved.branch ?? baselineBranch ?? 'workspace'
      settle(() => toast.error(`Couldn’t move ${name}`, moved?.lastError))
      return
    }
    if (moved.parentId !== newParentId) return // not landed yet
    const where = parentBranch ?? 'its new parent'
    settle(() => {
      if (moved?.mergeConflicts) {
        toast.warning(`${moved.branch} conflicts with ${where} — resolve it from its panel`)
      } else {
        toast.success(`${moved?.branch} is clean under ${where}`)
      }
    })
  }
  const unsub = useSidebarStore.subscribe(check)
  const timer = setTimeout(() => settle(() => {}), 8000)
  check() // in case it already landed
}

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

export function WorkspaceTreeProvider({ children }: { children: ReactNode }) {
  const navigate = useNavigate()
  const router = useRouter()
  const [creatingChildOf, setCreatingChildOf] = useState<CreatingState | null>(null)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [draggingWs, setDraggingWs] = useState<DraggingState | null>(null)
  const [hoverTargetId, setHoverTargetId] = useState<string | null>(null)
  const [pendingCreates, setPendingCreates] = useState<Map<string, PendingCreate>>(new Map())

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
        const found = repo.workspaces.find(
          (w) => w.branch === branch && w.parentId === parentId,
        )
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
    [creatingChildOf], // eslint-disable-line react-hooks/exhaustive-deps
  )

  const cancelCreate = useCallback(() => setCreatingChildOf(null), [])
  const startRenaming = useCallback((wsId: string) => setRenamingId(wsId), [])

  const confirmRename = useCallback(
    (branch: string) => {
      if (renamingId && branch.trim()) {
        useSidebarStore.getState().renameWorkspace(renamingId, branch.trim())
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
        ghostRef.current.style.left = `${e.clientX + 12}px`
        ghostRef.current.style.top = `${e.clientY - 10}px`
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
            void performReparentWorkspace(ws.id, targetWsId, ws.repoId)
          }
        }
      } else if (target?.startsWith('repo:')) {
        const targetRepoId = target.slice(5)
        if (targetRepoId === ws.repoId) {
          void performReparentWorkspace(ws.id, undefined, ws.repoId)
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
      renamingId,
      startRenaming,
      confirmRename,
      cancelRename,
      onPointerDownDrag,
    ],
  )

  const dragValue = useMemo<WorkspaceTreeDragContextValue>(
    () => ({ draggingWs, hoverTargetId }),
    [draggingWs, hoverTargetId],
  )

  return (
    <>
      <WorkspaceTreeActionsContext.Provider value={actionsValue}>
        <WorkspaceTreeDragContext.Provider value={dragValue}>
          {children}
        </WorkspaceTreeDragContext.Provider>
      </WorkspaceTreeActionsContext.Provider>
      {draggingWs && (
        <div
          ref={ghostRef}
          className="pointer-events-none fixed z-50 rounded-md border border-border bg-secondary px-2 py-1 font-mono text-[13px] text-secondary-foreground shadow-md opacity-90"
          style={{
            left: lastDragPosRef.current ? lastDragPosRef.current.x + 12 : 0,
            top: lastDragPosRef.current ? lastDragPosRef.current.y - 10 : 0,
          }}
        >
          {draggingWs.label}
        </div>
      )}
    </>
  )
}
