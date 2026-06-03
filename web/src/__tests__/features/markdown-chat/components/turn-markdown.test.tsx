import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import { TurnMarkdown } from '@/features/markdown-chat/components/markdown/turn-markdown'

describe('TurnMarkdown', () => {
  it('renders markdown elements (heading, bold, inline code)', () => {
    render(<TurnMarkdown content={'## Hello World\n\nThis is **bold** and `inline`.'} />)
    expect(screen.getByRole('heading', { level: 2 }).textContent).toContain('Hello World')
    expect(screen.getByText('bold').tagName).toBe('STRONG')
    expect(screen.getByText('inline').tagName).toBe('CODE')
  })

  it('converts a <!-- tool-call --> comment into a pill', () => {
    const tc =
      '<!-- tool-call:{"name":"read_file","args":{"path":"a.ts"},"status":"done","output":"x"} -->'
    render(<TurnMarkdown content={tc} />)
    expect(screen.getByText('read_file')).toBeTruthy()
  })

  it('renders raw text (no markdown parse) while streaming', () => {
    render(<TurnMarkdown content={'## not a heading'} streaming />)
    expect(screen.getByText(/## not a heading/)).toBeTruthy()
    expect(screen.queryByRole('heading')).toBeNull()
  })
})
