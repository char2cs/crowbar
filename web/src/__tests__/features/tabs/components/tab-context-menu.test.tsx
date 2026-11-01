import { describe, it, expect, vi } from 'vitest'
import { useState } from 'react'
import { render, fireEvent, screen, waitFor } from '@testing-library/react'
import TabContextMenu from '@/features/tabs/components/tab-context-menu'
import { PrimitiveDialogProvider } from '@/components/ui/primitive-dialog-service'
import type { PaneContent } from '@/features/panes/types/pane-content'

const terminal = {
  id: 'buf-term',
  type: 'terminal',
  name: 'zsh',
  sessionId: 'session-1',
  path: 'terminal://session-1',
  isPinned: false,
} as PaneContent

/** tab-bar.tsx's wiring: closing flips `isOpen` and keeps the tab it was opened on. */
function TabWithMenu() {
  const [menu, setMenu] = useState<{ isOpen: boolean; buffer: PaneContent | null }>({
    isOpen: false,
    buffer: null,
  })
  return (
    <PrimitiveDialogProvider>
      <button
        onContextMenu={(e) => {
          e.preventDefault()
          setMenu({ isOpen: true, buffer: terminal })
        }}
      >
        zsh
      </button>
      <TabContextMenu
        isOpen={menu.isOpen}
        position={{ x: 0, y: 0 }}
        buffer={menu.buffer}
        onClose={() => setMenu((m) => ({ ...m, isOpen: false }))}
        onPin={vi.fn()}
        onCloseTab={vi.fn()}
        onCloseOthers={vi.fn()}
        onCloseAll={vi.fn()}
        onCloseToRight={vi.fn()}
      />
    </PrimitiveDialogProvider>
  )
}

describe('TabContextMenu', () => {
  it("Rename on a terminal tab leaves the name prompt's input focused once the menu has closed", async () => {
    render(<TabWithMenu />)
    const tab = screen.getByText('zsh')
    // The tab holds focus when the menu opens: that is where the menu returns it.
    tab.focus()
    fireEvent.contextMenu(tab)
    fireEvent.click(await screen.findByText('Rename'))
    const input = await screen.findByPlaceholderText('Terminal name')
    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    await waitFor(() => expect(input).toHaveFocus())
    expect(tab).not.toHaveFocus()
  })
})
