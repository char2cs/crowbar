import { createContext, useCallback, useContext, useState, type ReactNode } from 'react'
import { useSidebarStore } from '@/lib/store/sidebar'

interface CreatingState {
  repoId: string
  parentId: string
}

interface DraggingState {
  id: string
  repoId: string
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
  // Drag
  draggingWs: DraggingState | null
  startDragging: (id: string, repoId: string) => void
  endDragging: () => void
  dropOnWorkspace: (targetWsId: string, targetRepoId: string) => void
  dropOnRepo: (targetRepoId: string) => void
  dropOnTrash: () => void
}

const WorkspaceTreeContext = createContext<WorkspaceTreeContextValue | null>(null)

export function useWorkspaceTreeContext() {
  const ctx = useContext(WorkspaceTreeContext)
  if (!ctx) throw new Error('useWorkspaceTreeContext must be used inside WorkspaceTreeProvider')
  return ctx
}

export function WorkspaceTreeProvider({ children }: { children: ReactNode }) {
  const [creatingChildOf, setCreatingChildOf] = useState<CreatingState | null>(null)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [draggingWs, setDraggingWs] = useState<DraggingState | null>(null)

  const startCreating = useCallback((repoId: string, parentId: string) => {
    setCreatingChildOf({ repoId, parentId })
  }, [])

  const confirmCreate = useCallback((branch: string) => {
    if (!creatingChildOf || !branch.trim()) { setCreatingChildOf(null); return }
    const id = `ws-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
    useSidebarStore.getState().addWorkspace(
      creatingChildOf.repoId, id, branch.trim(), creatingChildOf.parentId,
    )
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

  const startDragging = useCallback((id: string, repoId: string) => {
    setDraggingWs({ id, repoId })
  }, [])

  const endDragging = useCallback(() => setDraggingWs(null), [])

  const dropOnWorkspace = useCallback((targetWsId: string, targetRepoId: string) => {
    if (!draggingWs || draggingWs.id === targetWsId) { setDraggingWs(null); return }
    if (draggingWs.repoId !== targetRepoId) { setDraggingWs(null); return }
    useSidebarStore.getState().reparentWorkspace(draggingWs.id, targetWsId)
    setDraggingWs(null)
  }, [draggingWs])

  const dropOnRepo = useCallback((targetRepoId: string) => {
    if (!draggingWs || draggingWs.repoId !== targetRepoId) { setDraggingWs(null); return }
    useSidebarStore.getState().reparentWorkspace(draggingWs.id, undefined)
    setDraggingWs(null)
  }, [draggingWs])

  const dropOnTrash = useCallback(() => {
    if (draggingWs) useSidebarStore.getState().deleteWorkspace(draggingWs.id)
    setDraggingWs(null)
  }, [draggingWs])

  return (
    <WorkspaceTreeContext.Provider value={{
      creatingChildOf, startCreating, confirmCreate, cancelCreate,
      renamingId, startRenaming, confirmRename, cancelRename,
      draggingWs, startDragging, endDragging, dropOnWorkspace, dropOnRepo, dropOnTrash,
    }}>
      {children}
    </WorkspaceTreeContext.Provider>
  )
}
