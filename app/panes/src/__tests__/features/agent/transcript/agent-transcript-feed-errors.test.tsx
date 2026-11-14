import { beforeEach, describe, expect, it } from 'vitest'
import { screen } from '@testing-library/react'
import { chatStore, failFeed, openFeed } from '@/features/agent/stores/chat-store'
import { drawTranscript as draw } from '@/__tests__/__fixtures__/agent-transcript'

beforeEach(() => {
  chatStore.setState({ feeds: {}, outbox: {} })
})

describe('AgentTranscript feed errors', () => {
  it('says it is still trying when the feed cannot be reached', () => {
    openFeed('chat-1')
    draw({ status: 'error' })
    expect(screen.getByText(/couldn’t reach this chat/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /retry/i })).not.toBeInTheDocument()
  })

  it('says why when the daemon refused the chat, and does not promise to keep trying', () => {
    openFeed('chat-1')
    failFeed('chat-1', "This chat's history could not be read")
    draw({ status: 'error' })
    expect(screen.getByText("This chat's history could not be read")).toBeInTheDocument()
    expect(screen.queryByText(/keeps trying/i)).not.toBeInTheDocument()
  })
})
