import { ChatGlyph } from '@/components/ui/chat-glyph'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { cn } from '@/utils/cn'

interface AgentChatGlyphProps {
  providerIcon: string
  working: boolean
  className?: string
}

export function AgentChatGlyph({ providerIcon, working, className }: AgentChatGlyphProps) {
  if (working) {
    return <FlickerSpinner className={cn('size-4', className)} />
  }
  return <ChatGlyph svg={providerIcon} className={className} />
}
