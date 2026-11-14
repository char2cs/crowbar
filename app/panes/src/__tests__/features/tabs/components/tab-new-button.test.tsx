import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import TabAddButton from '@/features/tabs/components/tab-add-button'

describe('TabAddButton', () => {
  it('renders the + button', () => {
    render(<TabAddButton isBottomPane={false} onNewTab={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'New tab' })).toBeDefined()
  })

  // The choice of what lands in the pane lives on the New Tab page itself.
  it('opens a New Tab directly, with no menu in between', async () => {
    const user = userEvent.setup()
    const onNewTab = vi.fn()
    render(<TabAddButton isBottomPane={false} onNewTab={onNewTab} />)

    await user.click(screen.getByRole('button', { name: 'New tab' }))

    expect(onNewTab).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('menu')).toBeNull()
    expect(screen.queryByRole('menuitem')).toBeNull()
  })

  it('is absent in the bottom pane', () => {
    render(<TabAddButton isBottomPane={true} onNewTab={vi.fn()} />)
    expect(screen.queryByRole('button', { name: 'New tab' })).toBeNull()
  })
})
