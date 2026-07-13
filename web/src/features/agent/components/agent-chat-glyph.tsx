import { ChatsCircle } from '@phosphor-icons/react'
import { FlickerSpinner } from '@/components/ui/flicker-spinner'
import { ProviderIcon } from './provider-icon'
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
 * The one glyph that stands for a chat, wherever a chat is listed — sidebar row,
 * pane tab. It answers both questions the user asks of a chat at a glance: WHOSE
 * agent it is (provider icon) and whether it is thinking (spinner). Keep the two
 * call sites on this component so a chat can never read as busy in one place and
 * idle in the other.
 *
 * Colour is deliberately absent: the spinner's dots and the provider's paths are
 * both `currentColor`, so the host row/tab tints them with its own text token.
 */
export function AgentChatGlyph({ providerIcon, working, className }: AgentChatGlyphProps) {
  if (working) {
    return <FlickerSpinner className={cn('size-4', className)} />
  }
  if (providerIcon) {
    return <ProviderIcon svg={providerIcon} className={cn('size-4', className)} />
  }
  // No provider yet (list still seeding, or a chat whose provider went away):
  // a chat glyph still reads as a chat — a generic file icon does not.
  return (
    <ChatsCircle aria-hidden="true" className={cn('size-4 shrink-0', className)} weight="regular" />
  )
}
