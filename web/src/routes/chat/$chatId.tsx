// web/src/routes/chat/$chatId.tsx
import { useState, useEffect, useRef, useCallback } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { nanoid } from 'nanoid'
import { ChatView } from '@/components/chat/ChatView'
import { simulateStream } from '@/lib/mock/simulate-stream'
import { getMockChat } from '@/lib/mock/chats'
import type { ChatMessage } from '@/lib/types'

export const Route = createFileRoute('/chat/$chatId')({
  component: ChatPage,
})

export function ChatPage() {
  const { chatId } = Route.useParams()
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [sending, setSending] = useState(false)
  const cancelStreamRef = useRef<(() => void) | null>(null)

  // Cancel any in-flight stream and reset when chatId changes
  useEffect(() => {
    cancelStreamRef.current?.()
    cancelStreamRef.current = null
    setSending(false)
    setMessages(getMockChat(chatId)?.messages ?? [])
  }, [chatId])

  // Cancel stream on unmount
  useEffect(() => {
    return () => { cancelStreamRef.current?.() }
  }, [])

  const handleSend = useCallback((content: string, _attachments: File[]) => {
    cancelStreamRef.current?.()

    const userMsg: ChatMessage = {
      id: nanoid(), role: 'user', content,
      authorName: 'Mateo', authorInitials: 'MU', timestamp: 'just now',
    }
    setMessages(prev => [...prev, userMsg])
    setSending(true)

    cancelStreamRef.current = simulateStream(
      'I can help with that. What would you like to know?',
      (chunk) => {
        setMessages(prev => {
          const last = prev[prev.length - 1]
          if (last?.role === 'assistant' && last.id === 'streaming') {
            return [...prev.slice(0, -1), { ...last, content: last.content + chunk }]
          }
          return [...prev, {
            id: 'streaming', role: 'assistant', content: chunk,
            authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
            timestamp: 'just now',
          }]
        })
      },
      () => {
        setMessages(prev => prev.map(m => m.id === 'streaming' ? { ...m, id: nanoid() } : m))
        setSending(false)
        cancelStreamRef.current = null
      },
    )
  }, [])

  return (
    <ChatView
      messages={messages}
      onSend={handleSend}
      sending={sending}
      inputPlaceholder="Ask about the Rabbyte project…"
    />
  )
}
