import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SelectionCluster } from '@/features/agent/controls/selection-cluster'

const renderCluster = () =>
  render(
    <SelectionCluster
      providers={[]}
      model=""
      effort=""
      presentation="chat"
      splitEnabled={false}
      showSwitcher
      onSelectionChange={vi.fn()}
      onSelectPresentation={vi.fn()}
    />,
  )

describe('SelectionCluster', () => {
  it('offers the chat/terminal switch on the desktop', () => {
    renderCluster()
    expect(screen.getByRole('tablist', { name: 'View' })).toBeInTheDocument()
  })
})
