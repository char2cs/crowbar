import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { ViewSwitch } from '@/features/agent/controls/view-switch'

describe('ViewSwitch', () => {
  it('names both views, marks the current one and switches on click', () => {
    const onSelect = vi.fn()
    render(<ViewSwitch presentation="chat" onSelect={onSelect} />)
    expect(screen.getByRole('tablist', { name: 'View' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Chat' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: 'Terminal' })).toHaveAttribute('aria-selected', 'false')
    fireEvent.click(screen.getByRole('tab', { name: 'Terminal' }))
    expect(onSelect).toHaveBeenCalledWith('terminal')
  })

  it('moves between the segments with the arrow keys', async () => {
    const onSelect = vi.fn()
    render(<ViewSwitch presentation="chat" onSelect={onSelect} />)
    await userEvent.click(screen.getByRole('tab', { name: 'Chat' }))
    await userEvent.keyboard('{ArrowRight}')
    expect(onSelect).toHaveBeenCalledWith('terminal')
  })

  it('disables the terminal while the provider cannot hand a live turn over', () => {
    render(<ViewSwitch presentation="chat" handoverBlocked onSelect={vi.fn()} />)
    expect(screen.getByRole('tab', { name: 'Terminal' })).toHaveAttribute('aria-disabled', 'true')
  })

  it('disables the face you would move to, in either direction', () => {
    const { rerender } = render(
      <ViewSwitch presentation="terminal" handoverBlocked onSelect={vi.fn()} />,
    )
    expect(screen.getByRole('tab', { name: 'Chat' })).toHaveAttribute('aria-disabled', 'true')
    rerender(<ViewSwitch presentation="chat" onSelect={vi.fn()} />)
    expect(screen.getByRole('tab', { name: 'Chat' })).not.toHaveAttribute('aria-disabled', 'true')
  })

  it('offers the diagnostic Split segment only when enabled', () => {
    const onSelect = vi.fn()
    const { rerender } = render(<ViewSwitch presentation="chat" onSelect={onSelect} />)
    expect(screen.queryByRole('tab', { name: 'Split' })).toBeNull()
    rerender(<ViewSwitch presentation="chat" splitEnabled onSelect={onSelect} />)
    fireEvent.click(screen.getByRole('tab', { name: 'Split' }))
    expect(onSelect).toHaveBeenCalledWith('split')
  })
})
