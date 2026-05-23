import { useEffect, useRef } from 'react'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { MessageBubble } from './MessageBubble'
import { ToolCallSeparator } from './ToolCallSeparator'
import { ChatInput } from './ChatInput'

export type FlowStep = 'brainstorm' | 'spec' | 'builder' | 'ai-review' | 'human-review'

export interface ChatMessage {
  id: string
  role: 'user' | 'assistant'
  content: string
  authorName: string
  authorInitials: string
  modelName?: string
  timestamp: string
  toolCalls?: number
  durationSec?: number
}

interface ChatViewProps {
  step: FlowStep
  onStepChange: (step: FlowStep) => void
  messages: ChatMessage[]
  onSend: (message: string) => void
  modelName?: string
  tokenPct?: number
  inputPlaceholder?: string
  sending?: boolean
}

const STEPS: { value: FlowStep; label: string }[] = [
  { value: 'brainstorm', label: 'Brainstorm' },
  { value: 'spec', label: 'Spec' },
  { value: 'builder', label: 'Builder' },
  { value: 'ai-review', label: 'AI Review' },
  { value: 'human-review', label: 'Human Review' },
]

function StepDot({ state }: { state: 'done' | 'active' | 'pending' }) {
  return (
    <span className={
      'h-1.5 w-1.5 rounded-full flex-shrink-0 ' +
      (state === 'done' ? 'bg-green-500' : state === 'active' ? 'bg-primary' : 'bg-muted')
    } />
  )
}

export function ChatView({
  step,
  onStepChange,
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

  const activeIdx = STEPS.findIndex(s => s.value === step)

  return (
    <div className="flex flex-1 flex-col overflow-hidden">
      {/* Flow step tabs */}
      <Tabs value={step} onValueChange={v => onStepChange(v as FlowStep)}>
        <TabsList className="h-10 w-full justify-start gap-0 rounded-none border-b border-border bg-card px-4">
          {STEPS.map((s, i) => (
            <div key={s.value} className="flex items-center">
              <TabsTrigger
                value={s.value}
                className="flex items-center gap-1.5 rounded-none border-b-2 border-transparent px-3 py-2 text-[13px] text-muted-foreground data-[state=active]:border-primary data-[state=active]:bg-transparent data-[state=active]:text-foreground data-[state=active]:shadow-none"
              >
                <StepDot state={i < activeIdx ? 'done' : i === activeIdx ? 'active' : 'pending'} />
                {s.label}
              </TabsTrigger>
              {i < STEPS.length - 1 && (
                <span className="mx-0.5 text-[12px] text-muted-foreground/30">›</span>
              )}
            </div>
          ))}
        </TabsList>
      </Tabs>

      {/* Message list */}
      <ScrollArea className="flex-1">
        <div className="py-6">
          {messages.map((msg, i) => (
            <div key={msg.id}>
              {i > 0 && msg.role === 'assistant' && messages[i - 1].role === 'user' &&
                msg.toolCalls !== undefined && msg.durationSec !== undefined && (
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

      {/* Input */}
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
