import { render, act } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { MarkdownPreview } from '@/features/panes/lib/markdown'

// ── Shiki mock ────────────────────────────────────────────────────────────────
// shiki uses dynamic import and wasm in the browser — neither works in jsdom.
// We mock the bundle so the ShikiCodeBlock component receives a predictable
// HTML string and we can assert the highlighted path as well as the fallback.

vi.mock('shiki/bundle/full', () => {
  const fakeHl = {
    loadLanguage: vi.fn().mockResolvedValue(undefined),
    codeToHtml: vi.fn().mockImplementation((code: string, { lang }: { lang: string }) => {
      return Promise.resolve(
        `<pre class="shiki"><code data-lang="${lang}"><span class="line">${code}</span></code></pre>`,
      )
    }),
  }
  return {
    getSingletonHighlighter: vi.fn().mockResolvedValue(fakeHl),
  }
})

// ── Helpers ────────────────────────────────────────────────────────────────────

/** Flush all pending promises (micro-task queue). */
async function flushPromises() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
}

// Reset the shiki singleton between tests so each test starts fresh.
beforeEach(async () => {
  // The singleton is module-level; re-importing forces a fresh module but
  // that's heavy. Instead we just let tests that need the highlight to await
  // the mock promise. The singleton accumulates across tests but the mock
  // always returns the same deterministic HTML so it doesn't matter.
})

afterEach(() => {
  vi.clearAllMocks()
})

// ── Tests ──────────────────────────────────────────────────────────────────────

describe('MarkdownPreview', () => {
  it('renders plain paragraph text', () => {
    render(<MarkdownPreview>Hello **world**</MarkdownPreview>)
    // react-markdown renders strong elements for **...**
    const strong = document.querySelector('strong')
    expect(strong).not.toBeNull()
    expect(strong?.textContent).toBe('world')
  })

  it('renders a fenced code block as plain fallback immediately (before shiki resolves)', () => {
    const { container } = render(<MarkdownPreview>{'```ts\nconst x = 1\n```'}</MarkdownPreview>)
    // Before async shiki resolves, the plain <pre><code> fallback should be present.
    const pre = container.querySelector('pre')
    expect(pre).not.toBeNull()
    expect(pre?.textContent).toContain('const x = 1')
  })

  it('upgrades to shiki-highlighted markup after async load', async () => {
    const { container } = render(<MarkdownPreview>{'```ts\nconst x = 1\n```'}</MarkdownPreview>)

    // Wait for the shiki promise chain to settle.
    await flushPromises()
    await flushPromises() // second flush for the highlight call itself

    // After upgrade the shiki mock returns HTML with data-lang.
    const shikiEl = container.querySelector('[data-lang="ts"]')
    expect(shikiEl).not.toBeNull()
    expect(shikiEl?.textContent).toContain('const x = 1')
  })

  it('renders inline code without shiki', () => {
    render(<MarkdownPreview>{'Use `console.log()` here'}</MarkdownPreview>)
    const inlineCode = document.querySelector('code')
    expect(inlineCode).not.toBeNull()
    expect(inlineCode?.textContent).toContain('console.log()')
  })

  it('renders a fenced block without a language tag as plain code', () => {
    const { container } = render(<MarkdownPreview>{'```\nplain block\n```'}</MarkdownPreview>)
    const pre = container.querySelector('pre')
    expect(pre).not.toBeNull()
    expect(pre?.textContent).toContain('plain block')
  })

  it('does not throw when shiki fails for unknown language', async () => {
    // Override loadLanguage to throw for this test only.
    const { getSingletonHighlighter } = await import('shiki/bundle/full')
    const hl = await (getSingletonHighlighter as ReturnType<typeof vi.fn>)()
    ;(hl.loadLanguage as ReturnType<typeof vi.fn>).mockRejectedValueOnce(
      new Error('unknown language'),
    )

    expect(() => {
      render(<MarkdownPreview>{'```unknownlang\ncode here\n```'}</MarkdownPreview>)
    }).not.toThrow()

    // Await and confirm we still have a plain fallback, no crash.
    await flushPromises()
    await flushPromises()
    const pre = document.querySelector('pre')
    expect(pre).not.toBeNull()
  })
})
