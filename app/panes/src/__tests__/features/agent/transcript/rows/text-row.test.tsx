import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { TranscriptRow } from '@/features/agent/transcript/rows/transcript-row'

import { delta, ledger } from '@/__tests__/__fixtures__/ledger-rows'
import { noRowActions } from '@/__tests__/__fixtures__/row-actions'

const actions = noRowActions

// One message whose text the renderer cannot take, standing in for any renderer crash.
vi.mock('@/features/agent/transcript/plate/markdown-message-static', async (importOriginal) => {
  const real =
    await importOriginal<
      typeof import('@/features/agent/transcript/plate/markdown-message-static')
    >()
  return {
    ...real,
    MarkdownMessageStatic: (props: Parameters<typeof real.MarkdownMessageStatic>[0]) => {
      if (String(props.children).includes('BOOM')) throw new Error('cannot render')
      return real.MarkdownMessageStatic(props)
    },
  }
})

const providers = [{ id: 'fake', displayName: 'Fake', icon: '<svg/>' }] as AgentProvider[]
const ID = 'm:turn-1:m1'

function seed(rows: LedgerRow[]) {
  openFeed('chat-1')
  for (const row of rows) applyFeedFrame('chat-1', { type: 'row', row })
  applyFeedFrame('chat-1', { type: 'ready', epoch: 'e1' })
}

const push = (row: LedgerRow) => act(() => void applyFeedFrame('chat-1', { type: 'row', row }))
const prose = () => document.querySelector(`[data-row-id="${ID}"] .agent-prose`)

beforeEach(() => chatStore.setState({ feeds: {} }))

