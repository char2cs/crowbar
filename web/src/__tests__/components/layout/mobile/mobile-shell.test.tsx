import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { treeUiStore, type RemovalDraft } from '@/features/workspace/tree/tree-ui-store'
import type { Project } from '@/lib/types'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { resetEntities, seedProjects } from '@/__tests__/__fixtures__/sidebar-entities'
import { writeChat } from '@/__tests__/__fixtures__/agent-chat'

const router = vi.hoisted(() => ({ pathname: '/ide/p1/r1/ws-1', navigate: vi.fn() }))
vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useNavigate: () => router.navigate,
  useRouterState: (opts?: { select?: (s: { location: { pathname: string } }) => unknown }) => {
    const state = { location: { pathname: router.pathname } }
    return opts?.select ? opts.select(state) : state
  },
  Outlet: () => null,
}))
// The workspace content is the desktop's own; its suite owns what it does.
vi.mock('@/components/layout/ide-shell-workspace-content', () => ({
  IdeShellWorkspaceContent: (props: { effectiveActiveWorkspaceId: string | null }) => (
    <div data-testid="workspace-content" data-ws={props.effectiveActiveWorkspaceId ?? ''} />
  ),
}))
const chrome = vi.hoisted(() => ({ native: false }))
vi.mock('@/lib/native-chrome', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/native-chrome')>()),
  hasNativeChrome: () => chrome.native,
}))

const { MobileShell } = await import('@/components/layout/mobile/mobile-shell')

const DOOMED: RemovalDraft = {
  kind: 'chat',
  id: 'c-doomed',
  label: 'Doomed',
  projectId: 'p1',
  repoId: 'r1',
  wsId: 'ws-1',
  providerIcon: '',
  hiddenIds: ['c-doomed'],
  extra: 0,
  fallbackWsId: null,
}

const project = { id: 'p1', name: 'crowbar', path: '/c', hostId: 'h1' } as unknown as Project

beforeEach(() => {
  chrome.native = false
  router.pathname = '/ide/p1/r1/ws-1'
  useMobileScreenStore.setState({ screen: 'chat', chatListOpen: false })
  useUIState.setState({ isSettingsOpen: false })
  resetEntities()
  seedProjects([project])
})

describe('MobileShell', () => {
  it('shows the routed workspace as the one pane', () => {
    render(<MobileShell />)
    expect(screen.getByTestId('workspace-content')).toHaveAttribute('data-ws', 'ws-1')
  })

  it('pushes the focused chat’s IDE, and back returns to the chat', () => {
    resetWindowPaneStoreForTests()
    act(() =>
      windowPaneStore
        .getState()
        .paneActions.openChat('c1', { projectId: 'p1', workspaceId: 'ws-1' }),
    )
    render(<MobileShell />)
    expect(screen.queryByRole('region', { name: 'IDE' })).not.toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /open ide/i }))
    const ide = screen.getByRole('region', { name: 'IDE' })
    expect(ide).toBeVisible()
    expect(screen.getByRole('button', { name: /new terminal/i })).toBeInTheDocument()

    fireEvent.click(screen.getByRole('button', { name: /^back$/i }))
    expect(useMobileScreenStore.getState().screen).toBe('chat')
    resetWindowPaneStoreForTests()
  })

  it('offers no IDE while no chat is focused', () => {
    resetWindowPaneStoreForTests()
    render(<MobileShell />)
    expect(screen.getByRole('button', { name: /open ide/i })).toBeDisabled()
  })

  // Title only: the desktop's identity row, view switch and provider picker stay off the phone.
  it('titles the screen with the focused chat', () => {
    resetWindowPaneStoreForTests()
    writeChat({ id: 'c1', workspaceId: 'ws-1', title: 'Fix login' })
    act(() =>
      windowPaneStore
        .getState()
        .paneActions.openChat('c1', { projectId: 'p1', workspaceId: 'ws-1' }),
    )
    render(<MobileShell />)
    expect(screen.getByRole('heading', { level: 1, name: 'Fix login' })).toBeInTheDocument()
    resetWindowPaneStoreForTests()
  })

  it('puts the chat list away once a chat is opened from it', () => {
    useMobileScreenStore.setState({ chatListOpen: true })
    const { rerender } = render(<MobileShell />)
    router.pathname = '/ide/p1/r1/ws-2'
    act(() => rerender(<MobileShell />))
    expect(useMobileScreenStore.getState().chatListOpen).toBe(false)
  })

  // Leaving a chat that is being removed is not opening another: the list stays up.
  it('keeps the chat list up when the focused chat is held for removal', () => {
    resetWindowPaneStoreForTests()
    act(() =>
      windowPaneStore
        .getState()
        .paneActions.openChat('c-doomed', { projectId: 'p1', workspaceId: 'ws-1' }),
    )
    useMobileScreenStore.setState({ chatListOpen: true })
    render(<MobileShell />)
    act(() => {
      treeUiStore.getState().hold([DOOMED])
      const { panes, paneActions } = windowPaneStore.getState()
      for (const [id, pane] of Object.entries(panes))
        if (pane.chatId === 'c-doomed') paneActions.closePane(id)
    })
    expect(useMobileScreenStore.getState().chatListOpen).toBe(true)
    act(() => treeUiStore.setState(treeUiStore.getInitialState(), true))
    resetWindowPaneStoreForTests()
  })

  // The phone app's settings are a native sheet; the web dialog is only the fallback.
  it('leaves Settings to the native sheet when native chrome is drawn', () => {
    useUIState.setState({ isSettingsOpen: true })
    chrome.native = true
    const { unmount } = render(<MobileShell />)
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
    unmount()
    chrome.native = false
    render(<MobileShell />)
    expect(screen.getByRole('dialog')).toBeInTheDocument()
  })
})
