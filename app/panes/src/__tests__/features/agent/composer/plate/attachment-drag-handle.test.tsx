import type { ReactNode } from 'react'
import { fireEvent, render, renderHook, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Active, Over } from '@dnd-kit/core'
import type { TElement } from 'platejs'
import { createPlateEditor, Plate, useEditorRef, usePlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import {
  AttachmentControls,
  AttachmentDropLine,
  resolveAttachmentDrop,
  useAttachmentDraggable,
  useAttachmentDropTarget,
} from '@/features/agent/composer/plate/attachment-drag-handle'

// The real drag gesture (pointer sensor, collision, drop) is exercised end to
// end in dnd-scope.test.tsx; this file covers the pure pieces and the chrome.

afterEach(() => {
  vi.restoreAllMocks()
})

function EditorWrapper({ children }: { children: ReactNode }) {
  const editor = usePlateEditor({
    plugins: chatComposerPlugins,
    value: [{ type: 'p', children: [{ text: 'hi' }] }],
  })
  return <Plate editor={editor}>{children}</Plate>
}

describe('AttachmentControls', () => {
  it('renders a grab affordance and a delete affordance', () => {
    render(<AttachmentControls onDelete={vi.fn()} />)
    expect(screen.getByRole('button', { name: /reorder/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /remove this attachment/i })).toBeInTheDocument()
  })

  it('calls onDelete on delete click', () => {
    const onDelete = vi.fn()
    render(<AttachmentControls onDelete={onDelete} />)
    fireEvent.click(screen.getByRole('button', { name: /remove this attachment/i }))
    expect(onDelete).toHaveBeenCalledTimes(1)
  })

  // `size="icon-xs"` is the shared square small-icon token: one `size-N` utility sets width and
  // height together, so its presence is the squareness guarantee.
  it.each([/reorder/i, /remove this attachment/i])(
    'button %s uses the shared icon-xs size token',
    (name) => {
      render(<AttachmentControls onDelete={vi.fn()} />)
      const button = screen.getByRole('button', { name })
      expect(button.className).toMatch(/(?:^|\s)size-7(?:\s|$)/)
      expect(button.className).not.toContain('h-6')
      expect(button.className).not.toContain('w-4')
    },
  )

  // The buttons sit over arbitrary attachment content, so they need a truly OPAQUE surface:
  // `--popover` (solid in both themes), with no alpha-blended background under any variant.
  it.each([/reorder/i, /remove this attachment/i])('button %s is opaque in every state', (name) => {
    render(<AttachmentControls onDelete={vi.fn()} />)
    const button = screen.getByRole('button', { name })
    expect(button.className).toContain('dark:bg-popover')
    expect(button.className).toContain('dark:hover:bg-popover')
    expect(button.className).not.toMatch(/(?:^|\s)(?:[\w-]+:)*bg-[\w-]+\/\d+(?:\s|$)/)
  })
})

function threeBlocks() {
  const editor = createPlateEditor({
    plugins: chatComposerPlugins,
    value: [
      { type: 'p', children: [{ text: 'a' }] },
      { type: 'code_block', children: [{ type: 'code_line', children: [{ text: 'x' }] }] },
      { type: 'p', children: [{ text: 'b' }] },
      { type: 'p', children: [{ text: 'c' }] },
      { type: 'blockquote', children: [{ type: 'p', children: [{ text: 'nested' }] }] },
    ],
  })
  const [a, attachment, b, c, quote] = editor.children as TElement[]
  return {
    editor,
    a: a!,
    attachment: attachment!,
    b: b!,
    c: c!,
    nested: quote!.children[0] as TElement,
  }
}

describe('resolveAttachmentDrop', () => {
  function event(
    drag: { editor: unknown; element: TElement },
    drop: { editor: unknown; element: TElement } | null,
    pointerY: number,
  ) {
    return {
      active: { id: 'drag', data: { current: drag } } as unknown as Active,
      over: drop
        ? ({
            id: 'drop',
            rect: { top: 100, height: 20 },
            data: { current: drop },
          } as unknown as Over)
        : null,
      activatorEvent: { clientY: 0 } as unknown as Event,
      delta: { x: 0, y: pointerY },
    }
  }

  it('picks the top line in the upper half of the hovered block and the bottom line in the lower half', () => {
    const { editor, attachment, c } = threeBlocks()
    expect(
      resolveAttachmentDrop(event({ editor, element: attachment }, { editor, element: c }, 105))
        ?.target,
    ).toEqual({ id: 'drop', line: 'top' })
    expect(
      resolveAttachmentDrop(event({ editor, element: attachment }, { editor, element: c }, 115))
        ?.target,
    ).toEqual({ id: 'drop', line: 'bottom' })
  })

  const TOP = 105
  const BOTTOM = 115

  it.each([
    ['moves below a later sibling', 'c', BOTTOM, { at: [1], to: [3] }],
    ['moves above an earlier sibling', 'a', TOP, { at: [1], to: [0] }],
    ['is a no-op at the top of the next block', 'b', TOP, undefined],
    ['is a no-op at the bottom of the previous block', 'a', BOTTOM, undefined],
    ['refuses a drop on itself', 'attachment', TOP, undefined],
    ['refuses a drop into a different parent', 'nested', TOP, undefined],
  ] as const)('%s', (_name, hovered, y, expected) => {
    const blocks = threeBlocks()
    const { editor, attachment } = blocks
    const resolved = resolveAttachmentDrop(
      event({ editor, element: attachment }, { editor, element: blocks[hovered] }, y),
    )
    expect(resolved?.move).toEqual(expected)
  })

  it('refuses a drop into another editor', () => {
    const { editor, attachment } = threeBlocks()
    const other = threeBlocks()
    expect(
      resolveAttachmentDrop(
        event({ editor, element: attachment }, { editor: other.editor, element: other.c }, TOP),
      ),
    ).toBeNull()
  })

  it('resolves nothing when not over a target, or over one that is not an attachment target', () => {
    const { editor, attachment } = threeBlocks()
    expect(resolveAttachmentDrop(event({ editor, element: attachment }, null, 105))).toBeNull()
    expect(
      resolveAttachmentDrop(event({ editor, element: attachment }, {} as never, 105)),
    ).toBeNull()
  })
})

describe('useAttachmentDraggable', () => {
  // The trash button (AttachmentDeleteButton) calls this directly — it has
  // to remove the same RESOLVED block dragging operates on, not whatever
  // element the caller originally passed in (a file card passes its nested
  // link; the block to delete is its wrapping paragraph).
  it('remove() deletes the resolved block from the document', () => {
    function NestedFileCardWrapper({ children }: { children: ReactNode }) {
      const editor = usePlateEditor({
        plugins: chatComposerPlugins,
        value: [
          {
            type: 'p',
            children: [
              { type: 'a', url: 'chats/c1/attachments/a.pdf', children: [{ text: 'a.pdf' }] },
            ],
          },
          { type: 'p', children: [{ text: 'keep me' }] },
        ],
      })
      return <Plate editor={editor}>{children}</Plate>
    }

    function useDraggableForNestedLink() {
      const editor = useEditorRef()
      const link = (editor.children[0] as TElement).children[0] as TElement
      return { editor, result: useAttachmentDraggable(link) }
    }

    const { result } = renderHook(() => useDraggableForNestedLink(), {
      wrapper: NestedFileCardWrapper,
    })

    result.current.result.remove()

    expect(result.current.editor.children).toHaveLength(1)
    expect(result.current.editor.children[0]).toMatchObject({ children: [{ text: 'keep me' }] })
  })

  it('remove() deletes a top-level block as is', () => {
    const { result } = renderHook(
      () => {
        const editor = useEditorRef()
        return { editor, draggable: useAttachmentDraggable(editor.children[0] as TElement) }
      },
      { wrapper: EditorWrapper },
    )
    result.current.draggable.remove()
    expect(result.current.editor.children).toHaveLength(0)
  })

  it('remove() leaves the document alone for an element that is not in it', () => {
    const detached: TElement = { type: 'a', children: [{ text: 'x' } as never] }
    const { result } = renderHook(
      () => ({ editor: useEditorRef(), draggable: useAttachmentDraggable(detached) }),
      { wrapper: EditorWrapper },
    )
    result.current.draggable.remove()
    expect(result.current.editor.children).toHaveLength(1)
  })

  it('is inert outside a DndScope: not dragging, no drop line', () => {
    const { result } = renderHook(
      () => useAttachmentDraggable(useEditorRef().children[0] as TElement),
      { wrapper: EditorWrapper },
    )
    expect(result.current.isDragging).toBe(false)
    expect(result.current.dropLine).toBeUndefined()
  })
})

describe('useAttachmentDropTarget', () => {
  it('renders safely with no DndScope at all', () => {
    const { result } = renderHook(
      () => useAttachmentDropTarget(useEditorRef().children[0] as TElement),
      { wrapper: EditorWrapper },
    )
    expect(result.current.dropLine).toBeUndefined()
  })
})

describe('AttachmentDropLine', () => {
  it('renders nothing when there is no active drop line', () => {
    const { container } = render(<AttachmentDropLine line={undefined} />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders a bar at the top for a top drop line', () => {
    const { container } = render(<AttachmentDropLine line="top" />)
    expect(container.firstElementChild).toHaveClass('-top-px')
  })

  it('renders a bar at the bottom for a bottom drop line', () => {
    const { container } = render(<AttachmentDropLine line="bottom" />)
    expect(container.firstElementChild).toHaveClass('-bottom-px')
  })
})
