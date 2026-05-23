import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { ChatView } from '@/components/chat/ChatView'
import type { ChatMessage } from '@/lib/types'

export const Route = createFileRoute('/chat/$chatId')({
  component: ChatPage,
})

const MOCK_CHAT_MESSAGES: Record<string, ChatMessage[]> = {
  c1: [
    {
      id: 'a1', role: 'user',
      content: 'How should we structure the architecture across all three Rabbyte products?',
      authorName: 'Mateo', authorInitials: 'MU', timestamp: '2h ago',
    },
    {
      id: 'a2', role: 'assistant',
      content: 'The three products share a user identity layer, so a shared auth service is the right foundation. crowbar handles agent orchestration, quiver.core is the shared library, and quiver.desktop consumes both.',
      authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
      timestamp: '2h ago · 6.1s', toolCalls: 2, durationSec: 6.1,
    },
  ],
}

function ChatPage() {
  const { chatId } = Route.useParams()
  const [messages, setMessages] = useState<ChatMessage[]>(() =>
    MOCK_CHAT_MESSAGES[chatId] ?? [],
  )
  const [sending, setSending] = useState(false)

  const handleSend = (content: string) => {
    setMessages(prev => [...prev, {
      id: `u${Date.now()}`, role: 'user', content,
      authorName: 'Mateo', authorInitials: 'MU', timestamp: 'just now',
    }])
    setSending(true)
    setTimeout(() => {
      setMessages(prev => [...prev, {
        id: `a${Date.now()}`, role: 'assistant',
        content: '(Mock response — AI not yet connected)',
        authorName: 'Claude', authorInitials: '✦', modelName: 'Sonnet 4.6',
        timestamp: 'just now',
      }])
      setSending(false)
    }, 1500)
  }

  return (
    <ChatView
      messages={messages}
      onSend={handleSend}
      sending={sending}
      modelName="Sonnet 4.6"
      inputPlaceholder="Ask about the Rabbyte project…"
    />
  )
}
