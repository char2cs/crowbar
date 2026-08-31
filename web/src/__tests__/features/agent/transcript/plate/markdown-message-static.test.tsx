import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'

/**
 * A settled message, rendered without an interactive editor.
 *
 * These mirror markdown-message.test.tsx's own smoke tests (same fixtures,
 * same assertions), plus a full-fixture parity suite against the interactive
 * `MarkdownMessage` — the two must never visibly disagree, since a reader
 * flips between them (streaming -> settled) mid-conversation without any
 * visual seam.
 */
describe('MarkdownMessageStatic', () => {
  it('renders an answer that is only prose', () => {
    render(<MarkdownMessageStatic>Here is what I found.</MarkdownMessageStatic>)
    expect(screen.getByText('Here is what I found.')).toBeInTheDocument()
  })

  it('keeps a table an agent answered with', () => {
    render(
      <MarkdownMessageStatic>{'| a | b |\n| --- | --- |\n| 1 | 2 |'}</MarkdownMessageStatic>,
    )
    expect(screen.getByText('1').closest('td')).not.toBeNull()
    expect(screen.getByText('a').closest('table')).not.toBeNull()
  })

  it('keeps a fenced code block and its contents', () => {
    render(<MarkdownMessageStatic>{'```go\nfunc main() {}\n```'}</MarkdownMessageStatic>)
    expect(document.querySelector('pre')).not.toBeNull()
    expect(document.querySelector('pre')?.textContent).toContain('func main')
  })

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
    render(<MarkdownMessageStatic>{'![an image](https://example.com/pic.png)'}</MarkdownMessageStatic>)
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

  it('is not editable — a settled message is read, never typed into', () => {
    render(<MarkdownMessageStatic>plain</MarkdownMessageStatic>)
    // PlateStatic's root also carries [data-slate-editor] (same class names,
    // same node markup as the interactive path — that's the whole point of
    // sharing one plugin family) but, unlike MarkdownMessage's read-only
    // Plate/PlateContent, never sets `contenteditable` at all: there is no
    // Slate editable surface underneath it to mark false.
    const root = document.querySelector('[data-slate-editor]')
    expect(root).not.toBeNull()
    expect(root?.hasAttribute('contenteditable')).toBe(false)
  })
})

// The full representative fixture — every node type the chat markdown pipeline
// is responsible for: headings, a paragraph carrying marks, an inline link, an
// unordered and an ordered list, a table, a fenced code block, an image, a
// callout, and a raw HTML block. Both components consume the exact same
// plugin family (chatComposerPlugins vs. its chatComposerPluginsStatic
// derivative — see chat-composer-plugins.ts), so any divergence here is a
// real bug in one of the static-safe node swaps, not a fixture mismatch.
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

// The real tags this app's Plate renders vary by node: headings, links,
// tables, code blocks and images use literal semantic tags (h1/h2/a/table/
// pre/img), but a paragraph — including every list item, a table cell's
// contents, and a callout's body — renders as a plain `<div>` (see
// paragraph-node.tsx/PlateElement), and an unordered list item never gets a
// `<ul>` wrapper at all (see block-list.tsx: BlockList only wraps an
// ORDERED item, each in its own single-item `<ol>`).
//
// `[data-slate-node="element"]` is the one marker BOTH the interactive
// (`platejs/react`) and static (`platejs/static`) renderers set on every
// element node, whatever its real HTML tag — the actual "block-level
// structure" signal. (`data-slate-type` and the other `data-slate-<prop>`
// attributes seen on the static render are a `platejs/static`-only
// introspection feature; the interactive editor never sets them, confirmed
// by rendering the same fixture through MarkdownMessage directly.)
const NODE_SELECTOR = '[data-slate-node="element"]'

describe('MarkdownMessageStatic vs. MarkdownMessage parity', () => {
  it('renders the same text content for the full fixture', () => {
    const interactive = render(<MarkdownMessage>{FULL_FIXTURE}</MarkdownMessage>)
    const staticRender = render(<MarkdownMessageStatic>{FULL_FIXTURE}</MarkdownMessageStatic>)

    expect(staticRender.container.textContent).toBe(interactive.container.textContent)
  })

  it('renders the same element tag sequence for the full fixture', () => {
    const interactive = render(<MarkdownMessage>{FULL_FIXTURE}</MarkdownMessage>)
    const staticRender = render(<MarkdownMessageStatic>{FULL_FIXTURE}</MarkdownMessageStatic>)

    const interactiveTags = Array.from(interactive.container.querySelectorAll(NODE_SELECTOR)).map(
      (el) => el.tagName,
    )
    const staticTags = Array.from(staticRender.container.querySelectorAll(NODE_SELECTOR)).map(
      (el) => el.tagName,
    )

    expect(staticTags).toEqual(interactiveTags)
    // Every node type this fixture exercises actually showed up — a passing
    // empty-array comparison would be a vacuous pass, not parity.
    expect(staticTags.length).toBeGreaterThan(20)
    // IMG itself isn't in this list: the void image node's OWN
    // `[data-slate-node="element"]` wrapper is a DIV (`<img>` is nested
    // inside it) — see the dedicated href/src parity test below.
    expect(staticTags).toEqual(
      expect.arrayContaining(['H1', 'H2', 'A', 'TABLE', 'TR', 'TH', 'TD']),
    )
  })

  it('renders both list flavors: an unordered item (role=listitem) and an ordered <ol><li>, same as the interactive editor', () => {
    const interactive = render(<MarkdownMessage>{FULL_FIXTURE}</MarkdownMessage>)
    const staticRender = render(<MarkdownMessageStatic>{FULL_FIXTURE}</MarkdownMessageStatic>)

    // The unordered items get no `<ul>` wrapper — `role="listitem"` (set by
    // list-kit.tsx's `transformProps`, a real DOM attribute, not a
    // static-only introspection one) is the marker both renderers carry.
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
    const interactive = render(<MarkdownMessage>{FULL_FIXTURE}</MarkdownMessage>)
    const staticRender = render(<MarkdownMessageStatic>{FULL_FIXTURE}</MarkdownMessageStatic>)

    expect(staticRender.container.querySelector('a')?.getAttribute('href')).toBe(
      interactive.container.querySelector('a')?.getAttribute('href'),
    )
    expect(staticRender.container.querySelector('img')?.getAttribute('src')).toBe(
      interactive.container.querySelector('img')?.getAttribute('src'),
    )
  })

  it('renders the callout and raw HTML block identically', () => {
    const interactive = render(<MarkdownMessage>{FULL_FIXTURE}</MarkdownMessage>)
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
