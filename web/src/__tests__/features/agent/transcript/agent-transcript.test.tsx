import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import { AgentTranscript } from '@/features/agent/transcript/agent-transcript'

function draw(
  messages: AgentChatMessage[],
  overrides: Partial<Parameters<typeof AgentTranscript>[0]> = {},
) {
  return render(
    <AgentTranscript
      messages={messages}
      queue={[]}
      providers={[]}
      activity={{ toolCalls: [], subagents: [], interruptions: [], choices: [] }}
      working={false}
      loading={false}
      error={null}
      hasOlder={false}
      onLoadOlder={() => {}}
      onRetryLoad={() => {}}
      onOpenTerminal={() => {}}
      onEditPrompt={() => {}}
      onCancelPrompt={() => {}}
      onRetryPrompt={() => {}}
      {...overrides}
    />,
  )
}

describe('AgentTranscript provider labels', () => {
  it('shows the provider label on the first assistant message and on a provider change, not on consecutive same-provider replies', () => {
    const messages: AgentChatMessage[] = [
      { turnId: 't1', sequence: 1, role: 'user', providerId: '', text: 'hi', at: '' },
      { turnId: 't2', sequence: 2, role: 'assistant', providerId: 'claude', text: 'a', at: '' },
      { turnId: 't3', sequence: 3, role: 'assistant', providerId: 'claude', text: 'b', at: '' },
      { turnId: 't4', sequence: 4, role: 'assistant', providerId: 'codex', text: 'c', at: '' },
    ]
    render(
      <AgentTranscript
        messages={messages}
        queue={[]}
        providers={[]}
        activity={{ toolCalls: [], subagents: [], interruptions: [], choices: [] }}
        working={false}
        loading={false}
        error={null}
        hasOlder={false}
        onLoadOlder={() => {}}
        onRetryLoad={() => {}}
        onOpenTerminal={() => {}}
        onEditPrompt={() => {}}
        onCancelPrompt={() => {}}
        onRetryPrompt={() => {}}
      />,
    )
    // Sequence 2: first assistant message -> label shown. Sequence 3: same
    // provider as 2 -> no label. Sequence 4: provider changed -> label shown.
    const rows = screen.getAllByTestId(/^agent-message-\d+$/)
    expect(rows[1].querySelector('.meta')).not.toBeNull()
    expect(rows[2].querySelector('.meta')).toBeNull()
    expect(rows[3].querySelector('.meta')).not.toBeNull()
  })
})

