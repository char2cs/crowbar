import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { CommentComposer } from '@/features/panes/components/comment-composer'

function editable(container: HTMLElement): HTMLElement {
  const node = container.querySelector<HTMLElement>('[contenteditable="true"]')
  if (!node) throw new Error('composer has no editable')
  return node
}

// Cmd+Enter and Escape are deliberately NOT exercised here. A keydown
// dispatched at a Slate editable under jsdom never reaches a listener on that
// element — measured: window-capture and document-capture see the event, the
// editable's own listeners never do, and dispatchEvent returns already
// default-prevented. A test that fired one and asserted "did not submit" would
// pass for that reason alone, and would keep passing if the handler were
// deleted. What those keystrokes MEAN is pinned in comment-editor-keys.test.ts;
// that they arrive is checked in the running app.
describe('CommentComposer', () => {
  it('has no Write/Preview modes — the editable IS the preview', () => {
    render(<CommentComposer onSubmit={vi.fn()} onCancel={vi.fn()} />)
    expect(screen.queryByText('Write')).toBeNull()
    expect(screen.queryByText('Preview')).toBeNull()
  })

  it('refuses to submit an empty comment', () => {
    const onSubmit = vi.fn()
    render(<CommentComposer onSubmit={onSubmit} onCancel={vi.fn()} />)

    const button = screen.getByRole('button', { name: 'Comment' })
    expect(button).toBeDisabled()
    fireEvent.click(button)
    expect(onSubmit).not.toHaveBeenCalled()
  })

  it('seeds the editor from an existing comment and can post it back', () => {
    const onSubmit = vi.fn()
    render(
      <CommentComposer
        initialValue="This **allocates** per row."
        submitLabel="Save"
        onSubmit={onSubmit}
        onCancel={vi.fn()}
      />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(onSubmit).toHaveBeenCalledWith('This **allocates** per row.')
  })

  it('renders the seeded markdown as formatting, not as source', () => {
    const { container } = render(
      <CommentComposer initialValue="a **bold** word" onSubmit={vi.fn()} onCancel={vi.fn()} />,
    )

    // The whole point of the swap: `**` is not on screen, weight is.
    expect(editable(container).textContent).toBe('a bold word')
    expect(container.querySelector('strong')).not.toBeNull()
  })

  it('cancels from the Cancel button', () => {
    const onCancel = vi.fn()
    render(<CommentComposer initialValue="never mind" onSubmit={vi.fn()} onCancel={onCancel} />)

    fireEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(onCancel).toHaveBeenCalled()
  })

  it('keeps the text when a submit is attempted, so a failed POST loses nothing', () => {
    const onSubmit = vi.fn()
    const { container } = render(
      <CommentComposer initialValue="worth keeping" onSubmit={onSubmit} onCancel={vi.fn()} />,
    )

    fireEvent.click(screen.getByRole('button', { name: 'Comment' }))
    expect(editable(container).textContent).toBe('worth keeping')
    expect(screen.getByRole('button', { name: 'Comment' })).toBeEnabled()
  })

  it('carries its own opt-outs from the diff <pre> it gets slotted into', () => {
    // Both are inherited from the diff's <pre>: the code font, and
    // `white-space: pre` — which turns every newline between a rendered
    // comment's tags into a visible blank line.
    //
    // Asserted on the composer's OWN root, not on a wrapper, because that is
    // the whole claim: the draft variant is slotted straight into the
    // annotation with no ReviewThreadItem above it to supply either class. The
    // gutter button that opens that variant only mounts under real hover, which
    // the automation bridge cannot produce, so this is the gate on it.
    const { container } = render(<CommentComposer onSubmit={vi.fn()} onCancel={vi.fn()} />)
    const root = container.firstElementChild?.className ?? ''
    expect(root).toContain('ui-font')
    expect(root).toContain('whitespace-normal')
  })
})
