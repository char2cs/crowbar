import type { ReactNode } from 'react'
import { cleanup, fireEvent, render, renderHook, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { DndContext, type DndContextType } from 'react-dnd'
import { DndPlugin, useDraggable, useDropLine, useDropNode } from '@platejs/dnd'
import type { TElement } from 'platejs'
import {
  createPlateEditor,
  Plate,
  type PlateEditor,
  useEditorRef,
  usePlateEditor,
} from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import {
  ATTACHMENT_DND_TYPE,
  AttachmentDeleteButton,
  AttachmentDragHandle,
  AttachmentDropLine,
  canDropAttachmentNode,
  onAttachmentDropHandler,
  resolveDraggableBlockElement,
  useAttachmentDraggable,
  useAttachmentDropTarget,
} from '@/features/agent/composer/plate/attachment-drag-handle'

// `useDraggable`/`useDropLine`/`useDropNode` wire real HTML5 drag events,
// pointer capture and react-dnd's `DndContext` together — none of which this
// app actually provides a `<DndProvider>` for (confirmed by grep: no file
// under `src` renders one), so calling them for real here would throw
// "Expected drag drop context" outside a live drag session. Mocking them at
// the module boundary lets `useAttachmentDraggable`'s and
// `AttachmentDropLine`'s own (real, unmocked) composition logic — what gets
// forwarded in, what comes back out — be verified directly instead.
vi.mock('@platejs/dnd', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@platejs/dnd')>()
  return { ...actual, useDraggable: vi.fn(), useDropLine: vi.fn(), useDropNode: vi.fn() }
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

  // REGRESSION, reported live: this rendered as an odd `h-6 w-4` pill (24×16,
  // not square) with its own one-off sizing instead of the shared `Button`
  // size scale every other icon button in the app uses. `size="icon-xs"`
  // (button-variants.ts) is the established square small-icon token —
  // `size-N` is a single Tailwind utility that sets width AND height
  // together, so its presence IS the "actually square" guarantee, not just a
  // visual approximation.
  it('uses the shared icon-xs button size token, not a one-off non-square size', () => {
    render(<Harness />)
    const handle = screen.getByRole('button', { name: /reorder/i })
    expect(handle.className).toMatch(/(?:^|\s)size-7(?:\s|$)/)
    expect(handle.className).not.toContain('h-6')
    expect(handle.className).not.toContain('w-4')
  })

  // REGRESSION, reported live, THREE TIMES over:
  // 1. `variant="outline"` — translucent in dark mode (`dark:bg-input/32`,
  //    button-variants.ts).
  // 2. `variant="secondary"` — confirmed live via `getComputedStyle`,
  //    `--secondary` in this theme is a saturated olive green
  //    (`oklch(0.49 0.082 130)`), not a neutral surface at all.
  // 3. `bg-sidebar-element-idle`/`hover:bg-sidebar-element-hover` — matched
  //    the "Crowbar Seed" reference button's computed color EXACTLY
  //    (confirmed live), but that token is ITSELF a `color-mix(..., 11%,
  //    transparent)` blend (styles/theme.css) — invisible as translucency
  //    only because that reference button sits on a uniform toolbar
  //    background. An attachment's own content (a photo, a busy preview)
  //    is never uniform, so the SAME blend reads as genuinely see-through
  //    there — confirmed live, reported as "these things are still
  //    transparent."
  //
  // A truly OPAQUE neutral surface in this theme is `--popover`/`--card`
  // (`var(--background)` in dark mode, `var(--color-white)` in light — a
  // real solid color reference, no alpha channel at all — confirmed by
  // reading styles/theme.css directly, not assumed). `variant="outline"`
  // already uses `bg-popover` as its LIGHT-mode base; only its DARK-mode
  // override (`dark:bg-input/32`) and hover states are translucent, so
  // those are what get overridden here — kept as the outline base (its
  // border+shadow chrome) rather than switching variants again.
  it('is genuinely opaque in every state — no alpha-blended background utility survives, in either theme', () => {
    render(<Harness />)
    const handle = screen.getByRole('button', { name: /reorder/i })
    expect(handle.className).toContain('dark:bg-popover')
    expect(handle.className).toContain('dark:hover:bg-popover')
    // No stray alpha-sliced `bg-*/NN` utility left at all, under ANY variant
    // prefix (dark:, hover:, data-pressed:, ...) — this is the actual "is it
    // really opaque" contract, not just "did we remove the three specific
    // classes we already knew about."
    expect(handle.className).not.toMatch(/(?:^|\s)(?:[\w-]+:)*bg-[\w-]+\/\d+(?:\s|$)/)
  })
})

