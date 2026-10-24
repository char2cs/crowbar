import { render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SidebarFoot } from '@/components/layout/sidebar-foot'
import { useSettingsStore } from '@/features/settings/store'

const sidebar = vi.hoisted(() => ({ open: true }))
vi.mock('@/components/ui/sidebar', () => ({
  useSidebar: () => ({ open: sidebar.open }),
}))

beforeEach(() => {
  sidebar.open = true
  useSettingsStore.setState((state) => {
    state.settings.buildBadgeOverride = 'beta'
  })
})

describe('SidebarFoot', () => {
  it('carries the build readout while the sidebar is open', () => {
    render(<SidebarFoot />)
    expect(screen.getByText(/beta - /)).toBeInTheDocument()
  })

  it('leaves the readout to the tab strip while the sidebar is hidden', () => {
    sidebar.open = false
    render(<SidebarFoot />)
    expect(screen.queryByText(/beta - /)).toBeNull()
  })
})
