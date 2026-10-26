import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { NewTabView } from '@/features/panes/components/new-tab-view'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import { useHomeTreeStore, type HomeTree } from '@/lib/store/home-tree'

const ensurePaneChatThenOpen = vi.hoisted(() => vi.fn())
const openContent = vi.hoisted(() => vi.fn())
const openBranchReviewForWorkspace = vi.hoisted(() => vi.fn())
const platform = vi.hoisted(() => ({ mobile: false }))

vi.mock('@/features/panes/utils/pane-command-actions', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/panes/utils/pane-command-actions')>()),
  ensurePaneChatThenOpen,
  openBranchReviewForWorkspace,
}))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))
vi.mock('@/features/workspace/stores/hooks/use-buffer-store', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/workspace/stores/hooks/use-buffer-store')>()),
  useBufferActions: () => ({ openContent }),
}))

// An empty editor area greets with the Crowbar logo. Its actions go through
// ensurePaneChatThenOpen, which needs the pane's workspace: without one the
// view stays inert so it never mints a redundant chat (pane-command-actions.test.ts).
describe('NewTabView', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    platform.mobile = false
    resetWindowPaneStoreForTests()
    useHomeTreeStore.setState({ trees: {} })
    ensurePaneChatThenOpen.mockImplementation((_ws, _pane, open: (p: string) => void) =>
      open('pane-x'),
    )
  })

  it('without a resolved workspace shows only the decorative logo', () => {
    const { container } = render(<NewTabView paneId={ROOT_PANE_ID} wsId={null} />)
    expect(screen.queryAllByRole('button')).toHaveLength(0)
    const svgs = container.querySelectorAll('svg')
    expect(svgs).toHaveLength(1)
    expect(svgs[0]).toHaveAttribute('aria-hidden', 'true')
  })

  it('with a workspace offers New Terminal, New Browser and New File', () => {
    render(<NewTabView paneId={ROOT_PANE_ID} wsId="w1" />)
    expect(screen.getByRole('button', { name: /new terminal/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /new browser/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /new file/i })).toBeInTheDocument()
  })

  it('opens a browser tab through the pane-chat guard', async () => {
    render(<NewTabView paneId={ROOT_PANE_ID} wsId="w1" />)
    await userEvent.click(screen.getByRole('button', { name: /new browser/i }))
    expect(openContent).toHaveBeenCalledWith(expect.objectContaining({ type: 'browser' }), {
      paneId: 'pane-x',
    })
  })

  it('opens a terminal through the pane-chat guard', async () => {
    render(<NewTabView paneId={ROOT_PANE_ID} wsId="w1" />)
    await userEvent.click(screen.getByRole('button', { name: /new terminal/i }))
    expect(ensurePaneChatThenOpen).toHaveBeenCalledWith('w1', ROOT_PANE_ID, expect.any(Function))
    expect(openContent).toHaveBeenCalledWith({ type: 'terminal' }, { paneId: 'pane-x' })
  })

  it('opens an untitled file through the pane-chat guard', async () => {
    render(<NewTabView paneId={ROOT_PANE_ID} wsId="w1" />)
    await userEvent.click(screen.getByRole('button', { name: /new file/i }))
    expect(openContent).toHaveBeenCalledWith(
      expect.objectContaining({ type: 'editor', path: 'untitled:Untitled' }),
      { paneId: 'pane-x' },
    )
  })

  // Same gate as the header's shortcut: a thread owns no branch to review.
  it('offers Review Branch last for a non-thread chat, opening the review in this pane', async () => {
    render(<NewTabView paneId={ROOT_PANE_ID} wsId="w1" />)
    const names = screen.getAllByRole('button').map((b) => b.textContent)
    expect(names).toEqual(['New Terminal', 'New Browser', 'New File', 'Review Branch'])
    await userEvent.click(screen.getByRole('button', { name: /review branch/i }))
    expect(openBranchReviewForWorkspace).toHaveBeenCalledWith('w1', ROOT_PANE_ID, undefined)
  })

  // As a tab of its own, a choice takes that tab's place, like a browser's.
  it('as a New Tab page, every choice replaces the page instead of adding a tab', async () => {
    render(<NewTabView paneId={ROOT_PANE_ID} wsId="w1" replaceTabId="page-1" />)
    await userEvent.click(screen.getByRole('button', { name: /new terminal/i }))
    expect(openContent).toHaveBeenLastCalledWith(
      { type: 'terminal' },
      { paneId: 'pane-x', replaceTabId: 'page-1' },
    )
    await userEvent.click(screen.getByRole('button', { name: /new browser/i }))
    expect(openContent).toHaveBeenLastCalledWith(expect.objectContaining({ type: 'browser' }), {
      paneId: 'pane-x',
      replaceTabId: 'page-1',
    })
    await userEvent.click(screen.getByRole('button', { name: /new file/i }))
    expect(openContent).toHaveBeenLastCalledWith(expect.objectContaining({ type: 'editor' }), {
      paneId: 'pane-x',
      replaceTabId: 'page-1',
    })
    await userEvent.click(screen.getByRole('button', { name: /review branch/i }))
    expect(openBranchReviewForWorkspace).toHaveBeenCalledWith('w1', ROOT_PANE_ID, 'page-1')
  })

  it('hides Review Branch for a thread', () => {
    // A chat in a project home borrows its workspace: a thread.
    windowPaneStore.getState().paneActions.openChat('thread-1', { projectId: 'p1' })
    const paneId = windowPaneStore.getState().activePaneId
    useHomeTreeStore.setState({
      trees: { p1: { chats: [{ id: 'thread-1' }], folders: [] } as unknown as HomeTree },
    })
    render(<NewTabView paneId={paneId} wsId="w1" />)
    expect(screen.queryByRole('button', { name: /review branch/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /new file/i })).toBeInTheDocument()
  })

  // The browser pane is desktop-only and files come later: the phone offers what it can open.
  it('offers only New Terminal and Review Branch on the phone', () => {
    platform.mobile = true
    render(<NewTabView paneId={ROOT_PANE_ID} wsId="w1" />)
    const names = screen.getAllByRole('button').map((b) => b.textContent)
    expect(names).toEqual(['New Terminal', 'Review Branch'])
  })

  // The phone's empty IDE is the whole screen: a bigger logo over touch-sized rows.
  it('draws the phone variant with a larger logo, and the desktop one without', () => {
    const { container, unmount } = render(<NewTabView paneId={ROOT_PANE_ID} wsId="w1" />)
    expect(container.firstElementChild).not.toHaveAttribute('data-variant')
    expect(container.querySelector('svg[aria-hidden="true"]')).not.toHaveAttribute('width')
    unmount()

    platform.mobile = true
    const phone = render(<NewTabView paneId={ROOT_PANE_ID} wsId="w1" />)
    expect(phone.container.firstElementChild).toHaveAttribute('data-variant', 'phone')
    expect(phone.container.querySelector('svg[aria-hidden="true"]')).toHaveAttribute('width', '154')
  })

  // The desktop's gate: a chat on a branch offers its review; a thread or home chat owns none.
  it('on the phone offers Review Branch only to a chat on a branch', () => {
    platform.mobile = true
    const { unmount } = render(<NewTabView paneId={ROOT_PANE_ID} wsId="w1" />)
    expect(screen.getByRole('button', { name: /review branch/i })).toBeInTheDocument()
    unmount()

    windowPaneStore.getState().paneActions.openChat('thread-1', { projectId: 'p1' })
    const paneId = windowPaneStore.getState().activePaneId
    useHomeTreeStore.setState({
      trees: { p1: { chats: [{ id: 'thread-1' }], folders: [] } as unknown as HomeTree },
    })
    render(<NewTabView paneId={paneId} wsId="w1" />)
    expect(screen.queryByRole('button', { name: /review branch/i })).not.toBeInTheDocument()
  })
})
