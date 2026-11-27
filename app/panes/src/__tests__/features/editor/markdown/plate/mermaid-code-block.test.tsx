import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { MermaidCodeBlock } from '@/features/editor/markdown/plate/mermaid-code-block'

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

    expect(await screen.findByText('Edit source')).toBeInTheDocument()
    expect(document.querySelector('[data-testid="rendered-diagram"]')).toBeInTheDocument()

    const source = screen.getByTestId('raw-source')
    expect(source).toBeInTheDocument()
    expect(source.parentElement?.className).toContain('hidden')

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

    const source = screen.getByTestId('raw-source')
    expect(source.parentElement?.className).not.toContain('hidden')
    expect(screen.queryByText('Edit source')).toBeNull()
  })

  it('never sets state (or warns) after unmount while a render is in flight', async () => {
    vi.useFakeTimers()
    const errorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    try {
      const { unmount } = render(
        <MermaidCodeBlock code="flowchart LR\nA --> B">
          <pre data-testid="raw-source">x</pre>
        </MermaidCodeBlock>,
      )
      unmount()
      await vi.advanceTimersByTimeAsync(1000)
      expect(errorSpy).not.toHaveBeenCalled()
    } finally {
      errorSpy.mockRestore()
      vi.useRealTimers()
    }
  })
})
