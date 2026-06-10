import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useSidebarStore } from '@/lib/store/sidebar'
import { reparentWorkspace } from '@/lib/api/workspace'
import { postWorkspace, deleteWorkspace as apiDeleteWorkspace } from '@/lib/api'
import { toast } from '@/components/ui/toast'

/**
 * Creates the workspace on the backend, then mirrors it into the sidebar
 * store using the real id the backend returned. On failure no phantom node
 * is added — the error is surfaced via toast.
 */
export async function performCreateWorkspace(
  repoId: string, branch: string, parentId?: string,
): Promise<void> {
  try {
    const { id } = await postWorkspace(repoId, branch, parentId)
    useSidebarStore.getState().addWorkspace(repoId, id, branch, parentId)
  } catch (err) {
    console.error('Failed to create workspace:', err)
    toast.error('Failed to create workspace', err instanceof Error ? err.message : undefined)
  }
}

/**
 * Deletes the workspace on the backend, then removes it from the sidebar
 * store. Locked workspaces are never deleted. On failure the local store is
 * left untouched and the error is surfaced via toast.
 */
export async function performDeleteWorkspace(wsId: string): Promise<void> {
  const ws = useSidebarStore.getState().repos
    .flatMap(r => r.workspaces)
    .find(w => w.id === wsId)
  if (!ws || ws.status === 'locked') return
  try {
    await apiDeleteWorkspace(wsId)
    useSidebarStore.getState().deleteWorkspace(wsId)
  } catch (err) {
    console.error('Failed to delete workspace:', err)
    toast.error('Failed to delete workspace', err instanceof Error ? err.message : undefined)
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
  dragPos: { x: number; y: number } | null
  hoverTargetId: string | null
  onPointerDownDrag: (wsId: string, repoId: string, label: string, e: React.PointerEvent) => void
}

const WorkspaceTreeContext = createContext<WorkspaceTreeContextValue | null>(null)

export function useWorkspaceTreeContext() {
  const ctx = useContext(WorkspaceTreeContext)
  if (!ctx) throw new Error('useWorkspaceTreeContext must be used inside WorkspaceTreeProvider')
  return ctx
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
  const [creatingChildOf, setCreatingChildOf] = useState<CreatingState | null>(null)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [draggingWs, setDraggingWs] = useState<DraggingState | null>(null)
  const [dragPos, setDragPos] = useState<{ x: number; y: number } | null>(null)
  const [hoverTargetId, setHoverTargetId] = useState<string | null>(null)

  const pendingRef = useRef<{
    wsId: string; repoId: string; label: string
    startX: number; startY: number
    target: HTMLElement; pointerId: number
  } | null>(null)
  // Mirrors draggingWs for use inside window event handlers without stale closures.
  // Set synchronously at the point of change, not via useEffect, to avoid one-render lag.
  const draggingRef = useRef<DraggingState | null>(null)

  const startCreating = useCallback((repoId: string, parentId: string) => {
    setCreatingChildOf({ repoId, parentId })
  }, [])

  const confirmCreate = useCallback((branch: string) => {
    if (!creatingChildOf) return
    void performCreateWorkspace(creatingChildOf.repoId, branch, creatingChildOf.parentId)
    setCreatingChildOf(null)
  }, [creatingChildOf])

  const cancelCreate = useCallback(() => setCreatingChildOf(null), [])
  const startRenaming = useCallback((wsId: string) => setRenamingId(wsId), [])

  const confirmRename = useCallback((branch: string) => {
    if (renamingId && branch.trim()) {
      useSidebarStore.getState().renameWorkspace(renamingId, branch.trim())
    }
    setRenamingId(null)
  }, [renamingId])

  const cancelRename = useCallback(() => setRenamingId(null), [])

  const onPointerDownDrag = useCallback((wsId: string, repoId: string, label: string, e: React.PointerEvent) => {
    if (e.button !== 0) return
    if (draggingRef.current) return  // ignore second pointer mid-drag
    // Don't capture here — deferring setPointerCapture to the pointermove threshold
    // prevents it from swallowing the dblclick event used for rename.
    pendingRef.current = {
      wsId, repoId, label, startX: e.clientX, startY: e.clientY,
      target: e.currentTarget as HTMLElement, pointerId: e.pointerId,
    }
  }, [])

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
          setDraggingWs(ws)
          setDragPos({ x: e.clientX, y: e.clientY })
          // Update hover target immediately so highlight appears on the first move
          setHoverTargetId(findDropTarget(e.clientX, e.clientY, wsId))
        }
        return
      }
      if (!draggingRef.current) return
      setDragPos({ x: e.clientX, y: e.clientY })
      setHoverTargetId(findDropTarget(e.clientX, e.clientY, draggingRef.current.id))
    }

    function onPointerUp(e: PointerEvent) {
      pendingRef.current = null
      const ws = draggingRef.current
      if (!ws) return

      const target = findDropTarget(e.clientX, e.clientY, ws.id)

      if (target === 'trash') {
        void performDeleteWorkspace(ws.id)
      } else if (target?.startsWith('ws:')) {
        const targetWsId = target.slice(3)
        if (targetWsId !== ws.id) {
          const repos = useSidebarStore.getState().repos
          const targetRepo = repos.find(r => r.workspaces.some(w => w.id === targetWsId))
          if (targetRepo?.id === ws.repoId) {
            void reparentWorkspace(ws.id, targetWsId, ws.repoId)
          }
        }
      } else if (target?.startsWith('repo:')) {
        const targetRepoId = target.slice(5)
        if (targetRepoId === ws.repoId) {
          void reparentWorkspace(ws.id, undefined, ws.repoId)
        }
      }

      draggingRef.current = null
      setDraggingWs(null)
      setDragPos(null)
      setHoverTargetId(null)
    }

    function onPointerCancel() {
      pendingRef.current = null
      if (draggingRef.current) {
        draggingRef.current = null
        setDraggingWs(null)
        setDragPos(null)
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
  }, [])

  return (
    <WorkspaceTreeContext.Provider value={{
      creatingChildOf, startCreating, confirmCreate, cancelCreate,
      renamingId, startRenaming, confirmRename, cancelRename,
      draggingWs, dragPos, hoverTargetId, onPointerDownDrag,
    }}>
      {children}
    </WorkspaceTreeContext.Provider>
  )
}
