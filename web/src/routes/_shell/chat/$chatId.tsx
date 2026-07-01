import { createFileRoute } from '@tanstack/react-router'
import { ChatPage } from '@/features/markdown-chat/components/chat-page'

export const Route = createFileRoute('/_shell/chat/$chatId')({
  component: ChatRouteComponent,
})

function ChatRouteComponent() {
  const { chatId } = Route.useParams()
  return <ChatPage chatId={chatId} />
}
