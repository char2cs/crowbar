import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

vi.mock('@/lib/api/workspace', () => ({ retryProvision: vi.fn().mockResolvedValue(undefined) }))

import { retryProvision } from '@/lib/api/workspace'
import { useDetachModalStore } from '@/features/window/stores/detach-modal-store'
import { PlaceholderRowActions } from '@/components/layout/placeholder-row-actions'
import type { Workspace } from '@/lib/store/sidebar'

const ws = (over: Partial<Workspace> = {}): Workspace => ({
  id: 'w1',
  branch: 'develop',
  status: 'locked',
  age: '',
  heldByPath: '/repo',
  ...over,
})

beforeEach(() => {
  vi.clearAllMocks()
  useDetachModalStore.setState({ target: null })
})

describe('PlaceholderRowActions', () => {
  it('shows the reconstructed reason naming the holder', () => {
    render(<PlaceholderRowActions workspace={ws()} />)
    expect(screen.getByText(/checked out at \/repo/i)).toBeInTheDocument()
  })

  it('retries provisioning on Retry', async () => {
    render(<PlaceholderRowActions workspace={ws()} />)
    await userEvent.click(screen.getByRole('button', { name: /retry/i }))
    expect(retryProvision).toHaveBeenCalledWith('w1')
  })

  it('opens the detach modal on Detach when a holder is known', async () => {
    render(<PlaceholderRowActions workspace={ws()} />)
    await userEvent.click(screen.getByRole('button', { name: /detach/i }))
    expect(useDetachModalStore.getState().target).toEqual({
      wsId: 'w1',
      branch: 'develop',
      heldByPath: '/repo',
    })
  })

  it('hides Detach when there is no holder path', () => {
    render(<PlaceholderRowActions workspace={ws({ heldByPath: '' })} />)
    expect(screen.queryByRole('button', { name: /detach/i })).toBeNull()
  })
})
