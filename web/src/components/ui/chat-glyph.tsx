import { ChatsCircle } from '@phosphor-icons/react'
import { ProviderIcon } from './provider-icon'
import { cn } from '@/utils/cn'

interface ChatGlyphProps {
  /** Raw SVG for the chat's ACTIVE provider. '' until the chat list seeds. */
  svg: string
  className?: string
}

// A resting chat's look: the provider's mark, else a chat glyph. One definition so the row, tab,
// recents, drag ghost and removal tray agree; in `components/ui` so layout may use it, store-free
// (artwork arrives as an SVG string). No turn state (AgentChatGlyph adds it); colour is the host's.
export function ChatGlyph({ svg, className }: ChatGlyphProps) {
  if (svg) {
    return <ProviderIcon svg={svg} className={cn('size-4', className)} />
  }
  // No provider yet: a chat glyph still reads as a chat. `data-chat-glyph` names this fallback
  // as `data-provider-icon` does ProviderIcon, so a surface can assert which it drew.
  return (
    <ChatsCircle
      data-chat-glyph="true"
      aria-hidden="true"
      className={cn('size-4 shrink-0', className)}
      weight="regular"
    />
  )
}
