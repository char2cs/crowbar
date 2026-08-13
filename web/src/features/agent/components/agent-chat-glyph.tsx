import { ChatGlyph } from '@/components/ui/chat-glyph'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { cn } from '@/utils/cn'

interface AgentChatGlyphProps {
  /** Raw SVG for the chat's ACTIVE provider. Empty until the chat list seeds. */
  providerIcon: string
  /** The chat's agent is mid-turn. */
  working: boolean
  /** Sizes the glyph box; the spinner and the icon both fill it. */
  className?: string
}

/**
 * The one glyph that stands for a LIVE chat — sidebar row, pane tab, New Tab
 * list. It answers both questions the user asks of a chat at a glance: WHOSE
 * agent it is (provider icon) and whether it is thinking (spinner). Keep those
 * call sites on this component so a chat can never read as busy in one place and
 * idle in the other.
 *
 * The idle half is {@link ChatGlyph}, shared with the surfaces that draw a chat
 * with no turn to report — the removal tray, which is in `components/layout` and
 * cannot reach into this feature at all.
 */
export function AgentChatGlyph({ providerIcon, working, className }: AgentChatGlyphProps) {
  if (working) {
    return <FlickerSpinner className={cn('size-4', className)} />
  }
  return <ChatGlyph svg={providerIcon} className={className} />
}
