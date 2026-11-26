import { render, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'

vi.mock('@/features/panes/components/editor-host-registry', () => ({
  EditorHostRegistry: () => <div data-testid="editor-hosts" />,
}))

const { WorkspaceLayoutRoot } =
  await import('@/features/workspace/components/workspace-layout-root')

afterEach(() => {
  resetWindowPaneStoreForTests()
})

describe('WorkspaceLayoutRoot', () => {
  it('shows the single-chat stage', () => {
    render(<WorkspaceLayoutRoot />)
    expect(screen.getByRole('button', { name: /choose a chat/i })).toBeInTheDocument()
  })

  it("hosts the editors the phone's IDE opens files into", () => {
    render(<WorkspaceLayoutRoot />)
    expect(screen.getByTestId('editor-hosts')).toBeInTheDocument()
  })
})
