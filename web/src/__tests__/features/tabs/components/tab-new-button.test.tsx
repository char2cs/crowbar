import { fireEvent, render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import TabAddButton from '@/features/tabs/components/tab-add-button'

describe('TabAddButton', () => {
  it('renders the + trigger button', () => {
    render(<TabAddButton isBottomPane={false} onNewTab={vi.fn()} />)
    expect(screen.getByRole('button', { name: 'New tab' })).toBeDefined()
  })

  it('opens a New Tab on click', () => {
    const onNewTab = vi.fn()
    render(<TabAddButton isBottomPane={false} onNewTab={onNewTab} />)

    fireEvent.click(screen.getByRole('button', { name: 'New tab' }))

    expect(onNewTab).toHaveBeenCalledTimes(1)
  })

  it('renders no dropdown menu — a single click fires onNewTab directly', () => {
    render(<TabAddButton isBottomPane={false} onNewTab={vi.fn()} />)

    expect(screen.queryByRole('menu')).toBeNull()
    expect(screen.queryByText('New Terminal')).toBeNull()
  })

  it('is absent in the bottom pane', () => {
    render(<TabAddButton isBottomPane={true} onNewTab={vi.fn()} />)
    expect(screen.queryByRole('button', { name: 'New tab' })).toBeNull()
  })
})
