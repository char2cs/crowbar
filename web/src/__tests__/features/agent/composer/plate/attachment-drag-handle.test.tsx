import type { ReactNode } from 'react'
import { fireEvent, render, renderHook, screen } from '@testing-library/react'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Active, Over } from '@dnd-kit/core'
import type { TElement } from 'platejs'
import { createPlateEditor, Plate, useEditorRef, usePlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import {
  AttachmentDeleteButton,
  AttachmentDragHandle,
  AttachmentDropLine,
  attachmentDropMove,
  canDropAttachmentNode,
  resolveAttachmentDrop,
  resolveDraggableBlockElement,
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

function Harness() {
  const editor = usePlateEditor({
    plugins: chatComposerPlugins,
    value: [{ type: 'p', children: [{ text: 'hi' }] }],
  })
  return (
    <Plate editor={editor}>
      <AttachmentDragHandle onSelect={() => editor.tf.select(editor.children[0]!)} />
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
    render(<AttachmentDragHandle onSelect={onSelect} />)
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

describe('canDropAttachmentNode', () => {
  it('returns true for a drop target that is a same-parent sibling', () => {
    expect(canDropAttachmentNode([0, 1], [0, 2])).toBe(true)
  })

  it('returns false for a drop target under a different parent', () => {
    expect(canDropAttachmentNode([0, 1], [1, 0])).toBe(false)
  })
})

/**
 * Regression coverage for I1: two file-card attachments each deserialize as
 * `[N] p -> [N, 0] a` (a link nested one level inside its own wrapping
 * paragraph) — `canDropAttachmentNode` compares parents, so feeding it the
 * raw, nested link always compared two DIFFERENT paragraphs' paths and could
 * never match. `resolveDraggableBlockElement` walks each link up to its
 * wrapping paragraph BEFORE that comparison runs.
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
    // Before this fix, the raw (unresolved) links had paths `[0, 0]` and
    // `[1, 0]` — parents `[0]` and `[1]`, never equal. Resolved to their
    // wrapping paragraphs, both parents are the document root `[]`.
    expect(canDropAttachmentNode(editor.api.findPath(blockA)!, editor.api.findPath(blockB)!)).toBe(
      true,
    )
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
    ],
  })
  const [a, attachment, b, c] = editor.children as TElement[]
  return { editor, a: a!, attachment: attachment!, b: b!, c: c! }
}

describe('attachmentDropMove', () => {
  it('moves below a later sibling', () => {
    const { editor, attachment, c } = threeBlocks()
    expect(
      attachmentDropMove({ editor, element: attachment }, { editor, element: c }, 'bottom'),
    ).toEqual({ at: [1], to: [3] })
  })

  it('moves above an earlier sibling', () => {
    const { editor, attachment, a } = threeBlocks()
    expect(
      attachmentDropMove({ editor, element: attachment }, { editor, element: a }, 'top'),
    ).toEqual({ at: [1], to: [0] })
  })

  it('is a no-op (null) where the block already sits: the top of the next block, the bottom of the previous one', () => {
    const { editor, attachment, a, b } = threeBlocks()
    expect(
      attachmentDropMove({ editor, element: attachment }, { editor, element: b }, 'top'),
    ).toBeNull()
    expect(
      attachmentDropMove({ editor, element: attachment }, { editor, element: a }, 'bottom'),
    ).toBeNull()
  })

  it('refuses a drop on itself or into another editor', () => {
    const { editor, attachment } = threeBlocks()
    const other = threeBlocks()
    expect(
      attachmentDropMove({ editor, element: attachment }, { editor, element: attachment }, 'top'),
    ).toBeNull()
    expect(
      attachmentDropMove(
        { editor, element: attachment },
        { editor: other.editor, element: other.c },
        'top',
      ),
    ).toBeNull()
  })
})

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
