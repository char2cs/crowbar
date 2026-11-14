/**
 * What a chat looks like when it is not thinking.
 *
 * One definition, shared by every surface that draws a chat: the sidebar row and
 * the pane tab (through AgentChatGlyph), the New Tab list, the drag ghost (which
 * clones the row) and the REMOVAL TRAY — which lives in components/layout and
 * cannot reach into the agent feature, and which drew a generic message-square
 * stand-in until this was extracted. A chat held for removal that does not look
 * like the chat it came from is wrong at the one moment it matters most.
 */
import { cleanup, render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it } from 'vitest'
import { ChatGlyph } from '@/components/ui/chat-glyph'

afterEach(cleanup)

const CLAUDE = '<svg data-p="claude"></svg>'

describe('ChatGlyph', () => {
  it('draws the provider’s own artwork when there is any', () => {
    const { container } = render(<ChatGlyph svg={CLAUDE} />)
    expect(container.querySelector('[data-provider-icon]')).not.toBeNull()
    expect(container.querySelector('[data-p="claude"]')).not.toBeNull()
    // …and NOT the fallback, which is the whole point of the fallback.
    expect(container.querySelector('[data-chat-glyph]')).toBeNull()
  })

  it('falls back to a CHAT glyph when the provider is gone', () => {
    // The list is still seeding, or the chat's provider was turned off. A chat
    // still reads as a chat; a generic file icon does not.
    const { container } = render(<ChatGlyph svg="" />)
    expect(container.querySelector('[data-chat-glyph]')).not.toBeNull()
    expect(container.querySelector('[data-provider-icon]')).toBeNull()
  })

  it('takes the host’s sizing on either branch', () => {
    const { container, rerender } = render(<ChatGlyph svg={CLAUDE} className="size-3.5" />)
    // getAttribute, not .className: the fallback is an <svg>, whose className is
    // an SVGAnimatedString rather than a string.
    expect(container.querySelector('[data-provider-icon]')?.getAttribute('class')).toContain(
      'size-3.5',
    )
    rerender(<ChatGlyph svg="" className="size-3.5" />)
    expect(container.querySelector('[data-chat-glyph]')?.getAttribute('class')).toContain(
      'size-3.5',
    )
  })

  it('is inert to assistive tech — the row’s label is what names the chat', () => {
    render(<ChatGlyph svg={CLAUDE} />)
    expect(screen.queryByRole('img')).toBeNull()
  })
})
