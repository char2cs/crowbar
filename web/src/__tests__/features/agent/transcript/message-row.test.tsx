import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type {
  AgentChatMessage,
  AgentChatMessageRole,
  AgentProvider,
} from '@/features/agent/api/agent-api'
import { MessageRow } from '@/features/agent/transcript/message-row'

const providers = [{ id: 'claude', displayName: 'Claude' }] as AgentProvider[]

function row(role: AgentChatMessageRole, text: string) {
  const message: AgentChatMessage = {
    turnId: 't1',
    sequence: 1,
    role,
    providerId: 'claude',
    text,
    at: '2026-08-24T00:00:00Z',
  }
  return render(<MessageRow message={message} showProvider={false} providers={providers} />)
}

describe('MessageRow', () => {
  // A prompt is composed in a rich markdown editor and SENT as markdown. Showing
  // it back as source made the box's own content un-render the instant it was
  // sent — the same emphasis, two appearances one line apart.
  describe('renders both sides of the conversation as markdown', () => {
    for (const role of ['user', 'assistant'] as const) {
      it(`renders ${role} emphasis as emphasis, not as asterisks`, () => {
        row(role, 'the **descriptor** decides')
        // Plate wraps the string in its own leaf spans, so the mark is the
        // ANCESTOR — asserting on the matched node's own tag tests Slate's
        // internals, not whether the markdown rendered.
        expect(screen.getByText('descriptor').closest('strong')).not.toBeNull()
        expect(screen.queryByText(/\*\*descriptor\*\*/)).toBeNull()
      })

      it(`renders a ${role} code span as code`, () => {
        row(role, 'run `make dev`')
        expect(screen.getByText('make dev').closest('code')).not.toBeNull()
      })
    }
  })

  it('keeps the user bubble a bubble', () => {
    const { container } = row('user', 'hello')
    expect(container.querySelector('.bubble')).not.toBeNull()
    expect(container.querySelector('.row.me')).not.toBeNull()
  })

  // A sent message keeps the shape of the box it was typed in: stadium while it
  // fits on one line, 18px once it wraps.
  it('switches the bubble to the wrapped radius once it grows past one line', () => {
    const { container } = row('user', 'a'.repeat(61))
    expect(container.querySelector('.bubble.multi')).not.toBeNull()
  })

  it('switches the bubble to the wrapped radius on an explicit newline, whatever the length', () => {
    const { container } = row('user', 'short\nmessage')
    expect(container.querySelector('.bubble.multi')).not.toBeNull()
  })

  it('keeps the stadium radius for a short one-line message', () => {
    const { container } = row('user', 'hi')
    expect(container.querySelector('.bubble.multi')).toBeNull()
  })

  // A harness payload can run well past one line — collapsed behind the same
  // <details> pattern ChoiceSchema already uses, rather than pushing the rest
  // of the turn down.
  it('shows a short harness payload inline, uncollapsed', () => {
    row('harness', 'short payload')
    expect(screen.getByText('short payload').closest('details')).toBeNull()
    expect(screen.getByText('short payload').closest('code')).not.toBeNull()
  })

  it('collapses a long harness payload behind Show payload', () => {
    const long = 'x'.repeat(161)
    row('harness', long)
    const details = screen.getByText(long).closest('details')
    expect(details).not.toBeNull()
    expect(screen.getByText('Show payload').tagName).toBe('SUMMARY')
  })

  it('collapses a harness payload with several lines even under the length threshold', () => {
    const { container } = row('harness', 'one\ntwo\nthree\nfour')
    expect(container.querySelector('details')).not.toBeNull()
  })

  // The chat's very first turn keeps the empty document's own typography rather
  // than switching to a bubble the instant it is sent.
  describe('the first turn', () => {
    function firstTurnRow(text: string) {
      const message: AgentChatMessage = {
        turnId: 't1',
        sequence: 0,
        role: 'user',
        providerId: 'claude',
        text,
        at: '2026-08-24T00:00:00Z',
      }
      return render(
        <MessageRow message={message} showProvider={false} providers={providers} firstTurn />,
      )
    }

    it('renders as a frozen document, not a bubble', () => {
      const { container } = firstTurnRow('describe the change')
      expect(container.querySelector('.frozen')).not.toBeNull()
      expect(container.querySelector('.bubble')).toBeNull()
      // Full width — no right alignment.
      expect(container.querySelector('.row.me')).toBeNull()
    })

    it('marks itself for anything reading the DOM directly', () => {
      const { container } = firstTurnRow('describe the change')
      expect(container.querySelector('[data-first-turn="true"]')).not.toBeNull()
    })

    it('still renders through the same markdown pipeline as every other message', () => {
      firstTurnRow('the **descriptor** decides')
      expect(screen.getByText('descriptor').closest('strong')).not.toBeNull()
    })

    it('does not freeze a non-first user message', () => {
      const message: AgentChatMessage = {
        turnId: 't2',
        sequence: 5,
        role: 'user',
        providerId: 'claude',
        text: 'a later message',
        at: '2026-08-24T00:00:00Z',
      }
      const { container } = render(
        <MessageRow message={message} showProvider={false} providers={providers} />,
      )
      expect(container.querySelector('.bubble')).not.toBeNull()
      expect(container.querySelector('.frozen')).toBeNull()
    })

    // firstTurn only ever means anything for the user's own words — a role
    // nobody types is never "the first turn" in the sense this is drawing.
    it('is a no-op on a role other than user', () => {
      const message: AgentChatMessage = {
        turnId: 't1',
        sequence: 0,
        role: 'assistant',
        providerId: 'claude',
        text: 'hi',
        at: '2026-08-24T00:00:00Z',
      }
      const { container } = render(
        <MessageRow message={message} showProvider={false} providers={providers} firstTurn />,
      )
      expect(container.querySelector('.frozen')).toBeNull()
      expect(container.querySelector('.assistant')).not.toBeNull()
    })
  })

  // The assistant's answer to the frozen first turn keeps that same larger
  // hand rather than dropping to ordinary reply prose the instant the turn
  // is over.
  describe('the first reply', () => {
    function firstReplyRow(text: string) {
      const message: AgentChatMessage = {
        turnId: 't1',
        sequence: 1,
        role: 'assistant',
        providerId: 'claude',
        text,
        at: '2026-08-24T00:00:00Z',
      }
      return render(
        <MessageRow message={message} showProvider={false} providers={providers} firstReply />,
      )
    }

    it('keeps the frozen document size on top of its ordinary assistant styling', () => {
      const { container } = firstReplyRow('the reply')
      expect(container.querySelector('.assistant.frozen')).not.toBeNull()
    })

    it('marks itself for anything reading the DOM directly', () => {
      const { container } = firstReplyRow('the reply')
      expect(container.querySelector('[data-first-reply="true"]')).not.toBeNull()
    })

    it('does not mark a later assistant reply', () => {
      const message: AgentChatMessage = {
        turnId: 't2',
        sequence: 5,
        role: 'assistant',
        providerId: 'claude',
        text: 'a later reply',
        at: '2026-08-24T00:00:00Z',
      }
      const { container } = render(
        <MessageRow message={message} showProvider={false} providers={providers} />,
      )
      expect(container.querySelector('.frozen')).toBeNull()
      expect(container.querySelector('[data-first-reply]')).toBeNull()
    })

    // firstReply only ever means anything for the assistant's own words — a
    // role that never answers a turn is never "the first reply" in the
    // sense this is drawing.
    it('is a no-op on a role other than assistant', () => {
      const message: AgentChatMessage = {
        turnId: 't1',
        sequence: 1,
        role: 'user',
        providerId: 'claude',
        text: 'a later user message',
        at: '2026-08-24T00:00:00Z',
      }
      const { container } = render(
        <MessageRow message={message} showProvider={false} providers={providers} firstReply />,
      )
      expect(container.querySelector('.frozen')).toBeNull()
      expect(container.querySelector('.bubble')).not.toBeNull()
    })
  })

  // The two roles nobody in the conversation typed stay VERBATIM, for opposite
  // reasons. A harness payload is markup: `<task-notification>` is an HTML tag to
  // a markdown parser, and rendering it swallows the row whole.
  it('shows a harness payload verbatim, tags and all', () => {
    row('harness', '<task-notification>agent finished</task-notification>')
    expect(
      screen.getByText('<task-notification>agent finished</task-notification>'),
    ).toBeInTheDocument()
  })

  it('shows a notice as the provider’s exact sentence', () => {
    row('notice', "You've hit your usage limit — resets at 4pm.")
    expect(screen.getByText("You've hit your usage limit — resets at 4pm.")).toBeInTheDocument()
  })

  it('never dresses a harness message in the user’s bubble', () => {
    const { container } = row('harness', 'a subagent finished')
    expect(container.querySelector('.bubble')).toBeNull()
    expect(container.querySelector('.harness')).not.toBeNull()
  })

  // A role this build has never heard of still renders its text: the ledger
  // recorded something the agent acted on, and a client too old to name it is
  // not a reason to hide it.
  it('still renders a role it has never heard of', () => {
    row('something_new' as AgentChatMessageRole, 'recorded anyway')
    expect(screen.getByText('recorded anyway')).toBeInTheDocument()
  })
})

describe('MessageRow memoization', () => {
  it('does not re-render when called twice with identical prop values', () => {
    const message: AgentChatMessage = {
      turnId: 't1',
      sequence: 1,
      role: 'assistant',
      providerId: 'claude',
      text: 'hi',
      at: '2026-08-24T00:00:00Z',
    }
    const { rerender, container } = render(
      <MessageRow message={message} showProvider={false} providers={providers} />,
    )
    const firstHTML = container.innerHTML
    // Same object references — memo's default shallow comparator must skip this render.
    rerender(<MessageRow message={message} showProvider={false} providers={providers} />)
    expect(container.innerHTML).toBe(firstHTML)
  })
})
