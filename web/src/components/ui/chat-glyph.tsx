import { ChatsCircle } from '@phosphor-icons/react'
import { ProviderIcon } from './provider-icon'
import { cn } from '@/utils/cn'

interface ChatGlyphProps {
  /** Raw SVG for the chat's ACTIVE provider. '' until the chat list seeds. */
  svg: string
  className?: string
}

/**
 * What a chat LOOKS like when it is not thinking — the provider's own mark, or a
 * chat glyph when there is no provider to draw.
 *
 * One definition, because a chat is drawn in five places and they must agree:
 * the sidebar row, the pane tab, the New Tab surface's recent list, the drag
 * ghost (which clones the row) and the removal tray. The tray is why this was
 * extracted: it drew a generic message-square stand-in, so a chat held for
 * removal did not look like the chat it came from — at the one moment the user
 * most needs to recognise what is about to be destroyed.
 *
 * It lives in `components/ui` rather than in the agent feature because the tray
 * lives in `components/layout` and must not reach across into a feature. The
 * provider's artwork reaches it as an SVG string on the removal draft, which is
 * also what keeps this component free of any store.
 *
 * NO turn state. {@link AgentChatGlyph} wraps this with the flip-dot spinner for
 * the surfaces that track a live chat; a chat in the tray is on its way out and
 * is not doing a turn.
 *
 * Colour is deliberately absent: the provider's paths are `currentColor`, so the
 * host row/tab/tray tints them with its own text token.
 */
export function ChatGlyph({ svg, className }: ChatGlyphProps) {
  if (svg) {
    return <ProviderIcon svg={svg} className={cn('size-4', className)} />
  }
  // No provider yet (list still seeding, or a chat whose provider went away): a
  // chat glyph still reads as a chat — a generic file icon does not. The
  // data-chat-glyph marker names this fallback the same way ProviderIcon carries
  // data-provider-icon, so a surface can assert it renders THIS glyph and not
  // some other icon (the whole point of the fallback) rather than merely
  // "some svg".
  return (
    <ChatsCircle
      data-chat-glyph="true"
      aria-hidden="true"
      className={cn('size-4 shrink-0', className)}
      weight="regular"
    />
  )
}
