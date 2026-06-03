// web/src/routes/chat/$chatId.tsx
import { createFileRoute } from '@tanstack/react-router'
import { MarkdownChatView } from '@/features/markdown-chat/components/markdown-chat-view'

export const Route = createFileRoute('/chat/$chatId')({
  component: ChatPage,
})

export function ChatPage() {
  const { chatId } = Route.useParams()
  return <MarkdownChatView workspaceId={chatId} stepId="chat" />
}