describe('AgentTranscript first-turn framing', () => {
  it('freezes the oldest loaded message when there is nothing earlier to page in', () => {
    draw([
      { turnId: 't1', sequence: 0, role: 'user', providerId: '', text: 'first', at: '' },
      { turnId: 't2', sequence: 1, role: 'assistant', providerId: 'claude', text: 'reply', at: '' },
    ])

    expect(screen.getByTestId('agent-message-0')).toHaveAttribute('data-first-turn', 'true')
    expect(screen.getByTestId('agent-message-1')).not.toHaveAttribute('data-first-turn')
  })

  // The true first message is not always the first one ON SCREEN — paging in
  // older history must not make the CURRENT oldest-loaded message look frozen
  // when it never was the beginning of the conversation.
  it('does not freeze the oldest LOADED message when older history exists', () => {
    draw(
      [
        {
          turnId: 't5',
          sequence: 5,
          role: 'user',
          providerId: '',
          text: 'not actually first',
          at: '',
        },
      ],
      { hasOlder: true },
    )

    expect(screen.getByTestId('agent-message-5')).not.toHaveAttribute('data-first-turn')
  })

  it('draws the plain hairline right after the frozen turn, with no reply yet', () => {
    draw([{ turnId: 't1', sequence: 0, role: 'user', providerId: '', text: 'first', at: '' }])

    expect(screen.getByTestId('agent-first-turn-divider')).toBeInTheDocument()
  })

  it('draws no hairline at all once older history exists and nothing is frozen', () => {
    draw([{ turnId: 't5', sequence: 5, role: 'user', providerId: '', text: 'not first', at: '' }], {
      hasOlder: true,
    })

    expect(screen.queryByTestId('agent-first-turn-divider')).toBeNull()
  })

  // The assistant's answer to the frozen turn keeps its larger size too —
  // see message-row.tsx's own note on `firstReply`.
  it('marks the assistant message that answers the frozen turn, and no other', () => {
    draw([
      { turnId: 't1', sequence: 0, role: 'user', providerId: '', text: 'first', at: '' },
      { turnId: 't1', sequence: 1, role: 'assistant', providerId: 'claude', text: 'reply', at: '' },
      { turnId: 't2', sequence: 2, role: 'user', providerId: '', text: 'follow-up', at: '' },
      {
        turnId: 't2',
        sequence: 3,
        role: 'assistant',
        providerId: 'claude',
        text: 'second reply',
        at: '',
      },
    ])

    expect(screen.getByTestId('agent-message-1')).toHaveAttribute('data-first-reply', 'true')
    expect(screen.getByTestId('agent-message-3')).not.toHaveAttribute('data-first-reply')
  })

  it('does not mark anything when a harness message sits between the frozen turn and the reply', () => {
    draw([
      { turnId: 't1', sequence: 0, role: 'user', providerId: '', text: 'first', at: '' },
      { turnId: 't1', sequence: 1, role: 'harness', providerId: 'claude', text: 'note', at: '' },
      { turnId: 't1', sequence: 2, role: 'assistant', providerId: 'claude', text: 'reply', at: '' },
    ])

    // The harness row between them is never a candidate; the reply is still
    // the first assistant message after the frozen turn.
    expect(screen.getByTestId('agent-message-2')).toHaveAttribute('data-first-reply', 'true')
  })

  it('marks nothing when there is no reply yet', () => {
    draw([{ turnId: 't1', sequence: 0, role: 'user', providerId: '', text: 'first', at: '' }])

    expect(screen.queryByTestId('[data-first-reply]')).toBeNull()
  })

  it('marks nothing when older history exists, even if the oldest loaded message is an assistant reply', () => {
    draw(
      [
        {
          turnId: 't1',
          sequence: 5,
          role: 'assistant',
          providerId: 'claude',
          text: 'not actually the first reply',
          at: '',
        },
      ],
      { hasOlder: true },
    )

    expect(screen.getByTestId('agent-message-5')).not.toHaveAttribute('data-first-reply')
  })
})

describe('AgentTranscript interrupted marker', () => {
  const oneFrozenTurn = [
    { turnId: 't1', sequence: 0, role: 'user' as const, providerId: '', text: 'first', at: '' },
  ]

  it('draws the trailing marker once the turn has actually gone idle, with nothing after it yet', () => {
    draw(oneFrozenTurn, { trailingInterruption: true, working: false })

    expect(screen.getByTestId('agent-interrupted-divider')).toHaveTextContent('Interrupted')
  })

  // The working line and the marker say opposite things about the same
  // instant — never both on screen, and the spinner's own disappearance is
  // what hands off to it, not a separate timer.
  it('does not draw the trailing marker while the turn still reads as working', () => {
    draw(oneFrozenTurn, { trailingInterruption: true, working: true })

    expect(screen.queryByTestId('agent-interrupted-divider')).toBeNull()
  })

  it('draws nothing when nothing was interrupted', () => {
    draw(oneFrozenTurn, { trailingInterruption: false, working: false })

    expect(screen.queryByTestId('agent-interrupted-divider')).toBeNull()
  })

  // REGRESSION: once a message actually follows the stop, the marker must hand
  // off to that FIXED position and stop trailing the transcript — the bug was
  // a marker that kept re-anchoring itself to "the end", drifting down every
  // time something new was said after the stop.
  it('draws anchored above the first message that follows the stop, not trailing, once one exists', () => {
    const messages = [
      { turnId: 't1', sequence: 0, role: 'user' as const, providerId: '', text: 'first', at: '' },
      { turnId: 't2', sequence: 1, role: 'user' as const, providerId: '', text: 'after the stop', at: '' },
    ]
    draw(messages, {
      interruptedBefore: { 1: true },
      trailingInterruption: false,
      working: false,
    })

    const divider = screen.getByTestId('agent-interrupted-divider')
    const later = screen.getByText('after the stop')
    expect(divider.compareDocumentPosition(later) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })
})
