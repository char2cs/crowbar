import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi } from 'vitest'
import { OpenEditorButton } from '@/features/tabs/components/open-editor-button'

describe('OpenEditorButton', () => {
  it('asks to open the editor area beside the chat on click', async () => {
    const onOpen = vi.fn()
    render(<OpenEditorButton wsId="w1" onOpen={onOpen} />)
    await userEvent.click(screen.getByRole('button', { name: 'Open editor' }))
    expect(onOpen).toHaveBeenCalledOnce()
  })

  it('is absent for a chat with no workspace — it would be a dead control', () => {
    render(<OpenEditorButton wsId={null} onOpen={vi.fn()} />)
    expect(screen.queryByRole('button', { name: 'Open editor' })).not.toBeInTheDocument()
  })
})
