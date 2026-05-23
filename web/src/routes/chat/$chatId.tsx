import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { ChatView } from '@/components/chat/ChatView'
import { getMockChat } from '@/lib/mock/chats'
import type { ChatMessage } from '@/lib/types'

export const Route = createFileRoute('/chat/$chatId')({
  component: ChatPage,
})

function ChatPage() {
  const { chatId } = Route.useParams()
  const [messages, setMessages] = useState<ChatMessage[]>(() =>
    getMockChat(chatId)?.messages ?? [],
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
