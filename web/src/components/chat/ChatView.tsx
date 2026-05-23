import { useEffect, useRef } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { MessageBubble } from './MessageBubble'
import { ToolCallSeparator } from './ToolCallSeparator'
import { ChatInput } from './ChatInput'
import type { ChatMessage } from '@/lib/types'

export type { ChatMessage }

interface ChatViewProps {
  messages: ChatMessage[]
  onSend: (content: string, attachments: File[]) => void
  inputPlaceholder?: string
  sending?: boolean
}

function ChatEmptyState() {
  return (
    <div className="flex flex-1 flex-col items-center justify-center gap-2 py-24 text-center">
      <span className="text-2xl text-primary/50">✦</span>
      <p className="text-sm font-medium text-foreground">Start a conversation</p>
      <p className="text-xs text-muted-foreground">Ask anything about this workspace</p>
    </div>
  )
}

export function ChatView({
  messages,
  onSend,
  inputPlaceholder = 'Message…',
  sending,
}: ChatViewProps) {
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages])

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      <ScrollArea className="flex-1">
        <div className="py-6">
          {messages.length === 0 && !sending ? (
            <ChatEmptyState />
          ) : (
            messages.map((msg, i) => (
              <div key={msg.id}>
                {i > 0 &&
                  msg.role === 'assistant' &&
                  messages[i - 1].role === 'user' &&
                  msg.toolCalls !== undefined &&
                  msg.durationSec !== undefined && (
                    <ToolCallSeparator toolCalls={msg.toolCalls} durationSec={msg.durationSec} />
                  )}
                <MessageBubble
                  role={msg.role}
                  content={msg.content}
                  authorName={msg.authorName}
                  authorInitials={msg.authorInitials}
                  modelName={msg.modelName}
                  timestamp={msg.timestamp}
                  isStreaming={msg.id === 'streaming'}
                />
              </div>
            ))
          )}
          <div ref={bottomRef} />
        </div>
      </ScrollArea>
      <ChatInput
        placeholder={inputPlaceholder}
        onSend={onSend}
        disabled={sending}
      />
    </div>
  )
}
