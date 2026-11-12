import { Profiler, type ComponentProps, type ReactNode } from 'react'
import { act, fireEvent, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { delta, ledger, seedFeed } from '@/__tests__/__fixtures__/ledger-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'
import {
  layOut,
  measureRows,
  observeResizes,
  rect,
} from '@/__tests__/__fixtures__/transcript-layout'
import { drawTranscript as draw } from '@/__tests__/__fixtures__/agent-transcript'

const { toolRenders } = vi.hoisted(() => ({ toolRenders: new Map<string, number>() }))
vi.mock('@/features/agent/transcript/rows/tool-row', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/agent/transcript/rows/tool-row')>()
  const { memo } = await import('react')
  type Props = ComponentProps<typeof actual.ToolRow>
  const Counted = (props: Props) => {
    toolRenders.set(props.item.id, (toolRenders.get(props.item.id) ?? 0) + 1)
    return <div data-row-id={props.item.id} data-testid="tool-row" />
  }
  return { ...actual, ToolRow: memo(Counted) }
})

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))

const originalRect = HTMLElement.prototype.getBoundingClientRect
let restoreRows = () => {}

beforeEach(() => {
  chatStore.setState({ feeds: {}, outbox: {} })
  toolRenders.clear()
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return { ...rect, toJSON: () => rect } as DOMRect
  }
})

