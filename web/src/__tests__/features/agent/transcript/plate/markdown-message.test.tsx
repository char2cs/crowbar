import { render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { __resetPerfForTests } from '@/lib/perf/instrumentation'

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

// A streaming bubble is one long-lived MarkdownMessage instance whose `children`
// grows on every token — see streaming-value-patch.ts. These pin down the
// property that exists to fix: growth PATCHES the existing document instead of
// rebuilding it, so nothing already on screen is torn down and recreated.
describe('MarkdownMessage streaming growth', () => {
  it('does not recreate the editor element across a streaming update', () => {
    const { rerender } = render(<MarkdownMessage>Building a</MarkdownMessage>)
    const before = document.querySelector('[data-slate-editor]')

    rerender(<MarkdownMessage>Building a CLI</MarkdownMessage>)

    expect(document.querySelector('[data-slate-editor]')).toBe(before)
  })

  it('keeps the same paragraph element while its text grows token by token', async () => {
    const { rerender, container } = render(<MarkdownMessage>Building</MarkdownMessage>)
    const paragraph = document.querySelector('[data-slate-node="element"]')

    rerender(<MarkdownMessage>Building a CLI</MarkdownMessage>)

    // Slate/Plate flush the patch's onChange on the next microtask, not
    // synchronously with the transform call — same reason the perf-span tests
    // below await a settle rather than asserting right after rerender.
    // `container.textContent`, not `getByText`: the appended run lands in its
    // own fade-in span (see the fade-in describe block below), so the two
    // halves are separate elements even though they read as one sentence.
    await waitFor(() => expect(container.textContent).toBe('Building a CLI'))
    expect(document.querySelector('[data-slate-node="element"]')).toBe(paragraph)
  })

  it('leaves an earlier, finished paragraph untouched when a new one starts', async () => {
    const { rerender } = render(<MarkdownMessage>{'First paragraph.'}</MarkdownMessage>)
    const firstElement = screen.getByText('First paragraph.')

    rerender(<MarkdownMessage>{'First paragraph.\n\nSecond'}</MarkdownMessage>)

    await waitFor(() => expect(screen.getByText('Second')).toBeInTheDocument())
    expect(screen.getByText('First paragraph.')).toBe(firstElement)
  })

  it('still renders correctly when growth is not a pure append (e.g. list markers resolve differently)', async () => {
    // Not the fast path — the parse of "- a" alone and "- a\n- b" differ by more
    // than a trailing append (a list wraps the item). The patch must still land
    // on the right document, just via the block-replace fallback.
    const { rerender } = render(<MarkdownMessage>{'- a'}</MarkdownMessage>)
    rerender(<MarkdownMessage>{'- a\n- b'}</MarkdownMessage>)

    await waitFor(() => expect(screen.getByText('b')).toBeInTheDocument())
    expect(screen.getByText('a')).toBeInTheDocument()
  })
})

// The visual this whole file exists to fix: new text fades in instead of
// popping onto the screen at full brightness, cascading word by word rather
// than however large a chunk the transport happened to deliver — see the
// 2026-08-28 chunking investigation for why that transport granularity is
// fixed and staggering the REVEAL is the lever left to pull. Both paths mark
// what they just inserted (streaming-value-patch.ts); this pins down that
// the mark actually reaches the DOM as `.chat-fresh-text` spans, one per
// word, with increasing delay, and only around the new text.
describe('MarkdownMessage streaming fade-in', () => {
  it('splits a plain appended run into one fade-in span per word, and nothing else', async () => {
    const { rerender, container } = render(<MarkdownMessage>Building</MarkdownMessage>)
    rerender(<MarkdownMessage>Building a CLI</MarkdownMessage>)

    await waitFor(() => expect(container.textContent).toBe('Building a CLI'))
    const fresh = container.querySelectorAll('.chat-fresh-text')
    expect(Array.from(fresh).map((el) => el.textContent)).toEqual([' a ', 'CLI'])
    expect(screen.getByText('Building').closest('.chat-fresh-text')).toBeNull()
  })

  it('stages each word with increasing animation-delay', async () => {
    const { rerender, container } = render(<MarkdownMessage>{'one'}</MarkdownMessage>)
    rerender(<MarkdownMessage>{'one two three four'}</MarkdownMessage>)

    await waitFor(() => expect(container.textContent).toBe('one two three four'))
    const delays = Array.from(container.querySelectorAll('.chat-fresh-text')).map(
      (el) => (el as HTMLElement).style.animationDelay,
    )
    // 150ms base (SCROLL_LEAD_MS — lets scroll-follow get underway before any
    // word in the chunk starts revealing) plus the usual 30ms/word stagger.
    expect(delays).toEqual(['150ms', '180ms', '210ms'])
  })

  it('wraps a single-word new block in one fade-in span (fallback path)', async () => {
    const { rerender, container } = render(<MarkdownMessage>{'First.'}</MarkdownMessage>)
    rerender(<MarkdownMessage>{'First.\n\nSecond.'}</MarkdownMessage>)

    await waitFor(() => expect(container.textContent).toBe('First.Second.'))
    const fresh = container.querySelectorAll('.chat-fresh-text')
    expect(fresh).toHaveLength(1)
    expect(fresh[0]?.textContent).toBe('Second.')
    expect(screen.getByText('First.').closest('.chat-fresh-text')).toBeNull()
  })

  it('cascades a multi-word new block across several staggered spans (fallback path)', async () => {
    const { rerender, container } = render(<MarkdownMessage>{'First.'}</MarkdownMessage>)
    rerender(<MarkdownMessage>{'First.\n\nA whole new sentence.'}</MarkdownMessage>)

    await waitFor(() => expect(container.textContent).toBe('First.A whole new sentence.'))
    expect(
      Array.from(container.querySelectorAll('.chat-fresh-text')).map((el) => el.textContent),
    ).toEqual(['A ', 'whole ', 'new ', 'sentence.'])
  })

  it('does not wrap text that was already there before streaming began', () => {
    // The very first render has nothing to fade — only growth after mount
    // (streaming-value-patch.ts's fast/fallback paths) ever sets the mark.
    render(<MarkdownMessage>Already here.</MarkdownMessage>)
    expect(document.querySelector('.chat-fresh-text')).toBeNull()
  })

  it('a later chunk gets its own independent fade spans, not a merge into the still-fading ones', async () => {
    // Consecutive chunks land as separate leaves until each settles — see
    // chat-fresh-text-plugin.test.ts. Rendered, that is separate
    // `.chat-fresh-text` spans, not one growing span, which is what lets
    // several trailing words fade at different stages at once, matching the
    // source video.
    const { rerender, container } = render(<MarkdownMessage>Hello</MarkdownMessage>)
    rerender(<MarkdownMessage>{'Hello there'}</MarkdownMessage>)
    await waitFor(() => expect(container.textContent).toBe('Hello there'))

    rerender(<MarkdownMessage>{'Hello there friend'}</MarkdownMessage>)
    await waitFor(() => expect(container.textContent).toBe('Hello there friend'))

    expect(container.querySelectorAll('.chat-fresh-text')).toHaveLength(2)
  })
})

describe('chat.stream.token perf span', () => {
  beforeEach(() => {
    __resetPerfForTests()
    window.__CROWBAR_PERF__ = true
  })

  afterEach(() => {
    delete window.__CROWBAR_PERF__
  })

  it('records one measure per distinct text value, on mount and on update', async () => {
    const { rerender } = render(<MarkdownMessage>first</MarkdownMessage>)
    await waitFor(() => {
      expect(performance.getEntriesByName('chat.stream.token', 'measure')).toHaveLength(1)
    })

    rerender(<MarkdownMessage>first second</MarkdownMessage>)
    await waitFor(() => {
      expect(performance.getEntriesByName('chat.stream.token', 'measure')).toHaveLength(2)
    })
  })

  it('does not record a second measure when props are unchanged', async () => {
    const { rerender } = render(<MarkdownMessage>steady</MarkdownMessage>)
    await waitFor(() => {
      expect(performance.getEntriesByName('chat.stream.token', 'measure')).toHaveLength(1)
    })

    rerender(<MarkdownMessage>steady</MarkdownMessage>)
    // Give any errant effect a chance to run, then assert no growth.
    await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
    expect(performance.getEntriesByName('chat.stream.token', 'measure')).toHaveLength(1)
  })
})
