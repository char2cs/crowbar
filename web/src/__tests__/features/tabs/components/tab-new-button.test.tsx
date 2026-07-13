import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import TabNewButton from '@/features/tabs/components/tab-new-button'

const baseProps = {
  isBottomPane: false,
  disablePaneActions: false,
  isInSplit: false,
  onNewTerminal: vi.fn(),
  onOpenUrl: vi.fn(),
  onClosePane: vi.fn(),
}

describe('TabNewButton', () => {
  it('renders the + trigger button', () => {
    render(<TabNewButton {...baseProps} />)
    expect(screen.getByRole('button', { name: 'New tab' })).toBeDefined()
  })
})
