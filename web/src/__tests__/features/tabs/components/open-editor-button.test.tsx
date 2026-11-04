import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it } from 'vitest'
import { resetWindowPaneStoreForTests } from '@/features/panes/stores/window-pane-store'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import { OpenEditorButton } from '@/features/tabs/components/open-editor-button'

const editorOpen = () => windowPaneStore.getState().panes[ROOT_PANE_ID].editorOpen

describe('OpenEditorButton', () => {
  beforeEach(() => {
    resetWindowPaneStoreForTests()
    act(() => {
      windowPaneStore.setState((s) => {
        s.panes[ROOT_PANE_ID] = { ...s.panes[ROOT_PANE_ID], editorOpen: false }
        return s
      })
    })
  })

  it('opens the editor area beside the chat on click', async () => {
    render(<OpenEditorButton paneId={ROOT_PANE_ID} wsId="w1" />)
    await userEvent.click(screen.getByRole('button', { name: 'Open editor' }))
    expect(editorOpen()).toBe(true)
  })

  it('is absent for a chat with no workspace — it would be a dead control', () => {
    render(<OpenEditorButton paneId={ROOT_PANE_ID} wsId={null} />)
    expect(screen.queryByRole('button', { name: 'Open editor' })).not.toBeInTheDocument()
  })
})
