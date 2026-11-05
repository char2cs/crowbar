import { createElement, createRef, useRef } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import {
  AgentEmptyDocument,
  type AgentEmptyDocumentHandle,
  type AgentEmptyDocumentProps,
  lastLineTop,
} from '@/features/agent/chat/agent-empty-document'

type Overrides = Partial<Omit<AgentEmptyDocumentProps, 'docRef' | 'ref'>>

// The composer writes its editor into `.doc`; here the document is empty.
function DocumentHost(props: Overrides & { handle: React.Ref<AgentEmptyDocumentHandle> }) {
  const docRef = useRef<HTMLDivElement>(null)
  const { handle, ...rest } = props
  return (
    <div>
      <div ref={docRef} className="doc" />
      <AgentEmptyDocument
        ref={handle}
        docRef={docRef}
        hasText={false}
        onSubmit={vi.fn()}
        controls={null}
        attach={null}
        working={false}
        canStop={false}
        sending={false}
        onStop={vi.fn()}
        {...rest}
      />
    </div>
  )
}

function draw(overrides: Overrides = {}) {
  const ref = createRef<AgentEmptyDocumentHandle>()
  const view = render(<DocumentHost handle={ref} {...overrides} />)
  return { ...view, ref }
}

describe('AgentEmptyDocument handle', () => {
  it('reports the handle’s own on-screen rect', () => {
    const { ref, container } = draw()

    const handle = container.querySelector('.dochandle') as HTMLElement
    const rect = { top: 84, left: 0, right: 0, bottom: 0, width: 0, height: 0 } as DOMRect
    vi.spyOn(handle, 'getBoundingClientRect').mockReturnValue(rect)

    expect(ref.current?.getHandleRect()).toBe(rect)
  })

  // A caller reading this off an unmounted instance gets "nothing to arrive
  // from", never a thrown error or a stale rect.
  it('reports null once unmounted', () => {
    const { ref, unmount } = draw()
    unmount()

    expect(ref.current).toBeNull()
  })
})

// The handle always sits under the last written line, never the caret's: editing an earlier
// sentence must not drag the send button up the page.
describe('lastLineTop', () => {
  function docWithParagraphs(bottoms: number[], docTop = 0): HTMLDivElement {
    const doc = document.createElement('div')
    vi.spyOn(doc, 'getBoundingClientRect').mockReturnValue({ top: docTop } as DOMRect)
    const editable = document.createElement('div')
    editable.setAttribute('data-slate-editor', 'true')
    for (const bottom of bottoms) {
      const p = document.createElement('p')
      p.textContent = 'text'
      vi.spyOn(p, 'getBoundingClientRect').mockReturnValue({ bottom } as DOMRect)
      editable.appendChild(p)
    }
    doc.appendChild(editable)
    return doc
  }

  it('sits under the LAST paragraph, not the first, when there is more than one', () => {
    expect(lastLineTop(docWithParagraphs([100, 250]))).toBe(250)
  })

  it('is measured relative to the doc, not the viewport', () => {
    expect(lastLineTop(docWithParagraphs([250], 40))).toBe(210)
  })

  it('falls back to the first-line position on a genuinely empty document', () => {
    expect(lastLineTop(docWithParagraphs([]))).toBeCloseTo(48 + 23.8)
  })

  it('falls back to the first-line position when the editor holds no text at all', () => {
    const doc = document.createElement('div')
    vi.spyOn(doc, 'getBoundingClientRect').mockReturnValue({ top: 0 } as DOMRect)
    const editable = document.createElement('div')
    editable.setAttribute('data-slate-editor', 'true')
    editable.appendChild(document.createElement('p')) // present, but empty
    doc.appendChild(editable)

    expect(lastLineTop(doc)).toBeCloseTo(48 + 23.8)
  })

  // The overlay chat header takes no flex space, so an empty document's fallback position adds
  // the same clearance as `.doc`'s top padding (`--agent-header-clearance`).
  it('adds headerClearancePx to the fallback position on a genuinely empty document', () => {
    expect(lastLineTop(docWithParagraphs([]), 52)).toBeCloseTo(48 + 52 + 23.8)
  })

  it('ignores headerClearancePx once there is a real last line to measure', () => {
    expect(lastLineTop(docWithParagraphs([100, 250]), 52)).toBe(250)
  })

  it('ignores where the caret actually is — a selection anchored in an earlier line does not move it', () => {
    const doc = docWithParagraphs([100, 250])
    document.body.appendChild(doc)
    try {
      const editable = doc.querySelector('[data-slate-editor]') as HTMLElement
      const firstParagraph = editable.firstElementChild as HTMLElement
      const range = document.createRange()
      range.selectNodeContents(firstParagraph)
      const selection = window.getSelection()
      selection?.removeAllRanges()
      selection?.addRange(range)

      expect(lastLineTop(doc)).toBe(250)
    } finally {
      document.body.removeChild(doc)
    }
  })
})

