import { describe, it, expect, beforeEach } from 'vitest'
import { useConversationStore } from '@/lib/store/conversations'

beforeEach(() => {
  useConversationStore.setState({ sessions: new Map() })
})

describe('useConversationStore', () => {
  it('getMessages returns empty array on cold start (no mock seed)', () => {
    const msgs = useConversationStore.getState().getMessages('ws3', 'brainstorm')
    expect(msgs).toHaveLength(0)
  })

  it('getMessages returns the same empty array on repeated calls', () => {
    const first = useConversationStore.getState().getMessages('ws1', 'spec')
    const second = useConversationStore.getState().getMessages('ws1', 'spec')
    expect(first).toBe(second)
  })
})
