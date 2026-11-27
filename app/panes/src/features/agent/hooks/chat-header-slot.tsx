import { IS_MOBILE } from '@/utils/platform'
import { cn } from '@/lib/utils'
import { createContext, use, useMemo, useState, type ReactNode } from 'react'

interface ChatHeaderSlot {
  element: HTMLElement | null
  setElement: (element: HTMLElement | null) => void
}

const ChatHeaderSlotContext = createContext<ChatHeaderSlot | null>(null)

export function ChatHeaderSlotProvider({ children }: { children: ReactNode }) {
  const [element, setElement] = useState<HTMLElement | null>(null)
  const value = useMemo(() => ({ element, setElement }), [element, setElement])
  return <ChatHeaderSlotContext value={value}>{children}</ChatHeaderSlotContext>
}

export function ChatHeaderSlot() {
  const slot = use(ChatHeaderSlotContext)
  return (
    <div
      ref={slot?.setElement}
      className={cn('flex shrink-0 items-center gap-2', IS_MOBILE && 'empty:hidden')}
    />
  )
}

export function useChatHeaderSlotElement(): HTMLElement | null {
  return use(ChatHeaderSlotContext)?.element ?? null
}
