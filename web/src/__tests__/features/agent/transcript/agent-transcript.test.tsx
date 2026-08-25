import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import { AgentTranscript } from '@/features/agent/transcript/agent-transcript'

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
