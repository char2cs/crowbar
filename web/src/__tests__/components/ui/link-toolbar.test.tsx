import { act, fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { Plate, PlateContent, usePlateEditor } from 'platejs/react'
import { ChatLinkKit } from '@/features/agent/composer/plate/attachments/chat-link-kit'

const openWorkspaceLink = vi.hoisted(() => vi.fn(() => Promise.resolve()))
vi.mock('@/lib/workspace-link', () => ({ openWorkspaceLink }))

const url = 'https://a.test/page'
// jsdom has no layout; the toolbar's floating position measures the caret's range.
Range.prototype.getBoundingClientRect ??= () => DOMRect.fromRect()
Range.prototype.getClientRects ??= () => [] as unknown as DOMRectList

function Editor() {
  const editor = usePlateEditor({
    plugins: ChatLinkKit,
    value: [{ type: 'p', children: [{ type: 'a', url, children: [{ text: 'a link' }] }] }],
    selection: { anchor: { path: [0, 0, 0], offset: 2 }, focus: { path: [0, 0, 0], offset: 2 } },
  })
  return (
    <Plate editor={editor}>
      <PlateContent aria-label="composer" />
    </Plate>
  )
}

describe('LinkFloatingToolbar', () => {
  it('opens the link under the caret in the OS browser rather than leaving it to the webview', async () => {
    render(<Editor />)
    act(() => screen.getByLabelText('composer').focus())

    const open = await screen.findByLabelText('Open link in a new tab')
    const click = fireEvent.click(open)

    expect(click).toBe(false)
    expect(openWorkspaceLink).toHaveBeenCalledWith(url, null)
  })
})
