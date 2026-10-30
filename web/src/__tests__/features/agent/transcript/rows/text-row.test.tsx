import { act, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it } from 'vitest'
import type { AgentProvider } from '@/features/agent/api/agent-providers-api'
import type { LedgerRow } from '@/features/agent/api/chat-rows'
import { applyFeedFrame, chatStore, openFeed } from '@/features/agent/stores/chat-store'
import { TranscriptRow } from '@/features/agent/transcript/rows/transcript-row'
import { delta, ledger } from '@/__tests__/__fixtures__/ledger-rows'

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
      <TranscriptRow chatId="chat-1" id={ID} providers={providers} place={{ turnbar: true }} />,
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
      <TranscriptRow chatId="chat-1" id={ID} providers={providers} place={{ turnbar: true }} />,
    )
    expect(prose()?.hasAttribute('contenteditable')).toBe(false)
    expect(screen.getByText('descriptor').closest('strong')).not.toBeNull()
  })

  it('follows the live stream ahead of its rows while unsettled', async () => {
    const l = ledger()
    seed([l.start('turn-1'), l.text('turn-1', 'm1', 0, 'Hel')])
    render(<TranscriptRow chatId="chat-1" id={ID} providers={providers} />)
    act(() => void applyFeedFrame('chat-1', { type: 'delta', delta: delta('m1', 1, 'Hello the') }))
    await waitFor(() => expect(prose()?.textContent).toContain('Hello the'))
  })

  it('shows the turn effort and time only on the reply that hands control back', () => {
    const l = ledger()
    seed([
      l.start('turn-1'),
      l.text('turn-1', 'm1', -1, 'done'),
      l.row('turn_ended', { status: 'done', effort: 'high' }, { turnId: 'turn-1' }),
    ])
    const { rerender } = render(
      <TranscriptRow chatId="chat-1" id={ID} providers={providers} place={{ turnbar: true }} />,
    )
    expect(screen.getByTestId('message-effort')).toHaveTextContent('high')
    rerender(
      <TranscriptRow chatId="chat-1" id={ID} providers={providers} place={{ turnbar: false }} />,
    )
    expect(screen.queryByTestId('message-turn-actions')).toBeNull()
  })

  it('holds the turn actions back until the reply settles', () => {
    const l = ledger()
    seed([l.start('turn-1'), l.text('turn-1', 'm1', 0, 'still going')])
    render(
      <TranscriptRow chatId="chat-1" id={ID} providers={providers} place={{ turnbar: true }} />,
    )
    expect(screen.queryByTestId('message-turn-actions')).toBeNull()
    push(l.end('turn-1'))
    expect(screen.getByTestId('message-turn-actions')).toBeInTheDocument()
  })

  it('draws nothing for an id the feed does not hold', () => {
    seed([])
    const { container } = render(<TranscriptRow chatId="chat-1" id="m:x:y" providers={providers} />)
    expect(container).toBeEmptyDOMElement()
  })
})

describe('UserRow', () => {
  it('waits for its turn before it joins the record', () => {
    const l = ledger()
    seed([l.prompt('r1', 'run `make dev`')])
    render(<TranscriptRow chatId="chat-1" id="p:r1" providers={providers} />)
    expect(document.querySelector('[data-row-id="p:r1"]')).toBeNull()
    push(l.start('turn-1', 'r1'))
    expect(screen.getByText('make dev').closest('code')).not.toBeNull()
    expect(document.querySelector('.row.me .bubble')).not.toBeNull()
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
    render(<TranscriptRow chatId="chat-1" id="u:2" providers={providers} />)
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
    render(<TranscriptRow chatId="chat-1" id="u:2" providers={providers} />)
    expect(screen.getByText('Show payload').tagName).toBe('SUMMARY')
  })

  it('shows a short harness payload inline', () => {
    const l = ledger()
    seed([
      l.start('turn-1'),
      l.row('user_message', { text: 'short payload', origin: 'harness' }, { turnId: 'turn-1' }),
    ])
    render(<TranscriptRow chatId="chat-1" id="u:2" providers={providers} />)
    expect(screen.getByText('short payload').closest('details')).toBeNull()
  })

  it('keeps the first turn in the document hand', () => {
    const l = ledger()
    seed([l.row('user_message', { text: 'first', origin: 'provider' })])
    render(
      <TranscriptRow chatId="chat-1" id="u:1" providers={providers} place={{ firstTurn: true }} />,
    )
    expect(document.querySelector('[data-first-turn="true"] .frozen')).not.toBeNull()
  })
})