afterEach(() => {
  platform.mobile = false
  restoreRows()
  restoreRows = () => {}
  HTMLElement.prototype.getBoundingClientRect = originalRect
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

/** The message list laid out as a box this tall and that high on screen. */
function layList(scrollHeight: number, clientHeight: number) {
  const list = screen.getByTestId('agent-message-list')
  const box = { scrollHeight, clientHeight }
  layOut(list, box)
  return { list, box }
}

const seedPrompts = (n: number) => {
  const l = ledger()
  seedFeed(
    'chat-1',
    Array.from({ length: n }, (_, i) => l.prompt(`r${i}`, `prompt ${i}`)),
  )
}

const outboxItem = (requestId: string): OutboxItem => ({
  requestId,
  wsId: 'ws-1',
  text: `prompt ${requestId}`,
  state: 'sending',
  steer: false,
})

describe('AgentTranscript', () => {
  it('symptom 7: a delta re-renders only its streaming row', () => {
    const l = ledger()
    seedFeed('chat-1', [
      l.prompt('r1', 'look'),
      l.start('turn-1', 'r1'),
      l.tool('turn-1', 'tool-1', 'pre'),
      l.tool('turn-1', 'tool-1', 'post'),
    ])
    draw({ working: true })
    expect(toolRenders.get('t:tool-1')).toBe(1)

    act(() => {
      applyFeedFrame('chat-1', { type: 'delta', delta: delta('m1', 1, 'Found ') })
      applyFeedFrame('chat-1', { type: 'delta', delta: delta('m1', 2, 'it.') })
    })

    expect(screen.getByText('Found it.')).toBeInTheDocument()
    expect(toolRenders.get('t:tool-1')).toBe(1)
  })

  it('folds 5000 replayed rows into one order without re-rendering per row', () => {
    openFeed('chat-1')
    let commits = 0
    const counted = ({ children }: { children: ReactNode }) => (
      <Profiler id="transcript" onRender={() => void (commits += 1)}>
        {children}
      </Profiler>
    )
    draw({ status: 'connecting' }, { wrapper: counted })
    const l = ledger()
    const rows = Array.from({ length: 2500 }, (_, i) => [
      l.prompt(`r${i}`, `prompt ${i}`),
      l.start(`turn-${i}`, `r${i}`),
    ]).flat()
    commits = 0

    act(() => {
      for (const row of rows) applyFeedFrame('chat-1', { type: 'row', row })
      applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
    })

    expect(chatStore.getState().feeds['chat-1']?.order).toHaveLength(2500)
    expect(commits).toBeGreaterThan(0)
    expect(commits).toBeLessThanOrEqual(3)
  })

  it('draws a marker the record carries as its pill', () => {
    const l = ledger()
    seedFeed('chat-1', [
      l.prompt('r1', 'go'),
      l.start('turn-1', 'r1'),
      l.row('marker', { kind: 'compaction_ended' }, { turnId: 'turn-1' }),
      l.end('turn-1', 'interrupted'),
    ])
    draw()
    expect(screen.getByTestId('agent-compaction-divider')).toHaveTextContent('Compacted')
    expect(screen.getByTestId('agent-interrupted-divider')).toHaveTextContent('Interrupted')
  })

  it('marks the first turn only on a feed that holds the chat from its start', () => {
    seedFeed('chat-1', [ledger().prompt('r1', 'ask')])
    draw()
    expect(screen.getByTestId('agent-first-turn-divider')).toBeInTheDocument()
    applyFeedFrame('c2', { type: 'row', row: ledger('c2', 41).prompt('r1', 'ask'), from: 40 })
    seedFeed('c2')
    const { container } = draw({ chatId: 'c2' })
    expect(container.querySelector('[data-testid="agent-first-turn-divider"]')).toBeNull()
  })

  it('keeps the turn actions of earlier turns while a new one runs', () => {
    const l = ledger()
    seedFeed('chat-1', [
      ...[1, 2].flatMap((n) => [
        l.prompt(`r${n}`, `ask ${n}`),
        l.start(`turn-${n}`, `r${n}`),
        l.text(`turn-${n}`, `m${n}`, 0, `answer ${n}`, true),
        l.end(`turn-${n}`),
      ]),
      l.prompt('r3', 'ask 3'),
      l.start('turn-3', 'r3'),
    ])
    draw({ working: true })
    expect(screen.getAllByTestId('message-turn-actions')).toHaveLength(2)
  })

  it('draws no turn actions above a call the turn ran after its last words', () => {
    const l = ledger()
    seedFeed('chat-1', [
      l.prompt('r1', 'touch it'),
      l.start('turn-1', 'r1'),
      l.text('turn-1', 'm1', 0, 'I will run it.', true),
      l.tool('turn-1', 'c1', 'pre', 'Bash'),
      l.tool('turn-1', 'c1', 'post', 'Bash'),
      l.end('turn-1'),
    ])
    draw()
    expect(screen.queryByTestId('message-turn-actions')).toBeNull()
  })

  it('symptom 6: rows are visible on the first render', () => {
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'first'), l.start('turn-1', 'r1'), l.prompt('r2', 'last')])
    draw()
    expect(screen.getByText('first')).toBeVisible()
    expect(screen.getByText('last')).toBeVisible()
  })

  it('pins a sent prompt to the top with a floor from the virtualizer offsets', () => {
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'one'), l.start('turn-1', 'r1'), l.end('turn-1')])
    const { container } = draw({ headerClearancePx: 52 })
    const stream = container.querySelector<HTMLElement>('.stream')!
    expect(stream.style.minHeight).toBe('')

    act(() => chatStore.setState({ outbox: { 'chat-1': [outboxItem('r2')] } }))

    // Rows measure 800 each (the stubbed rect), so the new prompt, second, starts
    // at 800; the floor leaves a viewport below it, less the header it must clear.
    expect(stream.style.minHeight).toBe(`${800 + 800 - 52}px`)
  })

  it('does not lift a sent prompt to the top on a phone', () => {
    platform.mobile = true
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'one'), l.start('turn-1', 'r1'), l.end('turn-1')])
    const { container } = draw()
    const stream = container.querySelector<HTMLElement>('.stream')!

    act(() => chatStore.setState({ outbox: { 'chat-1': [outboxItem('r2')] } }))

    expect(stream.style.minHeight).toBe('')
  })

  it('does not pin a prompt queued behind a running turn until it is sent', () => {
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'one'), l.start('turn-1', 'r1')])
    const { container, redraw } = draw({ working: true })
    const stream = container.querySelector<HTMLElement>('.stream')!

    act(() => chatStore.setState({ outbox: { 'chat-1': [outboxItem('r2')] } }))
    expect(stream.style.minHeight).toBe('')

    redraw({ working: false })
    act(() => seedFeed('chat-1', [l.end('turn-1'), l.prompt('r2', 'two')]))
    act(() => chatStore.setState({ outbox: { 'chat-1': [] } }))
    expect(stream.style.minHeight).toBe(`${800 + 800}px`)
  })

  it('pins a queued prompt only once it leaves the queue', () => {
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'one'), l.start('turn-1', 'r1'), l.end('turn-1')])
    const { container } = draw()
    const stream = container.querySelector<HTMLElement>('.stream')!

    const held = { ...outboxItem('r2'), state: 'queued' as const }
    act(() => chatStore.setState({ outbox: { 'chat-1': [held] } }))
    expect(stream.style.minHeight).toBe('')

    act(() => chatStore.setState({ outbox: { 'chat-1': [{ ...held, state: 'sending' }] } }))
    expect(stream.style.minHeight).toBe(`${800 + 800}px`)
  })

  it('draws no row for a queued or failed prompt', () => {
    seedFeed('chat-1', [ledger().prompt('r1', 'one')])
    draw()
    const unsent = (state: 'queued' | 'failed') => ({ ...outboxItem(`x-${state}`), state })
    act(() => chatStore.setState({ outbox: { 'chat-1': [unsent('queued'), unsent('failed')] } }))
    expect(document.querySelector('[data-row-id="p:x-queued"]')).toBeNull()
    expect(document.querySelector('[data-row-id="p:x-failed"]')).toBeNull()
  })

  it('releases the pin of a prompt taken back before its row landed', () => {
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'one')])
    const { container } = draw()
    const stream = container.querySelector<HTMLElement>('.stream')!
    act(() => chatStore.setState({ outbox: { 'chat-1': [outboxItem('r2')] } }))
    expect(stream.style.minHeight).not.toBe('')

    act(() => chatStore.setState({ outbox: { 'chat-1': [] } }))
    expect(stream.style.minHeight).toBe('')
  })

  it('lands the opening, then eases the growth a later commit brings', () => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame', 'performance'] })
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'look'), l.start('turn-1', 'r1')])
    const { redraw } = draw({ working: true })
    const { list, box } = layList(2000, 800)

    act(() => applyFeedFrame('chat-1', { type: 'delta', delta: delta('m1', 1, 'Found ') }))
    expect(list.scrollTop).toBe(1200)

    // A streaming row's growth reaches the list as a virtualizer re-render.
    box.scrollHeight = 2300
    redraw({ tail: '2026-10-01T12:00:00Z' })
    expect(list.scrollTop).toBe(1200)
    act(() => vi.advanceTimersByTime(1500))
    expect(list.scrollTop).toBe(1500)
  })

  it('keeps following through the virtualizer holding rows above still', () => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame', 'performance'] })
    const observed = observeResizes()
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'look'), l.prompt('r2', 'again')])
    const { redraw } = draw()
    const { list, box } = layList(2000, 800)
    redraw({ tail: 'a' })
    box.scrollHeight = 2300
    redraw({ tail: 'b' })
    act(() => vi.advanceTimersByTime(16))
    fireEvent.scroll(list)

    // The first row, far above the view, grows; the virtualizer moves the view by as much.
    const first = list.querySelector<HTMLElement>('[data-index="0"]')!
    first.getBoundingClientRect = () => ({ ...rect, height: 900, toJSON: () => rect })
    box.scrollHeight = 2400
    act(() => observed.get(first)?.([{ target: first } as never], {} as ResizeObserver))
    fireEvent.scroll(list)
    box.scrollHeight = 2600
    redraw({ tail: 'c' })
    act(() => vi.advanceTimersByTime(1500))

    expect(list.scrollTop).toBe(1800)
  })

  it('leaves a held bottom still while a row above it shrinks, until the list commits', () => {
    const observed = observeResizes()
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'look'), l.prompt('r2', 'again'), l.prompt('r3', 'more')])
    const { redraw } = draw()
    const { list } = layList(2600, 800)
    redraw({ tail: 'a' })
    expect(list.scrollTop).toBe(1800)

    // Its report comes between layout and paint; the re-render it asks for comes after.
    const first = list.querySelector<HTMLElement>('[data-index="0"]')!
    first.getBoundingClientRect = () => ({ ...rect, height: 300, toJSON: () => rect })
    observed.get(first)?.([{ target: first } as never], {} as ResizeObserver)
    expect(list.scrollTop).toBe(1800)
  })

  it('leaves a reader be while the row they are reading grows below the top', () => {
    const observed = observeResizes()
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'look'), l.prompt('r2', 'again'), l.prompt('r3', 'more')])
    const { redraw } = draw()
    const { list } = layList(2700, 800)
    redraw({ tail: 'a' })
    list.scrollTop = 1700
    fireEvent.scroll(list)

    const last = list.querySelector<HTMLElement>('[data-index="2"]')!
    last.getBoundingClientRect = () => ({ ...rect, height: 900, toJSON: () => rect })
    act(() => observed.get(last)?.([{ target: last } as never], {} as ResizeObserver))
    expect(list.scrollTop).toBe(1700)
  })

  it('lands on the newest row when its parked workspace comes back', () => {
    const observed = observeResizes()
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'look')])
    draw()
    const { list, box } = layList(2000, 0)
    const resize = (width: number, height: number) =>
      act(() =>
        observed.get(list)?.(
          [{ borderBoxSize: [{ inlineSize: width, blockSize: height }] } as never],
          {} as ResizeObserver,
        ),
      )

    resize(0, 0)
    box.clientHeight = 800
    resize(768, 800)
    expect(list.scrollTop).toBe(1200)
  })

  it('leaves a reader in history where they were when a queued prompt is taken back', () => {
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'one'), l.start('turn-1', 'r1')])
    const { redraw } = draw({ working: true })
    const { list, box } = layList(3000, 800)
    redraw({ tail: 'a' })
    list.scrollTop = 500
    fireEvent.scroll(list)

    act(() => chatStore.setState({ outbox: { 'chat-1': [outboxItem('r2')] } }))
    act(() => chatStore.setState({ outbox: { 'chat-1': [] } }))
    box.scrollHeight = 3400
    redraw({ tail: 'b' })

    expect(list.scrollTop).toBe(500)
  })

  it('offers a reader up the page a way back to the newest rows', () => {
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'one'), l.start('turn-1', 'r1')])
    const { redraw } = draw({ working: true })
    const { list, box } = layList(3000, 800)
    redraw({ tail: 'a' })
    expect(screen.queryByRole('button', { name: 'Jump to latest' })).toBeNull()
    list.scrollTop = 500
    fireEvent.scroll(list)
    box.scrollHeight = 3400
    redraw({ tail: 'b' })

    fireEvent.click(screen.getByRole('button', { name: 'Jump to latest' }))
    expect(list.scrollTop).toBe(2600)
    expect(screen.queryByRole('button', { name: 'Jump to latest' })).toBeNull()
  })

  it('keeps the pin while its turn runs and releases it to the rows when it ends', () => {
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'one')])
    const { container, redraw } = draw({ dockHeight: 92 })
    const stream = container.querySelector<HTMLElement>('.stream')!
    act(() => chatStore.setState({ outbox: { 'chat-1': [outboxItem('r2')] } }))
    act(() => {
      seedFeed('chat-1', [l.prompt('r2', 'two'), l.start('turn-2', 'r2')])
      chatStore.setState({ outbox: { 'chat-1': [] } })
    })
    redraw({ dockHeight: 92, working: true })
    expect(stream.style.minHeight).toBe(`${800 + 800}px`)
    expect(stream.dataset.pin).toBeUndefined()

    act(() => seedFeed('chat-1', [l.end('turn-2')]))
    redraw({ dockHeight: 92, working: false })
    // Two rows of 800, the gap above the dock's room, and that room (dock + 32).
    expect(stream.style.minHeight).toBe(`${1600 + 18 + 92 + 32}px`)
    expect(stream.dataset.pin).toBe('released')
  })

  it('keeps the pin for a reader up the page until they come back down', () => {
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'one')])
    const { container, redraw } = draw({ dockHeight: 92 })
    const stream = container.querySelector<HTMLElement>('.stream')!
    const { list } = layList(3200, 800)
    act(() => chatStore.setState({ outbox: { 'chat-1': [outboxItem('r2')] } }))
    act(() => {
      seedFeed('chat-1', [l.prompt('r2', 'two'), l.start('turn-2', 'r2')])
      chatStore.setState({ outbox: { 'chat-1': [] } })
    })
    redraw({ dockHeight: 92, working: true })
    list.scrollTop = 2400 - 250
    fireEvent.scroll(list)

    act(() => seedFeed('chat-1', [l.end('turn-2')]))
    redraw({ dockHeight: 92, working: false })
    expect(stream.style.minHeight).toBe(`${800 + 800}px`)
    expect(list.scrollTop).toBe(2150)

    act(() => {
      list.scrollTop = 2400
      fireEvent.scroll(list)
    })
    expect(stream.dataset.pin).toBe('released')
    expect(stream.style.minHeight).toBe(`${1600 + 18 + 92 + 32}px`)
  })

  it("lands the opening's correction of estimated row heights", () => {
    // Rows measure 150 (estimates are 96); the list is its rows plus 200 of chrome.
    restoreRows = measureRows({ row: 150, viewport: 200 })
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'one'), l.prompt('r2', 'two')])
    draw()
    expect(screen.getByTestId('agent-message-list').scrollTop).toBe(2 * 150)
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

  it('lands a change to an idle chat in one write', () => {
    vi.useFakeTimers({ toFake: ['requestAnimationFrame', 'cancelAnimationFrame', 'performance'] })
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'look'), l.start('turn-1', 'r1'), l.end('turn-1')])
    const { redraw } = draw({ dockHeight: 0 })
    const { list, box } = layList(2000, 800)
    redraw({ tail: 'a' })
    expect(list.scrollTop).toBe(1200)

    // The dock measures after the opening, and its room grows the list.
    box.scrollHeight = 2086
    redraw({ dockHeight: 86 })
    expect(list.scrollTop).toBe(1286)
  })
})