describe('AgentEmptyDocument header clearance', () => {
  // An empty `.doc` takes the fresh chat's fallback, through the component's effect.
  it('positions the handle below the overlay header when headerClearancePx is set', () => {
    const { container } = draw({ headerClearancePx: 52 })

    const handle = container.querySelector('.dochandle') as HTMLElement
    // 48 (doc's own top padding) + 52 (header clearance) + 23.8 (one line at
    // 14px/1.7) + 4 (HANDLE_LEAD), rounded — see firstLineTop/HANDLE_LEAD.
    expect(handle.style.transform).toBe('translateY(128px)')
  })

  it('matches the pre-existing placement when no overlay header is present', () => {
    const { container } = draw()

    const handle = container.querySelector('.dochandle') as HTMLElement
    expect(handle.style.transform).toBe('translateY(76px)')
  })
})

// `selectionchange` is a document event that Monaco fires ~120/s during a drag-select; each
// `place()` forces a layout, so the listener must ignore selections outside THIS document.
describe('AgentEmptyDocument selectionchange gating', () => {
  const FALLBACK_TRANSFORM = 'translateY(76px)' // 48 + 23.8 + HANDLE_LEAD, rounded

  /**
   * Give the rendered `.doc` a measurable last line, so a `place()` that runs
   * moves the handle somewhere the fallback never would.
   */
  function giveMeasurableLastLine(container: HTMLElement, bottom: number) {
    const doc = container.querySelector('.doc') as HTMLElement
    vi.spyOn(doc, 'getBoundingClientRect').mockReturnValue({ top: 0 } as DOMRect)
    const editable = document.createElement('div')
    editable.setAttribute('data-slate-editor', 'true')
    const p = document.createElement('p')
    p.textContent = 'a line'
    vi.spyOn(p, 'getBoundingClientRect').mockReturnValue({ bottom } as DOMRect)
    editable.appendChild(p)
    doc.appendChild(editable)
    return doc
  }

  function handleTransform(container: HTMLElement): string {
    return (container.querySelector('.dochandle') as HTMLElement).style.transform
  }

  function selectContentsOf(node: Node) {
    const range = document.createRange()
    range.selectNodeContents(node)
    const selection = window.getSelection()
    selection?.removeAllRanges()
    selection?.addRange(range)
  }

  it('ignores a selectionchange whose selection is outside this document', () => {
    const { container } = draw()
    giveMeasurableLastLine(container, 300)
    expect(handleTransform(container)).toBe(FALLBACK_TRANSFORM)

    // A selection anchored somewhere else entirely — the shape Monaco's hidden
    // textarea produces on every step of a drag-select.
    const outside = document.createElement('div')
    outside.textContent = 'elsewhere'
    document.body.appendChild(outside)
    try {
      selectContentsOf(outside)
      for (let i = 0; i < 20; i++) fireEvent(document, new Event('selectionchange'))

      expect(handleTransform(container)).toBe(FALLBACK_TRANSFORM)
    } finally {
      document.body.removeChild(outside)
      window.getSelection()?.removeAllRanges()
    }
  })

  it('still re-places the handle when the selection is inside this document', () => {
    const { container } = draw()
    const doc = giveMeasurableLastLine(container, 300)
    expect(handleTransform(container)).toBe(FALLBACK_TRANSFORM)

    try {
      selectContentsOf(doc)
      fireEvent(document, new Event('selectionchange'))

      expect(handleTransform(container)).toBe('translateY(304px)') // 300 + HANDLE_LEAD
    } finally {
      window.getSelection()?.removeAllRanges()
    }
  })
})

