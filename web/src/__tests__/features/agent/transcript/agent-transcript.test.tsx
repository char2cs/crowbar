import { Profiler, type ComponentProps } from 'react'
import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { delta, ledger, seedFeed } from '@/__tests__/__fixtures__/ledger-rows'
import type { PromptQueueItem } from '@/features/agent/lib/prompt-queue-persistence'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
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
  chatStore.setState({ feeds: {} })
  toolRenders.clear()
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
    return { ...rect, toJSON: () => rect } as DOMRect
  }
})

afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = originalRect
})

function draw(overrides: Partial<ComponentProps<typeof AgentTranscript>> = {}) {
  const props: ComponentProps<typeof AgentTranscript> = {
    chatId: 'chat-1',
    providers: [],
    queue: [],
    status: 'live',
    working: false,
    compacting: false,
    asking: false,
    onOpenTerminal: () => {},
    onEditPrompt: () => {},
    onCancelPrompt: () => {},
    onRetryPrompt: () => {},
    ...overrides,
  }
  const view = render(<AgentTranscript {...props} />)
  return {
    ...view,
    redraw: (next: Partial<ComponentProps<typeof AgentTranscript>>) =>
      view.rerender(<AgentTranscript {...props} {...next} />),
  }
}

const rowOf = (id: string) => document.querySelector(`[data-row-id="${id}"]`)

function queued(clientRequestId: string, text: string): PromptQueueItem {
  return {
    clientRequestId,
    text,
    state: 'submitting',
    createdAt: '2026-10-01T12:00:00Z',
    baselineSequence: 0,
  }
}

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
          queue={[]}
          status="connecting"
          working={false}
          compacting={false}
          asking={false}
          onOpenTerminal={() => {}}
          onEditPrompt={() => {}}
          onCancelPrompt={() => {}}
          onRetryPrompt={() => {}}
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

  // The queue row speaks for a prompt until its turn takes it, so the text is
  // on screen once at every step of the handover.
  it('shows a queued prompt once, then its delivered row', () => {
    const l = ledger()
    seedFeed('chat-1', [l.prompt('r1', 'ship it')])
    const view = draw({ queue: [queued('r1', 'ship it')] })
    expect(screen.getAllByText('ship it')).toHaveLength(1)
    expect(rowOf('p:r1')).toBeNull()

    act(() => seedFeed('chat-1', [l.start('turn-1', 'r1')]))
    view.redraw({ queue: [] })

    expect(screen.getAllByText('ship it')).toHaveLength(1)
    expect(rowOf('p:r1')).not.toBeNull()
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

  it('says it is still trying when the feed cannot be reached', () => {
    openFeed('chat-1')
    draw({ status: 'error' })

    expect(screen.getByText(/couldn’t reach this chat/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /retry/i })).not.toBeInTheDocument()
  })
})