// REGRESSION, reported live: there was no way to select and remove an
// attachment at all — dragging could reorder one but nothing could delete
// one. Sits right under the drag handle (same left edge, stacked below it).
describe('AttachmentDeleteButton', () => {
  it('renders a delete affordance', () => {
    render(<AttachmentDeleteButton onDelete={vi.fn()} />)
    expect(screen.getByRole('button', { name: /remove this attachment/i })).toBeInTheDocument()
  })

  it('calls onDelete on click', () => {
    const onDelete = vi.fn()
    render(<AttachmentDeleteButton onDelete={onDelete} />)
    fireEvent.click(screen.getByRole('button', { name: /remove this attachment/i }))
    expect(onDelete).toHaveBeenCalledTimes(1)
  })

  it('uses the shared icon-xs button size token, not a one-off non-square size', () => {
    render(<AttachmentDeleteButton onDelete={vi.fn()} />)
    const button = screen.getByRole('button', { name: /remove this attachment/i })
    expect(button.className).toMatch(/(?:^|\s)size-7(?:\s|$)/)
    expect(button.className).not.toContain('h-6')
    expect(button.className).not.toContain('w-4')
  })

  it('is genuinely opaque in every state — no alpha-blended background utility survives, in either theme', () => {
    render(<AttachmentDeleteButton onDelete={vi.fn()} />)
    const button = screen.getByRole('button', { name: /remove this attachment/i })
    expect(button.className).toContain('dark:bg-popover')
    expect(button.className).toContain('dark:hover:bg-popover')
    expect(button.className).not.toMatch(/(?:^|\s)(?:[\w-]+:)*bg-[\w-]+\/\d+(?:\s|$)/)
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

/**
 * Regression coverage for I1: two file-card attachments each deserialize as
 * `[N] p -> [N, 0] a` (a link nested one level inside its own wrapping
 * paragraph) — `canDropAttachmentNode` compares `PathApi.parent(...)` on
 * whatever `@platejs/dnd` derives from the element handed to `useDraggable`,
 * so feeding it the raw, nested link always compared two DIFFERENT
 * paragraphs' paths and could never match. `resolveDraggableBlockElement`
 * walks each link up to its wrapping paragraph BEFORE that comparison runs.
 */
describe('resolveDraggableBlockElement', () => {
  it('returns the element unchanged when it is already a top-level block (a fence-based attachment)', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: [{ type: 'code_block', children: [{ type: 'code_line', children: [{ text: 'x' }] }] }],
    })
    const codeBlock = editor.children[0] as TElement

    expect(resolveDraggableBlockElement(editor, codeBlock)).toBe(codeBlock)
  })

  it('walks a nested link up to its wrapping paragraph (a file-card attachment)', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: [
        {
          type: 'p',
          children: [
            { type: 'a', url: 'chats/c1/attachments/a.pdf', children: [{ text: 'a.pdf' }] },
          ],
        },
      ],
    })
    const paragraph = editor.children[0] as TElement
    const link = paragraph.children[0] as TElement

    const resolved = resolveDraggableBlockElement(editor, link)

    expect(resolved).toBe(paragraph)
    expect(resolved).not.toBe(link)
    expect(editor.api.findPath(resolved)).toEqual([0])
  })

  it('falls back to the element unchanged when it has no path in the document at all', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: [{ type: 'p', children: [{ text: 'hi' }] }],
    })
    const detached: TElement = { type: 'a', children: [{ text: 'x' } as never] }

    expect(resolveDraggableBlockElement(editor, detached)).toBe(detached)
  })

  it('resolves two separate file-card links to top-level paragraphs whose parent path is the SAME — the actual I1 fix', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: [
        {
          type: 'p',
          children: [
            { type: 'a', url: 'chats/c1/attachments/a.pdf', children: [{ text: 'a.pdf' }] },
          ],
        },
        {
          type: 'p',
          children: [
            { type: 'a', url: 'chats/c1/attachments/b.pdf', children: [{ text: 'b.pdf' }] },
          ],
        },
      ],
    })
    const linkA = (editor.children[0] as TElement).children[0] as TElement
    const linkB = (editor.children[1] as TElement).children[0] as TElement

    const blockA = resolveDraggableBlockElement(editor, linkA)
    const blockB = resolveDraggableBlockElement(editor, linkB)
    const dragEntry = [blockA, editor.api.findPath(blockA)] as [TElement, number[]]
    const dropEntry = [blockB, editor.api.findPath(blockB)] as [TElement, number[]]

    // Before this fix, feeding the raw (unresolved) links in produced
    // dragEntry/dropEntry paths `[0, 0]` and `[1, 0]` — parents `[0]` and
    // `[1]`, never equal, so this always returned false. Resolved to their
    // wrapping paragraphs, both entries' parent is the document root `[]`.
    const result = canDropAttachmentNode({
      dragEntry,
      dropEntry,
    } as unknown as Parameters<typeof canDropAttachmentNode>[0])

    expect(result).toBe(true)
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
  it('forwards the element, its type, and the pure drop callbacks to useDraggable, and adds a remove()', () => {
    const draggableState = {
      isAboutToDrag: false,
      isDragging: false,
      nodeRef: { current: null },
      previewRef: { current: null },
      handleRef: vi.fn(),
    }
    vi.mocked(useDraggable).mockReturnValue(draggableState)
    // A type deliberately unrelated to `ATTACHMENT_DND_TYPE` — proves
    // `useDraggable` gets the shared dnd type below, NOT this element's own
    // Slate schema type (the whole point of unifying it: two attachments of
    // different Slate types, or a plain paragraph, all need to match).
    const element: TElement = { type: 'attachment_image', children: [] }

    const { result } = renderHook(() => useAttachmentDraggable(element), {
      wrapper: EditorWrapper,
    })

    expect(useDraggable).toHaveBeenCalledWith({
      element,
      type: ATTACHMENT_DND_TYPE,
      canDropNode: canDropAttachmentNode,
      onDropHandler: onAttachmentDropHandler,
      drag: { end: expect.any(Function) },
    })
    expect(result.current).toMatchObject(draggableState)
    expect(result.current.remove).toEqual(expect.any(Function))
  })

  // The trash button (AttachmentDeleteButton) calls this directly — it has
  // to remove the same RESOLVED block dragging operates on, not whatever
  // element the caller originally passed in (a file card passes its nested
  // link; the block to delete is its wrapping paragraph).
  it('remove() deletes the resolved block from the document', () => {
    vi.mocked(useDraggable).mockReturnValue({
      isAboutToDrag: false,
      isDragging: false,
      nodeRef: { current: null },
      previewRef: { current: null },
      handleRef: vi.fn(),
    })

    function TwoBlocksWrapper({ children }: { children: ReactNode }) {
      const editor = usePlateEditor({
        plugins: chatComposerPlugins,
        value: [
          { type: 'code_block', children: [{ type: 'code_line', children: [{ text: 'a' }] }] },
          { type: 'p', children: [{ text: 'keep me' }] },
        ],
      })
      return <Plate editor={editor}>{children}</Plate>
    }

    function useDraggableForFirstBlock() {
      const editor = useEditorRef()
      const codeBlock = editor.children[0] as TElement
      return { editor, result: useAttachmentDraggable(codeBlock) }
    }

    const { result } = renderHook(() => useDraggableForFirstBlock(), {
      wrapper: TwoBlocksWrapper,
    })

    result.current.result.remove()

    expect(result.current.editor.children).toHaveLength(1)
    expect(result.current.editor.children[0]).toMatchObject({ type: 'p' })
  })

  // The I1 fix, exercised through the hook's real wiring rather than
  // `resolveDraggableBlockElement` directly: a file card calls
  // `useAttachmentDraggable(props.element)` with the LINK, same as always —
  // the resolution to its wrapping paragraph has to happen INSIDE the hook.
  it('resolves a nested file-card link to its wrapping paragraph before forwarding to useDraggable', () => {
    vi.mocked(useDraggable).mockReturnValue({
      isAboutToDrag: false,
      isDragging: false,
      nodeRef: { current: null },
      previewRef: { current: null },
      handleRef: vi.fn(),
    })

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
        ],
      })
      return <Plate editor={editor}>{children}</Plate>
    }

    function useDraggableForNestedLink() {
      const editor = useEditorRef()
      const paragraph = editor.children[0] as TElement
      const link = paragraph.children[0] as TElement
      return { paragraph, result: useAttachmentDraggable(link) }
    }

    const { result } = renderHook(() => useDraggableForNestedLink(), {
      wrapper: NestedFileCardWrapper,
    })

    expect(useDraggable).toHaveBeenCalledWith(
      expect.objectContaining({
        element: result.current.paragraph,
        type: ATTACHMENT_DND_TYPE,
      }),
    )
  })

  // REGRESSION, reported live: after switching the app's drag backend from
  // `HTML5Backend` to `TouchBackend` (dnd-scope.tsx, to sidestep Tauri's own
  // OS-file-drop interception swallowing native drag events), the blue
  // drop-line indicator got stuck showing after a drag ended. Root cause:
  // `DndPlugin`'s own `handlers.onDragEnd` (the thing that normally resets
  // `dropTarget` to `{id: null, line: ''}`) is wired to the native DOM
  // `dragend` event — which `TouchBackend` never fires, since it drives
  // dragging entirely off plain mouse events instead of the native Drag and
  // Drop API. `useDrag`'s own `end` callback IS backend-agnostic (fires
  // whenever dnd-core ends a drag, regardless of which backend produced it),
  // so that's where this now has to be reset instead.
  it("clears isDragging and the stale drop-target line on drag end, since TouchBackend never fires the native dragend @platejs/dnd's own cleanup relies on", () => {
    vi.mocked(useDraggable).mockReturnValue({
      isAboutToDrag: false,
      isDragging: false,
      nodeRef: { current: null },
      previewRef: { current: null },
      handleRef: vi.fn(),
    })

    function useDraggableAndReturnEditor() {
      const editor = useEditorRef()
      const element = editor.children[0] as TElement
      useAttachmentDraggable(element)
      return editor
    }

    const { result } = renderHook(() => useDraggableAndReturnEditor(), { wrapper: EditorWrapper })
    const editor = result.current

    // Simulate the mid-drag state `onHoverNode` (@platejs/dnd) leaves behind
    // while hovering a sibling — this is what stays stuck without the fix.
    editor.setOption(DndPlugin, 'dropTarget', { id: 'some-id', line: 'top' })
    editor.setOption(DndPlugin, 'isDragging', true)
    document.body.classList.add('dragging')

    const dragOptions = vi.mocked(useDraggable).mock.calls.at(-1)?.[0]
    dragOptions?.drag?.end?.(undefined as never, undefined as never)

    expect(editor.getOption(DndPlugin, 'dropTarget')).toEqual({ id: null, line: '' })
    expect(editor.getOption(DndPlugin, 'isDragging')).toBe(false)
    expect(document.body.classList.contains('dragging')).toBe(false)
  })
})

