import {
  createContext,
  useCallback,
  useContext,
  useEffect,
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
  } catch (err) {
    console.error('Failed to reparent workspace:', err)
    toast.error('Failed to reparent workspace', err instanceof Error ? err.message : undefined)
  }
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

interface WorkspaceTreeContextValue {
  // Create
  creatingChildOf: CreatingState | null
  startCreating: (repoId: string, parentId: string) => void
  confirmCreate: (branch: string) => void
  cancelCreate: () => void
  // Rename
  renamingId: string | null
  startRenaming: (wsId: string) => void
  confirmRename: (branch: string) => void
  cancelRename: () => void
  // Drag (pointer-based)
  draggingWs: DraggingState | null
  hoverTargetId: string | null
  onPointerDownDrag: (wsId: string, repoId: string, label: string, e: React.PointerEvent) => void
}

const WorkspaceTreeContext = createContext<WorkspaceTreeContextValue | null>(null)

export function useWorkspaceTreeContext() {
  const ctx = useContext(WorkspaceTreeContext)
  if (!ctx) throw new Error('useWorkspaceTreeContext must be used inside WorkspaceTreeProvider')
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
      void performCreateWorkspace(creatingChildOf.repoId, branch, creatingChildOf.parentId)
      setCreatingChildOf(null)
    },
    [creatingChildOf],
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

  return (
    <>
      <WorkspaceTreeContext.Provider
        value={{
          creatingChildOf,
          startCreating,
          confirmCreate,
          cancelCreate,
          renamingId,
          startRenaming,
          confirmRename,
          cancelRename,
          draggingWs,
          hoverTargetId,
          onPointerDownDrag,
        }}
      >
        {children}
      </WorkspaceTreeContext.Provider>
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
