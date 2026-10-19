import { createContext, use, useMemo, useState, type ReactNode } from 'react'

interface ChatHeaderSlot {
  element: HTMLElement | null
  setElement: (element: HTMLElement | null) => void
}

const ChatHeaderSlotContext = createContext<ChatHeaderSlot | null>(null)

/** One per pane: the header row and the surface below it are siblings, so the
 *  surface reaches the row's slot through this rather than a second copy of its state. */
export function ChatHeaderSlotProvider({ children }: { children: ReactNode }) {
  const [element, setElement] = useState<HTMLElement | null>(null)
  const value = useMemo(() => ({ element, setElement }), [element, setElement])
  return <ChatHeaderSlotContext value={value}>{children}</ChatHeaderSlotContext>
}

/** Where a header row lets the chat surface draw its own controls. */
export function ChatHeaderSlot() {
  const slot = use(ChatHeaderSlotContext)
  return <div ref={slot?.setElement} className="flex shrink-0 items-center gap-2" />
}

/** The mounted header slot, or null when this pane has no header row to host one. */
export function useChatHeaderSlotElement(): HTMLElement | null {
  return use(ChatHeaderSlotContext)?.element ?? null
}
