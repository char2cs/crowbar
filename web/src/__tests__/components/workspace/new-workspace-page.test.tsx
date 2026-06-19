import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { vi, test, expect, beforeEach } from 'vitest'

vi.mock('@/lib/api', () => ({
  postWorkspace: vi.fn(),
  fetchFlows: vi.fn(() => Promise.resolve([])),
}))

// awaitEntity subscribes through wsManager — mock it so the test drives frames.
const subscribers = new Set<(data: unknown) => void>()
vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (_endpoint: string, cb: (data: unknown) => void) => {
      subscribers.add(cb)
      return () => subscribers.delete(cb)
    },
    send: vi.fn(),
  },
}))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => vi.fn(),
  }
})

import { NewWorkspacePage } from '@/components/workspace/new-workspace-page'
import { useSidebarStore } from '@/lib/store/sidebar'
import * as api from '@/lib/api'

beforeEach(() => {
  subscribers.clear()
  vi.mocked(api.postWorkspace).mockReset()
  useSidebarStore.setState({
    repos: [
      {
        id: 'crowbar',
        projectId: 'p1',
        name: 'crowbar',
        avatarLabel: 'C',
        avatarColor: 'bg-indigo-700',
        workspaces: [],
      },
    ],
  })
})

test('resets loading and re-enables Create button when postWorkspace rejects', async () => {
  vi.mocked(api.postWorkspace).mockRejectedValueOnce(new Error('server error'))

  render(<NewWorkspacePage />)
  fireEvent.change(screen.getByLabelText('Branch'), { target: { value: 'feature/test' } })
  fireEvent.click(screen.getByRole('button', { name: /create/i }))

  await waitFor(() => {
    expect(screen.getByRole('button', { name: /create/i })).not.toBeDisabled()
  })
})