/**
 * `useAttachmentDropTarget` is a plain paragraph's half of drag-and-drop:
 * droppable, never draggable. Its one real behavioural contract — beyond
 * forwarding to `useDropNode` — is staying safe with no `<DndProvider>`
 * ancestor at all, since (unlike `useDraggable`) `useDropNode` has no
 * `editor.plugins.dnd` short-circuit of its own, and EVERY message has plain
 * paragraphs.
 */
describe('useAttachmentDropTarget', () => {
  it('never calls useDropNode, and returns a plain ref, with no DndContext at all', () => {
    const element: TElement = { type: 'p', children: [] }

    const { result } = renderHook(() => useAttachmentDropTarget(element), {
      wrapper: EditorWrapper,
    })

    expect(useDropNode).not.toHaveBeenCalled()
    expect(result.current.nodeRef.current).toBeNull()
  })

  it('registers as a drop target accepting ATTACHMENT_DND_TYPE when a real DndContext is present', () => {
    vi.mocked(useDropNode).mockReturnValue([{ isOver: false }, vi.fn()])
    const element: TElement = { type: 'p', children: [] }
    const fakeManager = {} as DndContextType['dragDropManager']

    function WrapperWithDndContext({ children }: { children: ReactNode }) {
      return (
        <DndContext.Provider value={{ dragDropManager: fakeManager }}>
          <EditorWrapper>{children}</EditorWrapper>
        </DndContext.Provider>
      )
    }

    renderHook(() => useAttachmentDropTarget(element), { wrapper: WrapperWithDndContext })

    expect(useDropNode).toHaveBeenCalledWith(
      expect.anything(),
      expect.objectContaining({
        accept: [ATTACHMENT_DND_TYPE],
        canDropNode: canDropAttachmentNode,
        element,
        onDropHandler: onAttachmentDropHandler,
      }),
    )
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
