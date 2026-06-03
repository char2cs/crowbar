import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'
import { useSidebarStore } from '@/lib/store/sidebar'

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
  return els.some(el => el instanceof Element && el.getAttribute('data-trash-drop') !== null)
}

export function ChatTreeProvider({ children, wsId }: { children: ReactNode; wsId: string }) {
  const [creatingChildOf, setCreatingChildOf] = useState<CreatingState | null>(null)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [draggingChat, setDraggingChat] = useState<DraggingState | null>(null)
  const [dragPos, setDragPos] = useState<{ x: number; y: number } | null>(null)
  const [hoverTrash, setHoverTrash] = useState(false)

  const pendingRef = useRef<{
    chatId: string; label: string
    startX: number; startY: number
    target: HTMLElement; pointerId: number
  } | null>(null)
  const draggingRef = useRef<DraggingState | null>(null)

  const startCreating = useCallback((parentId: string) => {
    setCreatingChildOf({ parentId })
  }, [])

  const confirmCreate = useCallback((title: string) => {
    if (!creatingChildOf || !title.trim()) return
    useSidebarStore.getState().addChat({
      id: crypto.randomUUID(),
      wsId,
      title: title.trim(),
      age: 'just now',
      parentId: creatingChildOf.parentId,
      status: 'idle',
      type: 'chat',
    })
    setCreatingChildOf(null)
  }, [creatingChildOf, wsId])

  const cancelCreate = useCallback(() => setCreatingChildOf(null), [])
  const startRenaming = useCallback((chatId: string) => setRenamingId(chatId), [])

  const confirmRename = useCallback((title: string) => {
    if (renamingId && title.trim()) {
      useSidebarStore.getState().renameChat(renamingId, title.trim())
    }
    setRenamingId(null)
  }, [renamingId])

  const cancelRename = useCallback(() => setRenamingId(null), [])

  const onPointerDownDrag = useCallback((chatId: string, label: string, e: React.PointerEvent) => {
    if (e.button !== 0) return
    if (draggingRef.current) return
    pendingRef.current = {
      chatId, label, startX: e.clientX, startY: e.clientY,
      target: e.currentTarget as HTMLElement, pointerId: e.pointerId,
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
        useSidebarStore.getState().deleteChat(chat.id)
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
    <ChatTreeContext.Provider value={{
      creatingChildOf, startCreating, confirmCreate, cancelCreate,
      renamingId, startRenaming, confirmRename, cancelRename,
      draggingChat, dragPos, hoverTrash, onPointerDownDrag,
    }}>
      {children}
    </ChatTreeContext.Provider>
  )
}
