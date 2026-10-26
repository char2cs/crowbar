import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))

const { SelectionCluster } = await import('@/features/agent/controls/selection-cluster')

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

afterEach(() => {
  platform.mobile = false
})

describe('SelectionCluster', () => {
  it('offers the chat/terminal switch on the desktop', () => {
    renderCluster()
    expect(screen.getByRole('tablist', { name: 'View' })).toBeInTheDocument()
  })

  // The phone shows the transcript only; the provider's TUI is a desktop surface.
  it('offers no switch on the phone', () => {
    platform.mobile = true
    renderCluster()
    expect(screen.queryByRole('tablist', { name: 'View' })).not.toBeInTheDocument()
  })
})
