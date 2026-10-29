import { act, render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { MermaidRenderState } from '@/features/editor/markdown/plate/mermaid-diagram'
import { DiagramBlock } from '@/features/agent/composer/plate/attachments/diagram-block'

let report: ((state: MermaidRenderState) => void) | undefined
vi.mock('@/features/editor/markdown/plate/mermaid-diagram', () => ({
  MermaidDiagram: ({ onStateChange }: { onStateChange: (s: MermaidRenderState) => void }) => {
    report = onStateChange
    return <svg data-testid="mermaid-svg" />
  },
}))

const animate = vi.fn()
const code = <pre>graph TD</pre>

function block() {
  return document.querySelector<HTMLElement>('.chat-diagram-block')!
}

beforeEach(() => {
  report = undefined
  animate.mockClear()
  Object.defineProperty(HTMLElement.prototype, 'animate', { value: animate, configurable: true })
  vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
    height: 300,
  } as DOMRect)
})

afterEach(() => {
  vi.restoreAllMocks()
  delete (HTMLElement.prototype as { animate?: unknown }).animate
})

describe('DiagramBlock', () => {
  it('keeps the block element and animates its height from skeleton to diagram', () => {
    const { rerender } = render(
      <DiagramBlock kind="excalidraw" target="skeleton" source="{" code={code} />,
    )
    const before = block()
    expect(before.dataset.state).toBe('skeleton')
    rerender(
      <DiagramBlock
        kind="excalidraw"
        target="diagram"
        source="{}"
        preview={<div data-testid="scene" />}
        code={code}
      />,
    )
    expect(block()).toBe(before)
    expect(before.dataset.state).toBe('diagram')
    expect(animate).toHaveBeenCalledTimes(1)
    expect(animate.mock.calls[0]![0]).toEqual([{ height: '160px' }, { height: '300px' }])
  })

  it('keeps the skeleton until MermaidDiagram reports success or error', () => {
    const { rerender } = render(
      <DiagramBlock kind="mermaid" target="skeleton" source="graph" code={code} />,
    )
    rerender(<DiagramBlock kind="mermaid" target="diagram" source="graph TD" code={code} />)
    expect(block().dataset.state).toBe('skeleton')
    act(() => report!('pending'))
    expect(block().dataset.state).toBe('skeleton')
    expect(animate).not.toHaveBeenCalled()
    act(() => report!('success'))
    expect(block().dataset.state).toBe('diagram')
    expect(animate).toHaveBeenCalledTimes(1)
  })

  it('animates the skeleton-to-code fallback the same way', () => {
    const { rerender } = render(
      <DiagramBlock kind="mermaid" target="skeleton" source="graph" code={code} />,
    )
    rerender(<DiagramBlock kind="mermaid" target="code" source="graph" code={code} />)
    expect(block().dataset.state).toBe('code')
    expect(document.querySelector('[data-slot="skeleton"]')).toBeNull()
    expect(animate.mock.calls[0]![0]).toEqual([{ height: '160px' }, { height: '300px' }])
  })

  it('does not animate under reduced motion', () => {
    vi.spyOn(window, 'matchMedia').mockReturnValue({ matches: true } as MediaQueryList)
    const { rerender } = render(
      <DiagramBlock kind="mermaid" target="skeleton" source="graph" code={code} />,
    )
    rerender(<DiagramBlock kind="mermaid" target="code" source="graph" code={code} />)
    expect(block().dataset.state).toBe('code')
    expect(animate).not.toHaveBeenCalled()
  })
})
