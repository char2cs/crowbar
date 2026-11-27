import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'

function renderInteractive(text: string) {
  return render(<MarkdownMessage>{text}</MarkdownMessage>)
}

describe('MarkdownMessageStatic', () => {
  it('renders an answer that is only prose', () => {
    render(<MarkdownMessageStatic>Here is what I found.</MarkdownMessageStatic>)
    expect(screen.getByText('Here is what I found.')).toBeInTheDocument()
  })

  it('keeps a table an agent answered with', () => {
    render(<MarkdownMessageStatic>{'| a | b |\n| --- | --- |\n| 1 | 2 |'}</MarkdownMessageStatic>)
    expect(screen.getByText('1').closest('td')).not.toBeNull()
    expect(screen.getByText('a').closest('table')).not.toBeNull()
  })

  it('keeps a fenced code block and its contents', () => {
    render(<MarkdownMessageStatic>{'```go\nfunc main() {}\n```'}</MarkdownMessageStatic>)
    expect(document.querySelector('pre')).not.toBeNull()
    expect(document.querySelector('pre')?.textContent).toContain('func main')
  })

  it('colours a fenced code block by its language once the grammar has loaded', async () => {
    render(<MarkdownMessageStatic>{'```go\nfunc main() {}\n```'}</MarkdownMessageStatic>)
    await vi.waitFor(
      () => expect(document.querySelector('pre .hljs-keyword')?.textContent).toBe('func'),
      { timeout: 10_000 },
    )
  }, 30_000)

  it('renders an inline code span inline, not as a block', () => {
    render(<MarkdownMessageStatic>{'run `make dev` first'}</MarkdownMessageStatic>)
    const code = screen.getByText('make dev')
    expect(code).toBeInTheDocument()
    expect(code.closest('pre')).toBeNull()
  })

  it('renders a link, href and text', () => {
    render(<MarkdownMessageStatic>{'[the site](https://example.com)'}</MarkdownMessageStatic>)
    const anchor = screen.getByText('the site').closest('a')
    expect(anchor).not.toBeNull()
    expect(anchor?.getAttribute('href')).toContain('example.com')
  })

  it('renders a callout with its marker stripped, not shown as `[!NOTE]`', () => {
    render(<MarkdownMessageStatic>{'> [!NOTE]\n> Careful with this one.'}</MarkdownMessageStatic>)
    expect(screen.getByText('Careful with this one.')).toBeInTheDocument()
    expect(screen.queryByText(/\[!NOTE\]/)).toBeNull()
  })

  it('renders an image', () => {
    render(
      <MarkdownMessageStatic>{'![an image](https://example.com/pic.png)'}</MarkdownMessageStatic>,
    )
    const img = document.querySelector('img')
    expect(img).not.toBeNull()
    expect(img?.getAttribute('src')).toContain('pic.png')
    expect(img?.getAttribute('alt')).toBe('an image')
  })

  it('renders a raw HTML block, tags and all, not as escaped text', () => {
    render(
      <MarkdownMessageStatic>
        {'<div align="center"><strong>Raw block</strong></div>'}
      </MarkdownMessageStatic>,
    )
    expect(document.querySelector('div.markdown-html-block strong')).not.toBeNull()
    expect(screen.getByText('Raw block')).toBeInTheDocument()
  })

  it('renders no drag handle for a settled attachment — text-attachment fence or file-card link', () => {
    render(
      <MarkdownMessageStatic>
        {'```text-attachment:AbC123xy\nsome long pasted text\n```'}
      </MarkdownMessageStatic>,
    )
    expect(screen.queryByRole('button', { name: /reorder this attachment/i })).toBeNull()
  })

  it('is not editable — a settled message is read, never typed into', () => {
    render(<MarkdownMessageStatic>plain</MarkdownMessageStatic>)
    const root = document.querySelector('[data-slate-editor]')
    expect(root).not.toBeNull()
    expect(root?.hasAttribute('contenteditable')).toBe(false)
  })
})

const FULL_FIXTURE = [
  '# Heading One',
  '',
  '## Heading Two',
  '',
  'A paragraph with **bold**, *italic*, and `inline code`, plus a [link](https://example.com/page).',
  '',
  '- Item one',
  '- Item two',
  '',
  '1. First',
  '2. Second',
  '',
  '| Column A | Column B |',
  '| --- | --- |',
  '| 1 | 2 |',
  '',
  '```go',
  'func main() {',
  '\tfmt.Println("hi")',
  '}',
  '```',
  '',
  '![An image](https://example.com/image.png)',
  '',
  '> [!NOTE]',
  '> Something worth calling out.',
  '',
  '<div align="center"><strong>Raw HTML block</strong></div>',
].join('\n')