describe('TextRow', () => {
  it('keeps the live renderer mounted when a streamed reply settles', async () => {
    const l = ledger()
    seed([l.start('turn-1'), l.text('turn-1', 'm1', 0, 'Hello')])
    render(
      <TranscriptRow
        actions={actions}
        chatId="chat-1"
        id={ID}
        providers={providers}
        place={{ turnbar: true }}
      />,
    )
    const before = prose()
    expect(before?.getAttribute('contenteditable')).toBe('false')
    expect(screen.queryByTestId('message-turn-actions')).toBeNull()
    push(l.text('turn-1', 'm1', 1, ' world', true))
    expect(prose()).toBe(before)
    await waitFor(() => expect(prose()?.textContent).toContain('Hello world'))
    expect(screen.getByTestId('message-turn-actions')).toBeInTheDocument()
  })

  it('renders a reply that was settled at mount statically', () => {
    const l = ledger()
    seed([
      l.start('turn-1'),
      l.text('turn-1', 'm1', -1, 'the **descriptor** decides'),
      l.end('turn-1'),
    ])
    render(
      <TranscriptRow
        actions={actions}
        chatId="chat-1"
        id={ID}
        providers={providers}
        place={{ turnbar: true }}
      />,
    )
    expect(prose()?.hasAttribute('contenteditable')).toBe(false)
    expect(screen.getByText('descriptor').closest('strong')).not.toBeNull()
  })

  it('draws a reply’s line breaks the same streamed as replayed', async () => {
    const l = ledger()
    const head = [l.start('turn-1'), l.text('turn-1', 'm1', 0, 'one\n')]
    const tail = [l.text('turn-1', 'm1', 1, 'two', true), l.end('turn-1')]
    const whiteSpace = () => getComputedStyle(prose()!).whiteSpace
    seed(head)
    const live = render(
      <TranscriptRow actions={actions} chatId="chat-1" id={ID} providers={providers} />,
    )
    for (const row of tail) push(row)
    await waitFor(() => expect(prose()!.textContent).toBe('one\ntwo'))
    const streamed = [prose()!.textContent, whiteSpace()]
    live.unmount()
    chatStore.setState({ feeds: {} })
    seed([...head, ...tail])
    render(<TranscriptRow actions={actions} chatId="chat-1" id={ID} providers={providers} />)
    expect([prose()!.textContent, whiteSpace()]).toEqual(streamed)
    expect(streamed[1]).toBe('pre-wrap')
  })

  it('follows the live stream ahead of its rows while unsettled', async () => {
    const l = ledger()
    seed([l.start('turn-1'), l.text('turn-1', 'm1', 0, 'Hel')])
    render(<TranscriptRow actions={actions} chatId="chat-1" id={ID} providers={providers} />)
    act(() => void applyFeedFrame('chat-1', { type: 'delta', delta: delta('m1', 1, 'Hello the') }))
    await waitFor(() => expect(prose()?.textContent).toContain('Hello the'))
  })

  it('names the model the turn ran on beside its effort', () => {
    const l = ledger()
    seed([
      l.row('marker', { kind: 'model_changed', detail: 'model-luna' }),
      l.start('turn-1'),
      l.text('turn-1', 'm1', -1, 'done'),
      l.row('turn_ended', { status: 'done', effort: 'low' }, { turnId: 'turn-1' }),
    ])
    render(
      <TranscriptRow
        actions={actions}
        chatId="chat-1"
        id={ID}
        providers={providers}
        place={{ turnbar: true }}
      />,
    )
    expect(screen.getByTestId('message-model')).toHaveTextContent('model-luna')
    expect(screen.getByTestId('message-effort')).toHaveTextContent('low')
  })

  it('shows the turn effort and time only on the reply that hands control back', () => {
    const l = ledger()
    seed([
      l.start('turn-1'),
      l.text('turn-1', 'm1', -1, 'done'),
      l.row('turn_ended', { status: 'done', effort: 'high' }, { turnId: 'turn-1' }),
    ])
    const { rerender } = render(
      <TranscriptRow
        actions={actions}
        chatId="chat-1"
        id={ID}
        providers={providers}
        place={{ turnbar: true }}
      />,
    )
    expect(screen.getByTestId('message-effort')).toHaveTextContent('high')
    rerender(
      <TranscriptRow
        actions={actions}
        chatId="chat-1"
        id={ID}
        providers={providers}
        place={{ turnbar: false }}
      />,
    )
    expect(screen.queryByTestId('message-turn-actions')).toBeNull()
  })

  it('holds the turn actions back until the reply settles', () => {
    const l = ledger()
    seed([l.start('turn-1'), l.text('turn-1', 'm1', 0, 'still going')])
    render(
      <TranscriptRow
        actions={actions}
        chatId="chat-1"
        id={ID}
        providers={providers}
        place={{ turnbar: true }}
      />,
    )
    expect(screen.queryByTestId('message-turn-actions')).toBeNull()
    push(l.end('turn-1'))
    expect(screen.getByTestId('message-turn-actions')).toBeInTheDocument()
  })

  it('keeps the footer to what a reader wants: no provider glyph, duration or clock text', () => {
    const l = ledger()
    seed([
      l.start('turn-1'),
      l.text('turn-1', 'm1', -1, 'done'),
      l.row('turn_ended', { status: 'done', effort: 'low' }, { turnId: 'turn-1' }),
    ])
    render(
      <TranscriptRow
        actions={actions}
        chatId="chat-1"
        id={ID}
        providers={providers}
        place={{ turnbar: true }}
      />,
    )
    const bar = screen.getByTestId('message-turn-actions')
    expect(bar.querySelector('svg:not(button svg)')).toBeNull()
    expect(bar.textContent).not.toMatch(/\d+[mhd] ·/)
    expect(screen.getByRole('button', { name: 'Copy this message' })).toBeInTheDocument()
  })

  it('confines a message the renderer cannot draw to a plain-text fallback', () => {
    const l = ledger()
    seed([
      l.start('turn-1'),
      l.text('turn-1', 'm1', -1, 'BOOM **raw**'),
      l.text('turn-1', 'm2', -1, 'still fine'),
      l.end('turn-1'),
    ])
    const quiet = vi.spyOn(console, 'error').mockImplementation(() => {})
    render(
      <>
        <TranscriptRow actions={actions} chatId="chat-1" id={ID} providers={providers} />
        <TranscriptRow actions={actions} chatId="chat-1" id="m:turn-1:m2" providers={providers} />
      </>,
    )
    quiet.mockRestore()
    expect(screen.getByText('This message could not be displayed.')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Show as plain text' }))
    expect(screen.getByText('BOOM **raw**')).toBeInTheDocument()
    expect(screen.getByText('still fine')).toBeInTheDocument()
  })

  it('draws nothing for an id the feed does not hold', () => {
    seed([])
    const { container } = render(
      <TranscriptRow actions={actions} chatId="chat-1" id="m:x:y" providers={providers} />,
    )
    expect(container).toBeEmptyDOMElement()
  })
})

describe('UserRow', () => {
  it('shows a held prompt as sending until its turn takes it', () => {
    const l = ledger()
    seed([l.prompt('r1', 'run `make dev`')])
    render(<TranscriptRow actions={actions} chatId="chat-1" id="p:r1" providers={providers} />)
    expect(document.querySelector('[data-row-id="p:r1"]')).toHaveAttribute('data-state', 'pending')
    push(l.start('turn-1', 'r1'))
    expect(document.querySelector('[data-row-id="p:r1"]')).not.toHaveAttribute('data-state')
    expect(screen.getByText('make dev').closest('code')).not.toBeNull()
    expect(document.querySelector('.row.me .bubble')).not.toBeNull()
  })

  it('draws a slash command as a system marker, not a bubble', () => {
    const l = ledger()
    seed([l.row('user_message', { text: '/compact', origin: 'provider' })])
    render(<TranscriptRow actions={actions} chatId="chat-1" id="u:1" providers={providers} />)
    expect(screen.getByTestId('slash-command')).toHaveTextContent('/compact')
    expect(document.querySelector('.bubble')).toBeNull()
  })

  it('keeps the arguments after the command beside it', () => {
    const l = ledger()
    seed([l.row('user_message', { text: '/review the diff', origin: 'provider' })])
    render(<TranscriptRow actions={actions} chatId="chat-1" id="u:1" providers={providers} />)
    expect(screen.getByTestId('slash-command')).toHaveTextContent('/review')
    expect(screen.getByText('the diff')).toBeInTheDocument()
  })

  it.each(['/Users/me/file.ts is broken', 'run /compact later', 'a path /usr/bin'])(
    'leaves %s as an ordinary message',
    (text) => {
      const l = ledger()
      seed([l.row('user_message', { text, origin: 'provider' })])
      render(<TranscriptRow actions={actions} chatId="chat-1" id="u:1" providers={providers} />)
      expect(screen.queryByTestId('slash-command')).toBeNull()
      expect(document.querySelector('.bubble')).not.toBeNull()
    },
  )

  it('stamps no time under a sent message', () => {
    const l = ledger()
    seed([l.row('user_message', { text: 'hello', origin: 'provider' })])
    render(<TranscriptRow actions={actions} chatId="chat-1" id="u:1" providers={providers} />)
    expect(document.querySelector('time')).toBeNull()
  })

  it('says who sent a harness message instead of wearing the user bubble', () => {
    const l = ledger()
    seed([
      l.start('turn-1'),
      l.row(
        'user_message',
        { text: '<task-notification/>', origin: 'harness' },
        { turnId: 'turn-1' },
      ),
    ])
    render(<TranscriptRow actions={actions} chatId="chat-1" id="u:2" providers={providers} />)
    expect(screen.getByTestId('message-harness-label')).toHaveTextContent(
      'Sent to the agent by Fake',
    )
    expect(document.querySelector('.bubble')).toBeNull()
  })

  // A long payload would push the rest of the turn down.
  it.each([
    ['a long', 'x'.repeat(161)],
    ['a several-line', 'one\ntwo\nthree\nfour'],
  ])('collapses %s harness payload behind Show payload', (_, text) => {
    const l = ledger()
    seed([
      l.start('turn-1'),
      l.row('user_message', { text, origin: 'harness' }, { turnId: 'turn-1' }),
    ])
    render(<TranscriptRow actions={actions} chatId="chat-1" id="u:2" providers={providers} />)
    expect(screen.getByText('Show payload').tagName).toBe('SUMMARY')
  })

  it('shows a short harness payload inline', () => {
    const l = ledger()
    seed([
      l.start('turn-1'),
      l.row('user_message', { text: 'short payload', origin: 'harness' }, { turnId: 'turn-1' }),
    ])
    render(<TranscriptRow actions={actions} chatId="chat-1" id="u:2" providers={providers} />)
    expect(screen.getByText('short payload').closest('details')).toBeNull()
  })

  it('keeps the first turn in the document hand', () => {
    const l = ledger()
    seed([l.row('user_message', { text: 'first', origin: 'provider' })])
    render(
      <TranscriptRow
        actions={actions}
        chatId="chat-1"
        id="u:1"
        providers={providers}
        place={{ firstTurn: true }}
      />,
    )
    expect(document.querySelector('[data-first-turn="true"] .frozen')).not.toBeNull()
  })
})
