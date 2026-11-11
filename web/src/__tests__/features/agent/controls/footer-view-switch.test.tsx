import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { FooterViewSwitch } from '@/features/agent/controls/footer-view-switch'

describe('FooterViewSwitch', () => {
  it('names both views, marks the current one and switches on click', () => {
    const onSelect = vi.fn()
    render(<FooterViewSwitch presentation="chat" onSelect={onSelect} />)
    expect(screen.getByRole('tablist', { name: 'View' })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Chat' })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByRole('tab', { name: 'Terminal' })).toHaveAttribute('aria-selected', 'false')
    fireEvent.click(screen.getByRole('tab', { name: 'Terminal' }))
    expect(onSelect).toHaveBeenCalledWith('terminal')
  })

  it('moves between the segments with the arrow keys', async () => {
    const onSelect = vi.fn()
    render(<FooterViewSwitch presentation="chat" onSelect={onSelect} />)
    await userEvent.click(screen.getByRole('tab', { name: 'Chat' }))
    await userEvent.keyboard('{ArrowRight}')
    expect(onSelect).toHaveBeenCalledWith('terminal')
  })

  it('disables the terminal while the provider cannot hand a live turn over', () => {
    render(<FooterViewSwitch presentation="chat" handoverBlocked onSelect={vi.fn()} />)
    expect(screen.getByRole('tab', { name: 'Terminal' })).toHaveAttribute('aria-disabled', 'true')
  })
})