const NODE_SELECTOR = '[data-slate-node="element"]'

describe('MarkdownMessageStatic vs. MarkdownMessage parity', () => {
  it('renders the same text content for the full fixture', () => {
    const interactive = renderInteractive(FULL_FIXTURE)
    const staticRender = render(<MarkdownMessageStatic>{FULL_FIXTURE}</MarkdownMessageStatic>)

    expect(staticRender.container.textContent).toBe(interactive.container.textContent)
  })

  it('renders the same element tag sequence for the full fixture', () => {
    const interactive = renderInteractive(FULL_FIXTURE)
    const staticRender = render(<MarkdownMessageStatic>{FULL_FIXTURE}</MarkdownMessageStatic>)

    const interactiveTags = Array.from(interactive.container.querySelectorAll(NODE_SELECTOR)).map(
      (el) => el.tagName,
    )
    const staticTags = Array.from(staticRender.container.querySelectorAll(NODE_SELECTOR)).map(
      (el) => el.tagName,
    )

    expect(staticTags).toEqual(interactiveTags)
    expect(staticTags.length).toBeGreaterThan(20)
    expect(staticTags).toEqual(expect.arrayContaining(['H1', 'H2', 'A', 'TABLE', 'TR', 'TH', 'TD']))
  })

  it('renders both list flavors: an unordered item (role=listitem) and an ordered <ol><li>, same as the interactive editor', () => {
    const interactive = renderInteractive(FULL_FIXTURE)
    const staticRender = render(<MarkdownMessageStatic>{FULL_FIXTURE}</MarkdownMessageStatic>)

    const staticListItems = staticRender.container.querySelectorAll('[role="listitem"]')
    const interactiveListItems = interactive.container.querySelectorAll('[role="listitem"]')
    expect(staticListItems).toHaveLength(interactiveListItems.length)
    expect(staticListItems).toHaveLength(2)

    const staticOl = staticRender.container.querySelectorAll('ol > li')
    const interactiveOl = interactive.container.querySelectorAll('ol > li')
    expect(staticOl).toHaveLength(interactiveOl.length)
    expect(staticOl).toHaveLength(2)
  })

  it('renders the same href and image src for the full fixture', () => {
    const interactive = renderInteractive(FULL_FIXTURE)
    const staticRender = render(<MarkdownMessageStatic>{FULL_FIXTURE}</MarkdownMessageStatic>)

    expect(staticRender.container.querySelector('a')?.getAttribute('href')).toBe(
      interactive.container.querySelector('a')?.getAttribute('href'),
    )
    expect(staticRender.container.querySelector('img')?.getAttribute('src')).toBe(
      interactive.container.querySelector('img')?.getAttribute('src'),
    )
  })

  it('breaks a chat message’s line where the interactive editor does', () => {
    const text = 'one\ntwo\nthree'
    const interactive = renderInteractive(text)
    const staticRender = render(
      <MarkdownMessageStatic variant="chat">{text}</MarkdownMessageStatic>,
    )
    const whiteSpace = (root: HTMLElement) =>
      getComputedStyle(root.querySelector('.agent-prose')!).whiteSpace

    expect(staticRender.container.textContent).toBe(interactive.container.textContent)
    expect(whiteSpace(interactive.container)).toBe('pre-wrap')
    expect(whiteSpace(staticRender.container)).toBe(whiteSpace(interactive.container))
  })

  it('renders the callout and raw HTML block identically', () => {
    const interactive = renderInteractive(FULL_FIXTURE)
    const staticRender = render(<MarkdownMessageStatic>{FULL_FIXTURE}</MarkdownMessageStatic>)

    expect(
      staticRender.container.querySelector('[data-slate-node="element"] .markdown-html-block')
        ?.innerHTML,
    ).toBe(
      interactive.container.querySelector('[data-slate-node="element"] .markdown-html-block')
        ?.innerHTML,
    )
    expect(staticRender.container.textContent).toContain('Something worth calling out.')
    expect(staticRender.container.textContent).not.toMatch(/\[!NOTE\]/)
  })
})
