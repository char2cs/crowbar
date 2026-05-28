import { useSidebarStore } from '@/features/layout/stores/sidebar-store'

beforeEach(() => {
  useSidebarStore.setState({ sidebarVisible: true })
})

it('defaults sidebarVisible to true', () => {
  expect(useSidebarStore.getState().sidebarVisible).toBe(true)
})

it('setSidebarVisible(false) sets sidebarVisible to false', () => {
  useSidebarStore.getState().setSidebarVisible(false)
  expect(useSidebarStore.getState().sidebarVisible).toBe(false)
})

it('setSidebarVisible(true) restores sidebarVisible', () => {
  useSidebarStore.getState().setSidebarVisible(false)
  useSidebarStore.getState().setSidebarVisible(true)
  expect(useSidebarStore.getState().sidebarVisible).toBe(true)
})
