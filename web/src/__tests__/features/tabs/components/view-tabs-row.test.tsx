import { act } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ViewTabsRow } from '@/features/tabs/components/view-tabs-row'
import { chatPaneIndex } from '@/features/panes/lib/view-selectors'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { useSettingsStore } from '@/features/settings/store'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'

const sidebar = vi.hoisted(() => ({ open: true }))
vi.mock('@tanstack/react-router', () => ({ useNavigate: () => vi.fn() }))
vi.mock('@/components/ui/sidebar', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/components/ui/sidebar')>()),
  useSidebar: () => ({ open: sidebar.open, toggleSidebar: vi.fn() }),
}))
vi.mock('@/features/panes/lib/release-closed-chat', () => ({
  releaseClosedChat: vi.fn(async () => {}),
}))

const actions = () => windowPaneStore.getState().paneActions
const paneOf = (chatId: string) => chatPaneIndex(windowPaneStore.getState().panes).get(chatId)!
const viewOf = (chatId: string) => windowPaneStore.getState().panes[paneOf(chatId)].viewId!

beforeEach(() => {
  sidebar.open = true
  useSettingsStore.setState((state) => {
    state.settings.buildBadgeOverride = 'beta'
    state.settings.sidebarPosition = 'left'
  })
  useSidebarStore.setState({
    ...getInitialState(),
    repos: [
      {
        id: 'r1',
        projectId: 'p1',
        name: 'crowbar',
        avatarLabel: 'C',
        avatarColor: 'bg-indigo-700',
        workspaces: [],
        chats: [
          { id: 'chat-1', repoId: 'r1', title: 'Chat One', order: 0, workspaceId: 'ws-1' },
          { id: 'chat-2', repoId: 'r1', title: 'Chat Two', order: 1, workspaceId: 'ws-1' },
        ],
      },
    ],
  })
  resetWindowPaneStoreForTests()
  actions().setActiveProject('p1')
})

describe('ViewTabsRow', () => {
  it('renders nothing while the project has no views', () => {
    const { container } = render(<ViewTabsRow />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing before any project is active', () => {
    resetWindowPaneStoreForTests()
    const { container } = render(<ViewTabsRow />)
    expect(container).toBeEmptyDOMElement()
  })

  it('draws the sidebar row for each view, labelled by its chat', () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    actions().openChat('chat-2', { workspaceId: 'ws-1' })
    render(<ViewTabsRow />)
    expect(screen.getByTestId('recents-row-chat-1')).toHaveTextContent('Chat One')
    expect(screen.getByTestId('recents-row-chat-2')).toHaveTextContent('Chat Two')
  })

  it('clicking a row brings its view on screen', async () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    actions().openChat('chat-2', { workspaceId: 'ws-1' })
    render(<ViewTabsRow />)
    await userEvent.click(within(screen.getByTestId('recents-row-chat-1')).getByRole('treeitem'))
    expect(windowPaneStore.getState().activeViewId).toBe(viewOf('chat-1'))
  })

  it("a row's × ends that view", async () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    actions().openChat('chat-2', { workspaceId: 'ws-1' })
    const closed = viewOf('chat-1')
    render(<ViewTabsRow />)
    const shell = screen.getByTestId('recents-row-chat-1').parentElement!
    await userEvent.click(within(shell).getByRole('button', { name: 'Close view' }))
    expect(windowPaneStore.getState().views[closed]).toBeUndefined()
    expect(screen.queryByTestId('recents-row-chat-1')).not.toBeInTheDocument()
  })

  it('shows a set as one shell with every member', () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    act(() => actions().dropChatOnPane('chat-2', paneOf('chat-1'), 'right', 'ws-1'))
    render(<ViewTabsRow />)
    expect(screen.getByTestId(`recents-set-${viewOf('chat-1')}`)).toBeInTheDocument()
    expect(screen.getAllByTestId(/^recents-row-/)).toHaveLength(2)
  })

  it('re-renders when a reorder writes only viewOrder', () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    actions().openChat('chat-2', { workspaceId: 'ws-1' })
    render(<ViewTabsRow />)
    const order = () =>
      screen.getAllByTestId(/^recents-row-/).map((el) => el.getAttribute('data-testid'))
    expect(order()).toEqual(['recents-row-chat-1', 'recents-row-chat-2'])

    const [first, second] = windowPaneStore.getState().viewOrder
    act(() => actions().reorderView(second, first, 'before'))

    expect(order()).toEqual(['recents-row-chat-2', 'recents-row-chat-1'])
  })

  // The sidebar's foot shows the build readout while it is open.
  it('carries the build readout only while the sidebar is hidden', () => {
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    const { unmount } = render(<ViewTabsRow />)
    expect(screen.queryByText(/beta - /)).toBeNull()
    unmount()

    sidebar.open = false
    render(<ViewTabsRow />)
    expect(screen.getByText(/beta - /)).toBeInTheDocument()
  })

  it('places the readout before the reopen toggle so they never overlap', () => {
    sidebar.open = false
    useSettingsStore.setState((state) => {
      state.settings.sidebarPosition = 'right'
    })
    actions().openChat('chat-1', { workspaceId: 'ws-1' })
    // jsdom has no layout: report a row flush against the window's right edge.
    const rect = { left: 0, top: 0, width: window.innerWidth, height: 32, right: window.innerWidth }
    const measure = vi
      .spyOn(HTMLElement.prototype, 'getBoundingClientRect')
      .mockReturnValue(rect as DOMRect)
    render(<ViewTabsRow />)
    measure.mockRestore()
    const readout = screen.getByText(/beta - /)
    const toggle = screen.getByRole('button', { name: 'Show sidebar' })
    expect(readout.compareDocumentPosition(toggle)).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
  })
})
