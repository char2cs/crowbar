import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'

// The paragraph as a live drop target (drop line while an attachment hovers
// it, move on release) is covered end to end in dnd-scope.test.tsx.
describe('ChatParagraphElement', () => {
  it('renders a plain paragraph normally, with no DndScope ancestor at all', () => {
    expect(() => render(<MarkdownMessage>{'just some text'}</MarkdownMessage>)).not.toThrow()
    const paragraph = screen.getByText('just some text').closest('.slate-p')
    expect(paragraph).not.toBeNull()
    expect(paragraph!.querySelector('.-top-px, .-bottom-px')).toBeNull()
  })
})

describe('static paragraph rendering', () => {
  it('renders a settled message paragraph with no drop line', () => {
    render(<MarkdownMessageStatic>{'just some text'}</MarkdownMessageStatic>)
    const paragraph = screen.getByText('just some text').closest('.slate-p')
    expect(paragraph).not.toBeNull()
    expect(paragraph!.querySelector('.-top-px, .-bottom-px')).toBeNull()
  })
})
