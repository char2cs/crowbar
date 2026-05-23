import { useEffect, useRef } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { MessageBubble } from './MessageBubble'
import { ToolCallSeparator } from './ToolCallSeparator'
import { ChatInput } from './ChatInput'
import type { ChatMessage } from '@/lib/types'

export type { ChatMessage }

interface ChatViewProps {
  messages: ChatMessage[]
  onSend: (content: string) => void
  modelName?: string
  tokenPct?: number
  inputPlaceholder?: string
  sending?: boolean
}

export function ChatView({
  messages,
  onSend,
  modelName = 'Sonnet 4.6',
  tokenPct = 0,
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
          {messages.map((msg, i) => (
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
              />
            </div>
          ))}
          <div ref={bottomRef} />
        </div>
      </ScrollArea>
      <ChatInput
        placeholder={inputPlaceholder}
        onSend={onSend}
        modelName={modelName}
        tokenPct={tokenPct}
        disabled={sending}
      />
    </div>
  )
}
