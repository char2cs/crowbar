import { act, fireEvent, render, screen } from '@testing-library/react'
import { expect, it, vi } from 'vitest'
import { createPlateEditor, Plate, PlateContent } from 'platejs/react'
import { ChatLinkKit } from '@/features/agent/composer/plate/attachments/chat-link-kit'

const openWorkspaceLink = vi.hoisted(() => vi.fn(() => Promise.resolve()))
vi.mock('@/lib/workspace-link', () => ({ openWorkspaceLink }))

const url = 'https://a.test/page'
const caret = { path: [0, 0, 0], offset: 2 }
const value = [{ type: 'p', children: [{ type: 'a', url, children: [{ text: 'a link' }] }] }]
const selection = { anchor: caret, focus: caret }
const editor = createPlateEditor({ plugins: ChatLinkKit, value, selection })

it('opens the link under the caret in the OS browser, not in the webview', async () => {
  render(
    <Plate editor={editor}>
      <PlateContent aria-label="composer" />
    </Plate>,
  )
  act(() => screen.getByLabelText('composer').focus())
  expect(fireEvent.click(await screen.findByLabelText('Open link in a new tab'))).toBe(false)
  expect(openWorkspaceLink).toHaveBeenCalledWith(url, null)
})
