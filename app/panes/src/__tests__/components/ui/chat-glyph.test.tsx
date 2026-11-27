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
    expect(container.querySelector('[data-chat-glyph]')).toBeNull()
  })

  it('falls back to a CHAT glyph when the provider is gone', () => {
    const { container } = render(<ChatGlyph svg="" />)
    expect(container.querySelector('[data-chat-glyph]')).not.toBeNull()
    expect(container.querySelector('[data-provider-icon]')).toBeNull()
  })

  it('takes the host’s sizing on either branch', () => {
    const { container, rerender } = render(<ChatGlyph svg={CLAUDE} className="size-3.5" />)
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
