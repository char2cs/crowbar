import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react'
import { useSidebarStore, type ProjectChat } from '@/lib/store/sidebar'
import {
  postChat,
  forkChat as apiForkChat,
  patchChat,
  deleteChat as apiDeleteChat,
  chatDtoToProjectChat,
} from '@/lib/api/chat'

type SetChatError = (id: string, msg: string) => void

/**
 * Creates the chat on the backend, then mirrors it into the sidebar store
 * using the real id the backend returned. On failure no phantom node is
 * added — the error is surfaced inline via setChatError.
 */
export async function performCreateChat(
  wsId: string,
  title: string,
  setChatError?: SetChatError,
): Promise<ProjectChat | null> {
  if (!wsId || !title.trim()) return null
  try {
    const chat = await postChat(wsId, title.trim())
    const projectChat = chatDtoToProjectChat(chat)
    useSidebarStore.getState().addChat(projectChat)
    return projectChat
  } catch (err) {
    console.error('Failed to create chat:', err)
    setChatError?.('create', err instanceof Error ? err.message : 'Failed to create chat')
    return null
  }
}

/**
 * Forks the parent chat on the backend, then mirrors the new node into the
 * sidebar store. The fork endpoint copies the parent title, so when the user
 * typed a different title the fork is renamed right after. On fork failure
 * no phantom node is added.
 */
export async function performForkChat(
  parentId: string,
  title: string,
  setChatError?: SetChatError,
): Promise<void> {
  let chat
  try {
    chat = await apiForkChat(parentId)
  } catch (err) {
    console.error('Failed to fork chat:', err)
    setChatError?.(parentId, err instanceof Error ? err.message : 'Failed to fork chat')
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
      setChatError?.(parentId, err instanceof Error ? err.message : 'Failed to rename forked chat')
    }
  }
  useSidebarStore.getState().addChat(chatDtoToProjectChat(chat))
}

/**
 * Renames the chat on the backend, then updates the sidebar store. On
 * failure the local store is left untouched.
 */
export async function performRenameChat(
  chatId: string,
  title: string,
  setChatError?: SetChatError,
): Promise<void> {
  if (!title.trim()) return
  try {
    await patchChat(chatId, title.trim())
    useSidebarStore.getState().renameChat(chatId, title.trim())
  } catch (err) {
    console.error('Failed to rename chat:', err)
    setChatError?.(chatId, err instanceof Error ? err.message : 'Failed to rename chat')
  }
}

/**
 * Deletes the chat on the backend, then removes it from the sidebar store.
 * On failure the local store is left untouched.
 */
export async function performDeleteChat(chatId: string, setChatError?: SetChatError): Promise<void> {
  const chat = useSidebarStore.getState().chats.find((c) => c.id === chatId)
  if (!chat) return
  try {
    await apiDeleteChat(chatId)
    useSidebarStore.getState().deleteChat(chatId)
  } catch (err) {
    console.error('Failed to delete chat:', err)
    setChatError?.(chatId, err instanceof Error ? err.message : 'Failed to delete chat')
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
  hoverTrash: boolean
  onPointerDownDrag: (chatId: string, label: string, e: React.PointerEvent) => void
  chatErrors: Map<string, string>
  setChatError: (id: string, msg: string) => void
  clearChatError: (id: string) => void
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
  const [hoverTrash, setHoverTrash] = useState(false)
  const [chatErrors, setChatErrors] = useState<Map<string, string>>(new Map())

  const setChatError = useCallback((id: string, msg: string) => {
    setChatErrors((prev) => new Map(prev).set(id, msg))
  }, [])

  const clearChatError = useCallback((id: string) => {
    setChatErrors((prev) => {
      const n = new Map(prev)
      n.delete(id)
      return n
    })
  }, [])

  // Ghost div position is updated imperatively in pointermove — no React state,
  // no tree re-renders on every pixel of mouse movement.
  const ghostRef = useRef<HTMLDivElement | null>(null)
  // Tracks the initial position for the ghost's first render.
  const lastDragPosRef = useRef<{ x: number; y: number } | null>(null)
  // Equality refs — avoids state writes when values haven't changed.
  const hoverTrashRef = useRef(false)

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
      void performForkChat(creatingChildOf.parentId, title, setChatError)
      setCreatingChildOf(null)
    },
    [creatingChildOf, setChatError],
  )

  const cancelCreate = useCallback(() => setCreatingChildOf(null), [])
  const startRenaming = useCallback((chatId: string) => setRenamingId(chatId), [])

  const confirmRename = useCallback(
    (title: string) => {
      if (renamingId && title.trim()) {
        void performRenameChat(renamingId, title, setChatError)
      }
      setRenamingId(null)
    },
    [renamingId, setChatError],
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
          // Store position before the React re-render so the ghost mounts at
          // the correct location on its first render.
          lastDragPosRef.current = { x: e.clientX, y: e.clientY }
          setDraggingChat(chat)
          const initialTrash = isOverTrash(e.clientX, e.clientY)
          hoverTrashRef.current = initialTrash
          setHoverTrash(initialTrash)
        }
        return
      }
      if (!draggingRef.current) return
      // Move ghost directly — no React state update, no tree re-render.
      if (ghostRef.current) {
        ghostRef.current.style.left = `${e.clientX + 12}px`
        ghostRef.current.style.top = `${e.clientY - 10}px`
      }
      // Only re-render when trash hover state actually changes.
      const newHoverTrash = isOverTrash(e.clientX, e.clientY)
      if (newHoverTrash !== hoverTrashRef.current) {
        hoverTrashRef.current = newHoverTrash
        setHoverTrash(newHoverTrash)
      }
    }

    function onPointerUp(e: PointerEvent) {
      pendingRef.current = null
      const chat = draggingRef.current
      if (!chat) return
      if (isOverTrash(e.clientX, e.clientY)) {
        void performDeleteChat(chat.id, setChatError)
      }
      draggingRef.current = null
      hoverTrashRef.current = false
      lastDragPosRef.current = null
      setDraggingChat(null)
      setHoverTrash(false)
    }

    function onPointerCancel() {
      pendingRef.current = null
      if (draggingRef.current) {
        draggingRef.current = null
        hoverTrashRef.current = false
        lastDragPosRef.current = null
        setDraggingChat(null)
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
  }, [setChatError])

  return (
    <>
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
          hoverTrash,
          onPointerDownDrag,
          chatErrors,
          setChatError,
          clearChatError,
        }}
      >
        {children}
      </ChatTreeContext.Provider>
      {draggingChat && (
        <div
          ref={ghostRef}
          className="pointer-events-none fixed z-50 rounded-md border border-border bg-secondary px-2 py-1 text-[13px] text-secondary-foreground shadow-md opacity-90"
          style={{
            left: lastDragPosRef.current ? lastDragPosRef.current.x + 12 : 0,
            top: lastDragPosRef.current ? lastDragPosRef.current.y - 10 : 0,
          }}
        >
          {draggingChat.label}
        </div>
      )}
    </>
  )
}
