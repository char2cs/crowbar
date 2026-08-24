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
