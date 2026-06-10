import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { useSidebarStore } from '@/lib/store/sidebar'
import {
  postChat,
  forkChat as apiForkChat,
  patchChat,
  deleteChat as apiDeleteChat,
  chatDtoToProjectChat,
} from '@/lib/api/chat'
import { toast } from '@/components/ui/toast'

/**
 * Creates the chat on the backend, then mirrors it into the sidebar store
 * using the real id the backend returned. On failure no phantom node is
 * added — the error is surfaced via toast.
 */
export async function performCreateChat(wsId: string, title: string): Promise<void> {
  if (!wsId || !title.trim()) return
  try {
    const chat = await postChat(wsId, title.trim())
    useSidebarStore.getState().addChat(chatDtoToProjectChat(chat))
  } catch (err) {
    console.error('Failed to create chat:', err)
    toast.error('Failed to create chat', err instanceof Error ? err.message : undefined)
  }
}

/**
 * Forks the parent chat on the backend, then mirrors the new node into the
 * sidebar store. The fork endpoint copies the parent title, so when the user
 * typed a different title the fork is renamed right after. On fork failure
 * no phantom node is added.
 */
export async function performForkChat(parentId: string, title: string): Promise<void> {
  let chat
  try {
    chat = await apiForkChat(parentId)
  } catch (err) {
    console.error('Failed to fork chat:', err)
    toast.error('Failed to fork chat', err instanceof Error ? err.message : undefined)
    return
  }
  const trimmed = title.trim()
  if (trimmed && trimmed !== chat.title) {
    try {
      chat = await patchChat(chat.id, trimmed)
    } catch (err) {
      // The fork itself succeeded — keep it (with the parent title) and
      // surface the rename failure.
      console.error('Failed to rename forked chat:', err)
      toast.error('Failed to rename forked chat', err instanceof Error ? err.message : undefined)
    }
  }
  useSidebarStore.getState().addChat(chatDtoToProjectChat(chat))
}

/**
 * Renames the chat on the backend, then updates the sidebar store. On
 * failure the local store is left untouched.
 */
export async function performRenameChat(chatId: string, title: string): Promise<void> {
  if (!title.trim()) return
  try {
    await patchChat(chatId, title.trim())
    useSidebarStore.getState().renameChat(chatId, title.trim())
  } catch (err) {
    console.error('Failed to rename chat:', err)
    toast.error('Failed to rename chat', err instanceof Error ? err.message : undefined)
  }
}

/**
 * Deletes the chat on the backend, then removes it from the sidebar store.
 * On failure the local store is left untouched.
 */
export async function performDeleteChat(chatId: string): Promise<void> {
  const chat = useSidebarStore.getState().chats.find((c) => c.id === chatId)
  if (!chat) return
  try {
    await apiDeleteChat(chatId)
    useSidebarStore.getState().deleteChat(chatId)
  } catch (err) {
    console.error('Failed to delete chat:', err)
    toast.error('Failed to delete chat', err instanceof Error ? err.message : undefined)
  }
}

interface CreatingState {
  parentId: string
}

interface DraggingState {
  id: string
  label: string
}

interface ChatTreeContextValue {
  creatingChildOf: CreatingState | null
  startCreating: (parentId: string) => void
  confirmCreate: (title: string) => void
  cancelCreate: () => void
  renamingId: string | null
  startRenaming: (chatId: string) => void
  confirmRename: (title: string) => void
  cancelRename: () => void
  draggingChat: DraggingState | null
  dragPos: { x: number; y: number } | null
  hoverTrash: boolean
  onPointerDownDrag: (chatId: string, label: string, e: React.PointerEvent) => void
}

const ChatTreeContext = createContext<ChatTreeContextValue | null>(null)

export function useChatTreeContext() {
  const ctx = useContext(ChatTreeContext)
  if (!ctx) throw new Error('useChatTreeContext must be used inside ChatTreeProvider')
  return ctx
}

function isOverTrash(x: number, y: number): boolean {
  const els = document.elementsFromPoint(x, y)
  return els.some((el) => el instanceof Element && el.getAttribute('data-trash-drop') !== null)
}

export function ChatTreeProvider({ children }: { children: ReactNode }) {
  const [creatingChildOf, setCreatingChildOf] = useState<CreatingState | null>(null)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [draggingChat, setDraggingChat] = useState<DraggingState | null>(null)
  const [dragPos, setDragPos] = useState<{ x: number; y: number } | null>(null)
  const [hoverTrash, setHoverTrash] = useState(false)

  const pendingRef = useRef<{
    chatId: string
    label: string
    startX: number
    startY: number
    target: HTMLElement
    pointerId: number
  } | null>(null)
  const draggingRef = useRef<DraggingState | null>(null)

  const startCreating = useCallback((parentId: string) => {
    setCreatingChildOf({ parentId })
  }, [])

  const confirmCreate = useCallback(
    (title: string) => {
      if (!creatingChildOf || !title.trim()) return
      void performForkChat(creatingChildOf.parentId, title)
      setCreatingChildOf(null)
    },
    [creatingChildOf],
  )

  const cancelCreate = useCallback(() => setCreatingChildOf(null), [])
  const startRenaming = useCallback((chatId: string) => setRenamingId(chatId), [])

  const confirmRename = useCallback(
    (title: string) => {
      if (renamingId && title.trim()) {
        void performRenameChat(renamingId, title)
      }
      setRenamingId(null)
    },
    [renamingId],
  )

  const cancelRename = useCallback(() => setRenamingId(null), [])

  const onPointerDownDrag = useCallback((chatId: string, label: string, e: React.PointerEvent) => {
    if (e.button !== 0) return
    if (draggingRef.current) return
    pendingRef.current = {
      chatId,
      label,
      startX: e.clientX,
      startY: e.clientY,
      target: e.currentTarget as HTMLElement,
      pointerId: e.pointerId,
    }
  }, [])

  useEffect(() => {
    function onPointerMove(e: PointerEvent) {
      if (pendingRef.current) {
        const { chatId, label, startX, startY, target, pointerId } = pendingRef.current
        if (Math.hypot(e.clientX - startX, e.clientY - startY) > 5) {
          pendingRef.current = null
          if (target.isConnected) target.setPointerCapture(pointerId)
          const chat = { id: chatId, label }
          draggingRef.current = chat
          setDraggingChat(chat)
          setDragPos({ x: e.clientX, y: e.clientY })
          setHoverTrash(isOverTrash(e.clientX, e.clientY))
        }
        return
      }
      if (!draggingRef.current) return
      setDragPos({ x: e.clientX, y: e.clientY })
      setHoverTrash(isOverTrash(e.clientX, e.clientY))
    }

    function onPointerUp(e: PointerEvent) {
      pendingRef.current = null
      const chat = draggingRef.current
      if (!chat) return
      if (isOverTrash(e.clientX, e.clientY)) {
        void performDeleteChat(chat.id)
      }
      draggingRef.current = null
      setDraggingChat(null)
      setDragPos(null)
      setHoverTrash(false)
    }

    function onPointerCancel() {
      pendingRef.current = null
      if (draggingRef.current) {
        draggingRef.current = null
        setDraggingChat(null)
        setDragPos(null)
        setHoverTrash(false)
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
    <ChatTreeContext.Provider
      value={{
        creatingChildOf,
        startCreating,
        confirmCreate,
        cancelCreate,
        renamingId,
        startRenaming,
        confirmRename,
        cancelRename,
        draggingChat,
        dragPos,
        hoverTrash,
        onPointerDownDrag,
      }}
    >
      {children}
    </ChatTreeContext.Provider>
  )
}
