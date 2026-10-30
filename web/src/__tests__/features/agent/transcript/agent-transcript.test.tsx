import { Profiler, type ComponentProps } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { delta, ledger, seedFeed } from '@/__tests__/__fixtures__/ledger-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import type { OutboxItem } from '@/features/agent/stores/lib/outbox'
import { AgentTranscript } from '@/features/agent/transcript/agent-transcript'

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

// jsdom measures every element 0×0, and a zero-height viewport windows down to
// no rows at all; a pane-sized rect makes the window the one the app produces.
const rect = { top: 0, left: 0, right: 768, bottom: 800, width: 768, height: 800, x: 0, y: 0 }
const originalRect = HTMLElement.prototype.getBoundingClientRect

beforeEach(() => {
  chatStore.setState({ feeds: {}, outbox: {} })
  toolRenders.clear()
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return { ...rect, toJSON: () => rect } as DOMRect
  }
})

afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = originalRect
  vi.unstubAllGlobals()
  vi.useRealTimers()
})

function draw(overrides: Partial<ComponentProps<typeof AgentTranscript>> = {}) {
  const props: ComponentProps<typeof AgentTranscript> = {
    chatId: 'chat-1',
    providers: [],
    status: 'live',
    working: false,
    compacting: false,
    asking: false,
    onOpenTerminal: () => {},
    promptActions,
    ...overrides,
  }
  const view = render(<AgentTranscript {...props} />)
  return {
    ...view,
    redraw: (next: Partial<ComponentProps<typeof AgentTranscript>>) =>
      view.rerender(<AgentTranscript {...props} {...next} />),
  }
}

const promptActions = { retry: () => {}, edit: () => {}, cancel: () => {} }

/** Hands the test every ResizeObserver callback by the element it watches. */
function observeResizes() {
  const observed = new Map<Element, ResizeObserverCallback>()
  vi.stubGlobal(
    'ResizeObserver',
    class {
      constructor(private readonly callback: ResizeObserverCallback) {}
      observe(el: Element) {
        observed.set(el, this.callback)
      }
      unobserve() {}
      disconnect() {}
    },
  )
  return observed
}

/** jsdom lays nothing out: the list's metrics are the test's, and scrollTop clamps. */
function layOut(el: HTMLElement, box: { scrollHeight: number; clientHeight: number }) {
  let top = 0
  Object.defineProperty(el, 'scrollTop', {
    configurable: true,
    get: () => top,
    set: (v: number) => {
      top = Math.max(0, Math.min(v, box.scrollHeight - box.clientHeight))
    },
  })
  Object.defineProperty(el, 'scrollHeight', { configurable: true, get: () => box.scrollHeight })
  Object.defineProperty(el, 'clientHeight', { configurable: true, get: () => box.clientHeight })
  // The virtualizer writes through scrollTo, which jsdom leaves out.
  el.scrollTo = ((options: ScrollToOptions) => {
    el.scrollTop = options.top ?? top
  }) as typeof el.scrollTo
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
    render(
      <Profiler id="transcript" onRender={() => void (commits += 1)}>
        <AgentTranscript
          chatId="chat-1"
          providers={[]}
          status="connecting"
          working={false}
          compacting={false}
          asking={false}
          onOpenTerminal={() => {}}
          promptActions={promptActions}
        />
      </Profiler>,
    )
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
    const box = { scrollHeight: 2000, clientHeight: 800 }
    layOut(screen.getByTestId('agent-message-list'), box)
    const list = screen.getByTestId('agent-message-list')

    act(() => applyFeedFrame('chat-1', { type: 'delta', delta: delta('m1', 1, 'Found ') }))
    expect(list.scrollTop).toBe(1200)

    // A streaming row's growth reaches the list as a virtualizer re-render.
    box.scrollHeight = 2300
    redraw({ since: '2026-10-01T12:00:00Z' })
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
    const list = screen.getByTestId('agent-message-list')
    const box = { scrollHeight: 2000, clientHeight: 800 }
    layOut(list, box)
    redraw({ since: 'a' })
    box.scrollHeight = 2300
    redraw({ since: 'b' })
    act(() => vi.advanceTimersByTime(16))
    fireEvent.scroll(list)

    // The first row, far above the view, grows; the virtualizer moves the view by as much.
    const first = list.querySelector<HTMLElement>('[data-index="0"]')!
    first.getBoundingClientRect = () => ({ ...rect, height: 900, toJSON: () => rect })
    box.scrollHeight = 2400
    act(() => observed.get(first)?.([{ target: first } as never], {} as ResizeObserver))
    fireEvent.scroll(list)
    box.scrollHeight = 2600
    redraw({ since: 'c' })
    act(() => vi.advanceTimersByTime(1500))

    expect(list.scrollTop).toBe(1800)
  })

  it('lands on the newest row when its parked workspace comes back', () => {
    const observed = observeResizes()
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'look')])
    draw()
    const list = screen.getByTestId('agent-message-list')
    const box = { scrollHeight: 2000, clientHeight: 0 }
    layOut(list, box)
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
    const list = screen.getByTestId('agent-message-list')
    const box = { scrollHeight: 3000, clientHeight: 800 }
    layOut(list, box)
    redraw({ since: 'a' })
    list.scrollTop = 500
    fireEvent.scroll(list)

    act(() => chatStore.setState({ outbox: { 'chat-1': [outboxItem('r2')] } }))
    act(() => chatStore.setState({ outbox: { 'chat-1': [] } }))
    box.scrollHeight = 3400
    redraw({ since: 'b' })

    expect(list.scrollTop).toBe(500)
  })

  it('says it is still trying when the feed cannot be reached', () => {
    openFeed('chat-1')
    draw({ status: 'error' })

    expect(screen.getByText(/couldn’t reach this chat/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /retry/i })).not.toBeInTheDocument()
  })
})