describe('AgentEmptyDocument stop control', () => {
  // This surface duplicates composer-handle.tsx's send/stop button; Stop must not depend on the
  // document being empty, since a background handoff can start a turn before anything is typed.
  it('stops a running turn even with text already in the document', () => {
    const onStop = vi.fn()
    draw({ hasText: true, working: true, canStop: true, onStop })

    const button = screen.getByRole('button', { name: 'Stop this turn' })
    expect(button).toBeEnabled()
    expect(button.className).toMatch(/\bhalt\b/)
    fireEvent.click(button)
    expect(onStop).toHaveBeenCalled()
  })

  it('sends the document when there is no running turn to stop', () => {
    const onSubmit = vi.fn()
    draw({ hasText: true, onSubmit })

    const button = screen.getByRole('button', { name: 'Send prompt' })
    expect(button).toBeEnabled()
    fireEvent.click(button)
    expect(onSubmit).toHaveBeenCalled()
  })
})

describe('AgentEmptyDocument hasText', () => {
  it('stays disabled while the box is empty', () => {
    draw({ hasText: false })

    const button = screen.getByRole('button', { name: 'Send prompt' })
    expect(button).toBeDisabled()
    expect(button.className).toMatch(/\boff\b/)
  })

  it('enables once the box holds text', () => {
    draw({ hasText: true })

    expect(screen.getByRole('button', { name: 'Send prompt' })).toBeEnabled()
  })
})

// The reviving/idle/trust-wait signpost takes the control row's slot and shares its one
// `place()` transform, so a resize cannot separate the two.
describe('AgentEmptyDocument banner overlay', () => {
  it('renders the banner inside the handle in place of the control row when one is given', () => {
    const { container } = draw({
      banner: createElement('div', { 'data-testid': 'stub-signpost' }, 'Resume the provider'),
      controls: createElement('span', { 'data-testid': 'stub-controls' }),
    })

    const handle = container.querySelector('.dochandle') as HTMLElement
    expect(handle.querySelector('[data-testid="stub-signpost"]')).not.toBeNull()
    expect(handle.querySelector('.grp')).toBeNull()
    expect(handle.querySelector('[data-testid="stub-controls"]')).toBeNull()
    expect(screen.queryByRole('button', { name: 'Send prompt' })).toBeNull()
  })

  it('renders the ordinary control row, unchanged, when no banner is given', () => {
    const { container } = draw({
      controls: createElement('span', { 'data-testid': 'stub-controls' }),
    })

    const handle = container.querySelector('.dochandle') as HTMLElement
    expect(handle.querySelector('.grp')).not.toBeNull()
    expect(handle.querySelector('[data-testid="stub-controls"]')).not.toBeNull()
    expect(screen.getByRole('button', { name: 'Send prompt' })).toBeInTheDocument()
  })

  it('positions the handle the same way whether or not a banner occupies it', () => {
    const withoutBanner = draw()
    const withBanner = draw({
      banner: createElement('div', null, 'Resume the provider'),
    })

    const handleWithout = withoutBanner.container.querySelector('.dochandle') as HTMLElement
    const handleWith = withBanner.container.querySelector('.dochandle') as HTMLElement
    expect(handleWith.style.transform).toBe(handleWithout.style.transform)
  })
})

describe('AgentEmptyDocument sending state', () => {
  it('shows a sending spinner once dispatched but not yet proven delivered, same as the dock composer', () => {
    const { container } = draw({ sending: true })

    const button = screen.getByRole('button', { name: 'Sending' })
    expect(button).toBeDisabled()
    expect(button.className).toMatch(/\boff\b/)
    expect(container.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
  })

  it('prefers the stop control over the sending spinner when both apply', () => {
    draw({ working: true, canStop: true, sending: true })

    expect(screen.getByRole('button', { name: 'Stop this turn' })).toBeInTheDocument()
    expect(screen.queryByRole('status')).toBeNull()
  })
})
