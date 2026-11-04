import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))
// The desktop's split tree and editor hosts have their own suites.
vi.mock('@/features/panes/components/split-view-root', () => ({
  SplitViewRoot: () => <div data-testid="split-view-root" />,
}))
vi.mock('@/features/panes/components/editor-host-registry', () => ({
  EditorHostRegistry: () => <div data-testid="editor-hosts" />,
}))

const { WorkspaceLayoutRoot } =
  await import('@/features/workspace/components/workspace-layout-root')

afterEach(() => {
  platform.mobile = false
  resetWindowPaneStoreForTests()
})

describe('WorkspaceLayoutRoot', () => {
  it('lays out the split tree and editor hosts on the desktop', () => {
    render(<WorkspaceLayoutRoot />)
    expect(screen.getByTestId('split-view-root')).toBeInTheDocument()
    expect(screen.getByTestId('editor-hosts')).toBeInTheDocument()
  })

  it('shows the single-chat stage on the phone, with no splits', () => {
    platform.mobile = true
    render(<WorkspaceLayoutRoot />)
    expect(screen.queryByTestId('split-view-root')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /choose a chat/i })).toBeInTheDocument()
  })

  it("hosts the editors the phone's IDE opens files into", () => {
    platform.mobile = true
    render(<WorkspaceLayoutRoot />)
    expect(screen.getByTestId('editor-hosts')).toBeInTheDocument()
  })
})
