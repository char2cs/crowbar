import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'

/**
 * What an agent sends back, rendered.
 *
 * This is the only place the real editor mounts under test — the ledger and
 * queue suites mock it, because 199 messages of real Plate starved them of
 * their 5s budget and neither one is about markdown.
 */
describe('MarkdownMessage', () => {
  it('renders an answer that is only prose', () => {
    render(<MarkdownMessage>Here is what I found.</MarkdownMessage>)
    expect(screen.getByText('Here is what I found.')).toBeInTheDocument()
  })

  // The plugin set decides what SURVIVES: @platejs/markdown drops any node whose
  // plugin is unregistered, so a read set narrower than what an agent can answer
  // with is not a missing affordance — it is a table that disappears.
  it('keeps a table an agent answered with', () => {
    render(<MarkdownMessage>{'| a | b |\n| --- | --- |\n| 1 | 2 |'}</MarkdownMessage>)
    // `closest('td')` is the whole assertion: an unregistered table plugin does
    // not error, it renders the cells as loose paragraphs.
    expect(screen.getByText('1').closest('td')).not.toBeNull()
    expect(screen.getByText('a').closest('table')).not.toBeNull()
  })

  it('keeps a fenced code block and its contents', () => {
    render(<MarkdownMessage>{'```go\nfunc main() {}\n```'}</MarkdownMessage>)
    expect(screen.getByText(/func main/).closest('[data-slate-node="element"]')).not.toBeNull()
    expect(document.querySelector('pre')).not.toBeNull()
  })

  // REGRESSION, from the engine this replaced: react-markdown v9 dropped its
  // `inline` prop and every inline code span silently became a <pre> block.
  it('renders an inline code span inline, not as a block', () => {
    render(<MarkdownMessage>{'run `make dev` first'}</MarkdownMessage>)
    const code = screen.getByText('make dev')
    expect(code).toBeInTheDocument()
    expect(code.closest('pre')).toBeNull()
  })

  it('is read-only — a transcript is read, not edited', () => {
    render(<MarkdownMessage>plain</MarkdownMessage>)
    const editable = document.querySelector('[data-slate-editor]')
    expect(editable).not.toBeNull()
    expect(editable?.getAttribute('contenteditable')).toBe('false')
  })
})
