import { screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ledger, seedFeed } from '@/__tests__/__fixtures__/ledger-rows'
import { chatStore } from '@/features/agent/stores/chat-store'
import { measureRows, observeResizes } from '@/__tests__/__fixtures__/transcript-layout'
import { drawTranscript as draw } from '@/__tests__/__fixtures__/agent-transcript'

let restoreRows = () => {}
beforeEach(() => chatStore.setState({ feeds: {}, outbox: {} }))
afterEach(() => {
  restoreRows()
  vi.unstubAllGlobals()
})

const seedPrompts = (n: number) => {
  const l = ledger()
  const rows = Array.from({ length: n }, (_, i) => l.prompt(`r${i}`, `prompt ${i}`))
  seedFeed('chat-1', rows)
}

describe('AgentTranscript row layout', () => {
  it('stacks notice cards 10px apart and the row after them 18px below', () => {
    const l = ledger()
    const warn = (detail: string) => l.row('marker', { kind: 'warning', detail })
    seedFeed('chat-1', [warn('One. A'), warn('Two. B'), l.prompt('r1', 'hi')])
    restoreRows = measureRows({ row: 60, viewport: () => 600 })
    draw()
    const list = screen.getByTestId('agent-message-list')
    const rows = [...list.querySelectorAll<HTMLElement>('[data-index]')]
    expect(rows.slice(0, 2).map((row) => row.style.paddingBottom)).toEqual(['10px', '18px'])
  })

  it('keeps a viewport of short rows drawn past the top edge of the view', () => {
    // A fling outruns a re-render: the rows must already stand where it lands.
    restoreRows = measureRows({ row: 30, viewport: 800 })
    seedPrompts(300)
    draw()
    const drawn = screen.getByTestId('agent-message-list').querySelectorAll('[data-index]').length
    // 27 rows in view; 12 of overscan alone drew 39. The rows above the window are still
    // estimated at 96, so its reach counts them tall and draws fewer than the 54 it will settle on.
    expect(drawn).toBeGreaterThanOrEqual(45)
  })

  it('draws the newest rows on the commit that lands the opening', () => {
    // jsdom fires no scroll event for a write, as an engine fires it only a frame later.
    restoreRows = measureRows({ row: 150, viewport: 600 })
    seedPrompts(40)
    draw()
    const list = screen.getByTestId('agent-message-list')
    // Rows above the window keep their estimates: the bottom is the list's own.
    expect(list.scrollTop).toBe(list.scrollHeight - 600)
    expect(screen.getByText('prompt 39')).toBeInTheDocument()
  })

  it('draws the newest rows when a chat opened parked gets its box', () => {
    const observed = observeResizes()
    const box = { viewport: 0 }
    restoreRows = measureRows({ row: 150, viewport: () => box.viewport })
    seedPrompts(40)
    draw()
    const list = screen.getByTestId('agent-message-list')
    box.viewport = 600
    // Not in act: the box's report comes between layout and paint, outside React.
    observed.get(list)?.(
      [{ borderBoxSize: [{ inlineSize: 768, blockSize: 600 }] } as never],
      {} as ResizeObserver,
    )
    expect(list.scrollTop).toBe(list.scrollHeight - 600)
    expect(screen.getByText('prompt 39')).toBeInTheDocument()
  })
})
