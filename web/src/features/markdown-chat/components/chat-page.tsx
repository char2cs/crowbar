import { MarkdownChatView } from '@/features/markdown-chat/components/markdown-chat-view'

interface ChatPageProps {
  chatId: string
}

export function ChatPage({ chatId }: ChatPageProps) {
  return <MarkdownChatView workspaceId={chatId} stepId="chat" />
}
