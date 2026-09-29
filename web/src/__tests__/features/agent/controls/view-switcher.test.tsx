import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { ViewSwitcher } from '@/features/agent/controls/view-switcher'

const props = { splitEnabled: false, onSelect: vi.fn() }

describe('ViewSwitcher', () => {
  it('disables the face you would move to while a live turn cannot be handed over', () => {
    const { rerender } = render(<ViewSwitcher {...props} presentation="terminal" handoverBlocked />)
    expect(screen.getByRole('tab', { name: 'Chat' })).toBeDisabled()
    expect(screen.getByRole('tab', { name: 'Terminal' })).toBeEnabled()

    rerender(<ViewSwitcher {...props} presentation="chat" handoverBlocked />)
    expect(screen.getByRole('tab', { name: 'Terminal' })).toBeDisabled()
    expect(screen.getByRole('tab', { name: 'Chat' })).toBeEnabled()
  })

  it('leaves both faces available when nothing blocks the handover', () => {
    render(<ViewSwitcher {...props} presentation="terminal" />)
    expect(screen.getByRole('tab', { name: 'Chat' })).toBeEnabled()
  })
})
