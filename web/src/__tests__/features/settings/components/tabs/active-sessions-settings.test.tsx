import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@/features/agent/lib/kill-agent', () => ({ confirmAndKillAgent: vi.fn() }))

import { ActiveSessionsSettings } from '@/features/settings/components/tabs/active-sessions-settings'
import { confirmAndKillAgent } from '@/features/agent/lib/kill-agent'
import { useActiveSessionsStore } from '@/lib/store/active-sessions'

const running = (id: string, title: string) => ({
  chatId: id,
  kind: 'started',
  chat: { id, workspaceId: 'ws-1', title, liveRunnerId: 'run' },
})

beforeEach(() => {
  vi.clearAllMocks()
  useActiveSessionsStore.setState({ sessions: {}, attached: {} })
})

describe('ActiveSessionsSettings', () => {
  it('says so when nothing is running', () => {
    render(<ActiveSessionsSettings />)

    expect(screen.getByText('No agents are running.')).toBeTruthy()
  })

  it('lists each running agent with its viewers, and flags an orphan', () => {
    const { applyFrame } = useActiveSessionsStore.getState()
    applyFrame('repo:r1', running('c1', 'Fix login'))
    applyFrame('repo:r1', running('c2', 'Write docs'))
    applyFrame('repo:r1', { chatId: 'c1', kind: 'attached', attached: 2 })

    render(<ActiveSessionsSettings />)

    const [first, second] = screen.getAllByTestId('active-session-row')
    expect(within(first).getByText('Fix login')).toBeTruthy()
    expect(within(first).getByText('2 viewers')).toBeTruthy()
    expect(within(second).getByText('Write docs')).toBeTruthy()
    expect(within(second).getByText('No viewers')).toBeTruthy()
  })

  it('kills the agent of the row whose Kill was pressed', async () => {
    useActiveSessionsStore.getState().applyFrame('repo:r1', running('c1', 'Fix login'))
    render(<ActiveSessionsSettings />)

    await userEvent.click(screen.getByRole('button', { name: 'Kill the agent on Fix login' }))

    expect(confirmAndKillAgent).toHaveBeenCalledWith(
      expect.objectContaining({ chatId: 'c1', workspaceId: 'ws-1', attached: 0 }),
    )
  })
})
