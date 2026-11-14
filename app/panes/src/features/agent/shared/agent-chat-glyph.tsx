import { ChatGlyph } from '@/components/ui/chat-glyph'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { cn } from '@/utils/cn'

interface AgentChatGlyphProps {
  /** Raw SVG for the chat's ACTIVE provider. Empty until the chat list seeds. */
  providerIcon: string
  working: boolean
  className?: string
}

/** Every live-chat surface uses this, so a chat never reads busy in one place and
 *  idle in another. */
export function AgentChatGlyph({ providerIcon, working, className }: AgentChatGlyphProps) {
  if (working) {
    return <FlickerSpinner className={cn('size-4', className)} />
  }
  return <ChatGlyph svg={providerIcon} className={className} />
}
