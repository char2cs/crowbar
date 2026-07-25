import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MermaidCodeBlock } from '@/features/editor/markdown/plate/mermaid-code-block'

// `mermaid` is never exercised for real in jsdom (it needs real SVG layout
// APIs like `getBBox`, which jsdom doesn't implement) — mock it the same way
// the app already mocks a forced-throw deserialize path
// (markdown-editor-pane.test.tsx's THROW_SENTINEL), driving the exact two
// outcomes this component must handle: a successful render, and mermaid
// throwing on invalid syntax.
vi.mock('mermaid', () => ({
  default: {
    initialize: vi.fn(),
    render: vi.fn((_id: string, code: string) => {
      if (code.includes('BAD')) {
        return Promise.reject(new Error('Parse error: expected token'))
      }
      return Promise.resolve({ svg: '<svg data-testid="rendered-diagram"></svg>' })
    }),
  },
}))

describe('MermaidCodeBlock', () => {
  it('renders the diagram and auto-collapses the source once mermaid succeeds', async () => {
    render(
      <MermaidCodeBlock code="flowchart LR\nA --> B">
        <pre data-testid="raw-source">flowchart LR A --&gt; B</pre>
      </MermaidCodeBlock>,
    )

    // The toggle only appears once a diagram has actually rendered.
    expect(await screen.findByText('Edit source')).toBeInTheDocument()
    expect(document.querySelector('[data-testid="rendered-diagram"]')).toBeInTheDocument()

    // ...and the raw source is collapsed (CSS-hidden, not unmounted — see
    // the component's own comment on why it must stay mounted).
    const source = screen.getByTestId('raw-source')
    expect(source).toBeInTheDocument()
    expect(source.parentElement?.className).toContain('hidden')

    // Clicking the toggle reveals it again, still without unmounting it.
    fireEvent.click(screen.getByText('Edit source'))
    expect(source.parentElement?.className).not.toContain('hidden')
  })

  it('falls back to the raw source plus a quiet inline error on invalid mermaid syntax, without throwing', async () => {
    expect(() =>
      render(
        <MermaidCodeBlock code="BAD SYNTAX HERE">
          <pre data-testid="raw-source">BAD SYNTAX HERE</pre>
        </MermaidCodeBlock>,
      ),
    ).not.toThrow()

    expect(await screen.findByText(/couldn't render this diagram/i)).toBeInTheDocument()

    // The source was never editable-but-hidden here: on error it's forced
    // visible so the user can immediately see and fix the invalid line —
    // "reachable and editable" holds even for a broken diagram.
    const source = screen.getByTestId('raw-source')
    expect(source.parentElement?.className).not.toContain('hidden')
    // No toggle while broken — there's nothing to collapse to.
    expect(screen.queryByText('Edit source')).toBeNull()
  })

  it('never sets state (or warns) after unmount while a render is in flight', async () => {
    // Fake timers make the debounce + mocked async render deterministic —
    // no wall-clock sleep needed to observe what happens once they settle.
    vi.useFakeTimers()
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    try {
      const { unmount } = render(
        <MermaidCodeBlock code="flowchart LR\nA --> B">
          <pre data-testid="raw-source">x</pre>
        </MermaidCodeBlock>,
      )
      // Unmount before the debounce timer (and the mocked async render)
      // ever fires.
      unmount()
      // Advance past the debounce delay and flush the mocked import()/render
      // promise chain. If the `mountedRef` guard didn't work, the pending
      // `setSvg`/`onStateChange` calls would fire on an unmounted component.
      await vi.advanceTimersByTimeAsync(1000)
      expect(errorSpy).not.toHaveBeenCalled()
    } finally {
      errorSpy.mockRestore()
      vi.useRealTimers()
    }
  })
})
