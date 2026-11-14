import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@/features/agent/lib/kill-agent', () => ({ confirmAndKillAgent: vi.fn() }))

import { ActiveSessionsSettings } from '@/features/settings/components/tabs/active-sessions-settings'
import { confirmAndKillAgent } from '@/features/agent/lib/kill-agent'
import { runChat } from '@/__tests__/__fixtures__/running-chats'
import {
  projectDTO,
  resetEntities,
  seedProjects,
  seedRepos,
} from '@/__tests__/__fixtures__/sidebar-entities'

beforeEach(() => {
  vi.clearAllMocks()
  resetEntities()
})

describe('ActiveSessionsSettings', () => {
  it('says so when nothing is running', () => {
    render(<ActiveSessionsSettings />)

    expect(screen.getByText('No agents are running.')).toBeTruthy()
  })

  it('lists each running agent with its viewers, and flags an orphan', () => {
    runChat('c1', { title: 'Fix login', viewers: 2 })
    runChat('c2', { title: 'Write docs' })

    render(<ActiveSessionsSettings />)

    const [first, second] = screen.getAllByTestId('active-session-row')
    expect(within(first).getByText('Fix login')).toBeTruthy()
    expect(within(first).getByText('2 viewers')).toBeTruthy()
    expect(within(second).getByText('Write docs')).toBeTruthy()
    expect(within(second).getByText('No viewers')).toBeTruthy()
  })

  it('kills the agent of the row whose Kill was pressed', async () => {
    runChat('c1', { title: 'Fix login' })
    render(<ActiveSessionsSettings />)

    await userEvent.click(screen.getByRole('button', { name: 'Kill the agent on Fix login' }))

    expect(confirmAndKillAgent).toHaveBeenCalledWith(
      expect.objectContaining({ hostId: 'local', chatId: 'c1', workspaceId: 'ws-1', viewers: 0 }),
    )
  })

  it('names where each agent runs, as the chat header does', () => {
    seedProjects([projectDTO({ id: 'p1' })])
    seedRepos([
      {
        id: 'repo-1',
        projectId: 'p1',
        name: 'crowbar',
        avatarLabel: 'C',
        avatarColor: '',
        workspaces: [
          {
            id: 'ws-1',
            branch: 'enhancement/new-additions',
            parentBranch: 'feature/mobile-app',
            age: '',
          },
        ],
      },
    ])
    runChat('c1', { title: 'Fix login' })

    render(<ActiveSessionsSettings />)

    expect(
      within(screen.getByTestId('active-session-row')).getByText(
        'crowbar/feature/mobile-app/enhancement/new-additions',
      ),
    ).toBeTruthy()
  })

  it('shows an untitled chat as untitled, never as an empty row', () => {
    runChat('c1', { title: '' })

    render(<ActiveSessionsSettings />)

    expect(within(screen.getByTestId('active-session-row')).getByText('Untitled chat')).toBeTruthy()
  })

  it('falls back to a short id when the chat is in no visible project', () => {
    runChat('abcdef123456', { title: 'Fix login' })

    render(<ActiveSessionsSettings />)

    expect(within(screen.getByTestId('active-session-row')).getByText('abcdef12')).toBeTruthy()
  })
})
