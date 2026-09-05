import type { ReactNode } from 'react'
import { cleanup, fireEvent, render, renderHook, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { useDraggable, useDropLine } from '@platejs/dnd'
import type { TElement } from 'platejs'
import { Plate, type PlateEditor, usePlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import {
  AttachmentDragHandle,
  AttachmentDropLine,
  canDropAttachmentNode,
  onAttachmentDropHandler,
  useAttachmentDraggable,
} from '@/features/agent/composer/plate/attachment-drag-handle'

// `useDraggable`/`useDropLine` wire real HTML5 drag events, pointer capture
// and react-dnd's `DndContext` together — none of which this app actually
// provides a `<DndProvider>` for (confirmed by grep: no file under `src`
// renders one), so calling them for real here would throw "Expected drag
// drop context" outside a live drag session. Mocking them at the module
// boundary lets `useAttachmentDraggable`'s and `AttachmentDropLine`'s own
// (real, unmocked) composition logic — what gets forwarded in, what comes
// back out — be verified directly instead.
vi.mock('@platejs/dnd', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@platejs/dnd')>()
  return { ...actual, useDraggable: vi.fn(), useDropLine: vi.fn() }
})

afterEach(() => {
  cleanup()
  vi.resetAllMocks()
})

function EditorWrapper({ children }: { children: ReactNode }) {
  const editor = usePlateEditor({
    plugins: chatComposerPlugins,
    value: [{ type: 'p', children: [{ text: 'hi' }] }],
  })
  return <Plate editor={editor}>{children}</Plate>
}

function Harness() {
  const editor = usePlateEditor({
    plugins: chatComposerPlugins,
    value: [{ type: 'p', children: [{ text: 'hi' }] }],
  })
  return (
    <Plate editor={editor}>
      <AttachmentDragHandle dragRef={null} onSelect={() => editor.tf.select(editor.children[0]!)} />
    </Plate>
  )
}

describe('AttachmentDragHandle', () => {
  it('renders a grab affordance', () => {
    render(<Harness />)
    expect(screen.getByRole('button', { name: /reorder/i })).toBeInTheDocument()
  })

  it('calls onSelect on click', () => {
    const onSelect = vi.fn()
    render(<AttachmentDragHandle dragRef={null} onSelect={onSelect} />)
    fireEvent.click(screen.getByRole('button', { name: /reorder/i }))
    expect(onSelect).toHaveBeenCalledTimes(1)
  })
})

/**
 * `canDropAttachmentNode` and `onAttachmentDropHandler` are the two pure
 * callbacks handed to `@platejs/dnd`'s `useDraggable` — everything else in
 * this module wires real HTML5 drag events and pointer capture together,
 * which jsdom's `render()` doesn't meaningfully exercise (same "verify
 * live" carve-out as Tasks 27/28/29's Tauri drop wiring). These two are
 * ordinary functions, so they're called directly with constructed
 * `dragEntry`/`dropEntry`/`dragItem` values instead.
 */
describe('canDropAttachmentNode', () => {
  it('returns true for a drop target that is a same-parent sibling', () => {
    const dragEntry = [{ type: 'p', children: [] }, [0, 1]]
    const dropEntry = [{ type: 'p', children: [] }, [0, 2]]
    const result = canDropAttachmentNode({
      dragEntry,
      dropEntry,
    } as unknown as Parameters<typeof canDropAttachmentNode>[0])

    expect(result).toBe(true)
  })

  it('returns false for a drop target under a different parent', () => {
    const dragEntry = [{ type: 'p', children: [] }, [0, 1]]
    const dropEntry = [{ type: 'p', children: [] }, [1, 0]]
    const result = canDropAttachmentNode({
      dragEntry,
      dropEntry,
    } as unknown as Parameters<typeof canDropAttachmentNode>[0])

    expect(result).toBe(false)
  })
})

describe('onAttachmentDropHandler', () => {
  it('selects the dragged element when present', () => {
    const select = vi.fn()
    const editor = { tf: { select } } as unknown as PlateEditor
    const dragElement = { type: 'p', children: [{ text: 'x' }] }

    onAttachmentDropHandler(editor, {
      dragItem: { element: dragElement },
    } as unknown as Parameters<typeof onAttachmentDropHandler>[1])

    expect(select).toHaveBeenCalledTimes(1)
    expect(select).toHaveBeenCalledWith(dragElement)
  })

  it('does nothing when the drag item has no element', () => {
    const select = vi.fn()
    const editor = { tf: { select } } as unknown as PlateEditor

    onAttachmentDropHandler(editor, {
      dragItem: { dataTransfer: [], files: [], items: [] },
    } as unknown as Parameters<typeof onAttachmentDropHandler>[1])

    expect(select).not.toHaveBeenCalled()
  })
})

/**
 * `useAttachmentDraggable` itself is just composition: forward the element
 * (and its type) plus the two pure callbacks above into `useDraggable`, and
 * hand back whatever it returns. `useDraggable` is mocked (see top of file)
 * so this verifies that composition — the real, unmocked part of this
 * function — without needing a `<DndProvider>` this app doesn't have.
 */
describe('useAttachmentDraggable', () => {
  it('forwards the element, its type, and the pure drop callbacks to useDraggable', () => {
    const draggableState = {
      isAboutToDrag: false,
      isDragging: false,
      nodeRef: { current: null },
      previewRef: { current: null },
      handleRef: vi.fn(),
    }
    vi.mocked(useDraggable).mockReturnValue(draggableState)
    const element: TElement = { type: 'attachment_image', children: [] }

    const { result } = renderHook(() => useAttachmentDraggable(element), {
      wrapper: EditorWrapper,
    })

    expect(useDraggable).toHaveBeenCalledWith({
      element,
      type: 'attachment_image',
      canDropNode: canDropAttachmentNode,
      onDropHandler: onAttachmentDropHandler,
    })
    expect(result.current).toBe(draggableState)
  })
})

/**
 * Same mocked-boundary approach for `AttachmentDropLine`: `useDropLine` is
 * mocked, so this exercises the real render branch it's driving — nothing
 * when there's no active drop line, a top or bottom bar when there is.
 */
describe('AttachmentDropLine', () => {
  it('renders nothing when there is no active drop line', () => {
    vi.mocked(useDropLine).mockReturnValue({ dropLine: undefined })
    const { container } = render(<AttachmentDropLine />)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders a bar at the top for a top drop line', () => {
    vi.mocked(useDropLine).mockReturnValue({ dropLine: 'top' })
    const { container } = render(<AttachmentDropLine />)
    expect(container.firstElementChild).toHaveClass('-top-px')
  })

  it('renders a bar at the bottom for a bottom drop line', () => {
    vi.mocked(useDropLine).mockReturnValue({ dropLine: 'bottom' })
    const { container } = render(<AttachmentDropLine />)
    expect(container.firstElementChild).toHaveClass('-bottom-px')
  })
})
