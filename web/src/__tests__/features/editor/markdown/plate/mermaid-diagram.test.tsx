import { afterEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { MermaidDiagram } from '@/features/editor/markdown/plate/mermaid-diagram'

// Real mermaid on purpose: the leak is mermaid's own error rendering into
// document.body, which a mock cannot reproduce.
const INVALID_FLOWCHART = 'graph TD\n  A-->'
const VALID_FLOWCHART = 'graph TD\n  A-->B'
const SLOW = { timeout: 20_000 }

// jsdom has no SVG layout; mermaid measures text through getBBox.
function stubGetBBox(impl: () => unknown) {
  Object.defineProperty(SVGElement.prototype, 'getBBox', { value: impl, configurable: true })
}

function newBodyChildren(before: Set<Element>) {
  return Array.from(document.body.children).filter((node) => !before.has(node))
}

describe('MermaidDiagram', () => {
  afterEach(() => {
    Reflect.deleteProperty(SVGElement.prototype, 'getBBox')
  })

  it('shows the in-pane error and leaves nothing in document.body when the source is invalid', async () => {
    const onStateChange = vi.fn()
    render(<MermaidDiagram code={INVALID_FLOWCHART} onStateChange={onStateChange} />)
    const before = new Set(Array.from(document.body.children))

    expect(await screen.findByText(/couldn't render this diagram/i, {}, SLOW)).toBeInTheDocument()

    expect(onStateChange).toHaveBeenLastCalledWith('error')
    expect(newBodyChildren(before)).toEqual([])
    expect(document.body.textContent).not.toContain('Syntax error in text')
  })

  it('leaves nothing in document.body when valid source fails during layout', async () => {
    stubGetBBox(() => {
      throw new Error('layout failed')
    })
    render(<MermaidDiagram code={VALID_FLOWCHART} onStateChange={vi.fn()} />)
    const before = new Set(Array.from(document.body.children))

    expect(await screen.findByText(/layout failed/i, {}, SLOW)).toBeInTheDocument()

    expect(newBodyChildren(before)).toEqual([])
    expect(document.body.textContent).not.toContain('Syntax error in text')
  })

  it('renders a valid diagram into its pane', async () => {
    stubGetBBox(() => ({ x: 0, y: 0, width: 40, height: 20 }))
    const onStateChange = vi.fn()
    const { container } = render(
      <MermaidDiagram code={VALID_FLOWCHART} onStateChange={onStateChange} />,
    )
    const before = new Set(Array.from(document.body.children))

    await vi.waitFor(() => expect(onStateChange).toHaveBeenLastCalledWith('success'), SLOW)

    expect(container.querySelector('.mermaid-diagram svg')).not.toBeNull()
    expect(newBodyChildren(before)).toEqual([])
  })
})
