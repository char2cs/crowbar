import { createElement } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'

vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
// A terminal's xterm has its own suite; this one is about the IDE's tabs.
vi.mock('@/features/panes/components/terminal-pane', () => ({
  TerminalPane: (props: { bufferId: string; isVisible?: boolean }) =>
    createElement('div', {
      'data-testid': `terminal-${props.bufferId}`,
      'data-visible': String(props.isVisible ?? false),
    }),
}))
const platform = vi.hoisted(() => ({ mobile: true }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))

const { useMobileScreenStore } = await import('@/features/window/stores/mobile-screen-store')
const { MobileIdeScreen } = await import('@/components/layout/mobile/mobile-ide-screen')
const { openNewTabPage } = await import('@/features/panes/utils/pane-command-actions')

beforeEach(() => {
  resetWindowPaneStoreForTests()
  act(() =>
    windowPaneStore.getState().paneActions.openChat('c1', { projectId: 'p1', workspaceId: 'ws-1' }),
  )
})
afterEach(() => resetWindowPaneStoreForTests())

describe('MobileIdeScreen', () => {
  it('greets an empty workspace with what the phone can open', () => {
    render(<MobileIdeScreen hidden={false} />)
    expect(screen.getByRole('button', { name: /new terminal/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /review branch/i })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /new browser/i })).not.toBeInTheDocument()
  })

  // The tab strip itself is native (use-native-tabs); the IDE keeps every tab's view mounted.
  it('keeps each terminal mounted while another is shown', () => {
    render(<MobileIdeScreen hidden={false} />)
    fireEvent.click(screen.getByRole('button', { name: /new terminal/i }))
    const paneId = windowPaneStore.getState().activePaneId
    act(() => openNewTabPage('ws-1', paneId))
    fireEvent.click(screen.getByRole('button', { name: /new terminal/i }))
    const [first, second] = windowPaneStore.getState().buffers.map((b) => b.id)
    expect(screen.getByTestId(`terminal-${first}`)).toHaveAttribute('data-visible', 'false')
    expect(screen.getByTestId(`terminal-${second}`)).toHaveAttribute('data-visible', 'true')
    act(() => windowPaneStore.getState().paneActions.activateEditorTabInPane(paneId, first))
    expect(screen.getByTestId(`terminal-${first}`)).toHaveAttribute('data-visible', 'true')
  })

  it('draws no tab strip or chat title of its own', () => {
    render(<MobileIdeScreen hidden={false} />)
    fireEvent.click(screen.getByRole('button', { name: /new terminal/i }))
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
    expect(screen.queryByRole('heading')).not.toBeInTheDocument()
  })

  it('shows the workspace’s files over its tabs, and Back puts them away', () => {
    useMobileScreenStore.setState({ screen: 'ide', filesOpen: false })
    render(<MobileIdeScreen hidden={false} />)
    fireEvent.click(screen.getByRole('button', { name: 'Files' }))
    expect(screen.getByRole('region', { name: 'Files' })).toBeVisible()
    fireEvent.click(screen.getAllByRole('button', { name: 'Back' })[0])
    expect(screen.queryByRole('region', { name: 'Files' })).not.toBeInTheDocument()
    expect(useMobileScreenStore.getState().screen).toBe('ide')
  })

  it('renders nothing while no chat is focused', () => {
    resetWindowPaneStoreForTests()
    render(<MobileIdeScreen hidden={false} />)
    expect(screen.queryByRole('region', { name: 'IDE' })).not.toBeInTheDocument()
  })
})
