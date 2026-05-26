import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { vi, test, expect } from 'vitest'

vi.mock('@/lib/api', () => ({
  postWorkspace: vi.fn(),
  fetchFlows: vi.fn(() => Promise.resolve([])),
}))

vi.mock('@/lib/store/sidebar', () => ({
  useSidebarStore: () => ({ addWorkspace: vi.fn() }),
}))

vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    createFileRoute: () => () => ({ component: null }),
    useNavigate: () => vi.fn(),
  }
})

vi.mock('@tanstack/react-query', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-query')>()
  return {
    ...actual,
    useQuery: () => ({ data: [{ name: 'feature-development', description: 'Feature dev' }] }),
  }
})

import { NewWorkspacePage } from '@/routes/workspaces/new'
import * as api from '@/lib/api'

test('resets loading and re-enables Create button when postWorkspace rejects', async () => {
  vi.mocked(api.postWorkspace).mockRejectedValueOnce(new Error('server error'))

  render(<NewWorkspacePage />)
  fireEvent.change(screen.getByLabelText('Branch'), { target: { value: 'feature/test' } })
  fireEvent.click(screen.getByRole('button', { name: /create/i }))

  await waitFor(() => {
    expect(screen.getByRole('button', { name: /create/i })).not.toBeDisabled()
  })
})
