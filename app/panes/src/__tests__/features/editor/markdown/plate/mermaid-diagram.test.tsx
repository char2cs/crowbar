import { afterEach, describe, expect, it, vi } from 'vitest'
import { act, render, screen } from '@testing-library/react'
import {
  MermaidDiagram,
  type MermaidRenderState,
} from '@/features/editor/markdown/plate/mermaid-diagram'

// Real mermaid on purpose: the leak is mermaid's own error rendering into
// document.body, which a mock cannot reproduce.
const INVALID_FLOWCHART = 'graph TD\n  A-->'
const VALID_FLOWCHART = 'graph TD\n  A-->B'
const SLOW = { timeout: 20_000 }

// Collection has no test timeout: load mermaid here, so the component's own
// import resolves from the module cache.
await import('mermaid')

// jsdom has no SVG layout; mermaid measures text through getBBox.
function stubGetBBox(impl: () => unknown) {
  Object.defineProperty(SVGElement.prototype, 'getBBox', { value: impl, configurable: true })
}

function newBodyChildren(before: Set<Element>) {
  return Array.from(document.body.children).filter((node) => !before.has(node))
}

// The render finishes on a timer outside React; awaiting its verdict inside act
// commits the result with it, so no assertion polls for the DOM.
function renderDiagram(code: string) {
  let settle: (state: MermaidRenderState) => void = () => {}
  const settled = new Promise<MermaidRenderState>((resolve) => (settle = resolve))
  const onStateChange = vi.fn((state: MermaidRenderState) => {
    if (state !== 'pending') settle(state)
  })
  const view = render(<MermaidDiagram code={code} onStateChange={onStateChange} />)
  const before = new Set(Array.from(document.body.children))
  return { ...view, before, onStateChange, settle: () => act(() => settled) }
}

describe('MermaidDiagram', () => {
  afterEach(() => {
    Reflect.deleteProperty(SVGElement.prototype, 'getBBox')
  })

  it(
    'shows the in-pane error and leaves nothing in document.body when the source is invalid',
    SLOW,
    async () => {
      const { before, onStateChange, settle } = renderDiagram(INVALID_FLOWCHART)

      expect(await settle()).toBe('error')

      expect(screen.getByText(/couldn't render this diagram/i)).toBeInTheDocument()
      expect(onStateChange).toHaveBeenLastCalledWith('error')
      expect(newBodyChildren(before)).toEqual([])
      expect(document.body.textContent).not.toContain('Syntax error in text')
    },
  )

  it('leaves nothing in document.body when valid source fails during layout', SLOW, async () => {
    stubGetBBox(() => {
      throw new Error('layout failed')
    })
    const { before, settle } = renderDiagram(VALID_FLOWCHART)

    expect(await settle()).toBe('error')

    expect(screen.getByText(/layout failed/i)).toBeInTheDocument()
    expect(newBodyChildren(before)).toEqual([])
    expect(document.body.textContent).not.toContain('Syntax error in text')
  })

  it('renders a valid diagram into its pane', SLOW, async () => {
    stubGetBBox(() => ({ x: 0, y: 0, width: 40, height: 20 }))
    const { container, before, onStateChange, settle } = renderDiagram(VALID_FLOWCHART)

    expect(await settle()).toBe('success')

    expect(onStateChange).toHaveBeenLastCalledWith('success')
    expect(container.querySelector('.mermaid-diagram svg')).not.toBeNull()
    expect(newBodyChildren(before)).toEqual([])
  })
})
