import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { KEYS } from 'platejs'
import { Plate, PlateContent, type PlateEditor, usePlateEditor } from 'platejs/react'
import { BlockMenuKit } from '@/components/editor/plugins/block-menu-kit'
import type { ContextMenuItem } from '@/components/ui/context-menu'

type BlockNode = { id?: string; type?: string; indent?: number; align?: string; children?: [{ text?: string }] }

// Base UI's hover-to-open submenu (Turn into / Align) genuinely races its own
// auto-close under jsdom: there's no real layout, so every anchor rect
// (including the submenu trigger's) is 0×0×0×0, and the pending open — which
// only resolves asynchronously — loses that race to Base UI's own close path
// often enough to make a `fireEvent.pointerEnter`/`findByText('Heading 1')`
// gesture flaky in THIS harness specifically (heavier module graph than
// context-menu.test.tsx's bare `<ContextMenu>` render, so more real
// wall-clock time elapses before `findByText`'s polling wins the race —
// confirmed live: `console.trace`'d `handleClose` fired mid-`findByText`,
// called from Base UI's `MenuPositioner`, not from any click of ours). That's
// a jsdom/Base UI timing quirk, not something `block-context-menu.tsx`
// controls, so submenu wiring is verified a different way below: capture the
// `items` array `BlockContextMenu` passes to the shared `ContextMenu`, and
// invoke a nested item's `onClick` directly — still the real
// `BlockSelectionPlugin` transform, just reached without depending on Base
// UI's hover timing.
const { capturedItems } = vi.hoisted(() => ({
  capturedItems: { current: null as ContextMenuItem[] | null },
}))

vi.mock('@/components/ui/context-menu', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/components/ui/context-menu')>()
  return {
    ...actual,
    ContextMenu: (props: Parameters<typeof actual.ContextMenu>[0]) => {
      capturedItems.current = props.items
      return actual.ContextMenu(props)
    },
  }
})

type TouchNavigator = Navigator & { maxTouchPoints: number }

// jsdom defines its own `ontouchstart` handler property on `window`
// unconditionally — unlike a real non-touch desktop browser, where
// `'ontouchstart' in window` is false — so `useIsTouchDevice`'s touch check
// (`'ontouchstart' in window || navigator.maxTouchPoints > 0`) would read
// true in every test here regardless of `maxTouchPoints`. Deleting it before
// each test (and restoring the original descriptor after) makes this suite
// match a real desktop browser's shape, leaving `maxTouchPoints` as the one
// lever the touch-device test below needs.
const ontouchstartDescriptor = Object.getOwnPropertyDescriptor(window, 'ontouchstart')

beforeEach(() => {
  delete (window as { ontouchstart?: unknown }).ontouchstart
})

afterEach(() => {
  cleanup()
  ;(navigator as TouchNavigator).maxTouchPoints = 0
  if (ontouchstartDescriptor) Object.defineProperty(window, 'ontouchstart', ontouchstartDescriptor)
  capturedItems.current = null
})

// `BlockContextMenu` is wired in via `BlockMenuPlugin`'s `render.aboveEditable`
// (block-menu-kit.tsx), which `<PlateContent>` is what actually invokes
// (`@platejs/core`'s `PlateContent.tsx` walks `render.aboveEditable` itself —
// bare `<Plate>` never does). So the harness has to mount `<PlateContent>`,
// not a stand-in `<div>`, or `BlockContextMenu` never renders at all and every
// assertion below would pass vacuously.
//
// `editorRef` is populated during render (mirroring how `usePlateEditor`
// returns a stable instance) so the transform tests below can read
// `editor.children` straight from the document instead of only checking
// which menu items are visible.
function Harness({ editorRef }: { editorRef: { current: PlateEditor | null } }) {
  const editor = usePlateEditor({
    plugins: BlockMenuKit,
    value: [
      { type: 'p', id: 'block-1', children: [{ text: 'first' }] },
      { type: 'p', id: 'block-2', children: [{ text: 'second' }] },
    ],
  })
  editorRef.current = editor
  return (
    <Plate editor={editor}>
      <PlateContent />
    </Plate>
  )
}

// Right-clicks the DOM node for the first block (its `<p>`, tagged by Slate
// with `data-slate-node="element"`) — a real block, distinct from the
// editable root itself (`data-slate-editor="true"`, covered separately below).
async function selectFirstBlock() {
  const block = screen.getByText('first').closest('[data-slate-node="element"]') as HTMLElement
  fireEvent.contextMenu(block)
  // `BlockContextMenu` defers `api.blockMenu.show`/`openAt` by a
  // `setTimeout(0)` (kept from the pre-migration Radix implementation, which
  // deferred the same way) — flush that one pending macrotask before
  // asserting on the now-open menu.
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0))
  })
}

const menuItemLabels = () =>
  Array.from(document.querySelectorAll('[role="menuitem"]')).map((el) => el.textContent)

function renderHarness() {
  const editorRef: { current: PlateEditor | null } = { current: null }
  render(<Harness editorRef={editorRef} />)
  return editorRef
}

// Finds a nested item (e.g. one of "Turn into"'s or "Align"'s `items`) in the
// captured `items` array and invokes its `onClick` directly — see the
// `vi.mock` comment above for why this bypasses the rendered submenu's DOM.
function clickNestedItem(parentId: string, itemId: string) {
  const parent = capturedItems.current!.find((item) => item.id === parentId)!
  const nested = parent.items!.find((item) => item.id === itemId)!
  act(() => {
    nested.onClick()
  })
}

const findBlock = (editor: PlateEditor, id: string) =>
  editor.children.find((n) => (n as BlockNode).id === id) as BlockNode

describe('BlockContextMenu', () => {
  it('shows Delete / Duplicate / Turn into / Indent / Outdent / Align on right-click', async () => {
    renderHarness()

    await selectFirstBlock()

    expect(menuItemLabels()).toEqual(['Delete', 'Duplicate', 'Turn into', 'Indent', 'Outdent', 'Align'])
  })

  it('renders no menu at all on a touch device', async () => {
    ;(navigator as TouchNavigator).maxTouchPoints = 1
    renderHarness()
    fireEvent(window, new Event('resize'))

    await selectFirstBlock()

    expect(menuItemLabels()).toEqual([])
    expect(screen.getByText('first')).toBeInTheDocument()
  })

  it('does not open when the click target is the slate editor itself', () => {
    renderHarness()
    const editorRoot = document.querySelector('[data-slate-editor="true"]') as HTMLElement

    fireEvent.contextMenu(editorRoot)

    expect(menuItemLabels()).toEqual([])
  })
})

// The item-label test above only proves the menu renders the right names —
// it says nothing about whether each item is wired to the right editor
// transform. These exercise the real `BlockSelectionPlugin` transforms
// `block-context-menu.tsx` calls, reading the result straight off
// `editor.children` rather than off the (now-closed) menu's own DOM.
describe('BlockContextMenu item transforms', () => {
  it('Delete removes the right-clicked block', async () => {
    const editorRef = renderHarness()
    await selectFirstBlock()

    fireEvent.click(screen.getByText('Delete'))

    const editor = editorRef.current!
    expect(editor.children).toHaveLength(1)
    expect(findBlock(editor, 'block-2')).toBeDefined()
  })

  it('Duplicate adds a copy of the right-clicked block', async () => {
    const editorRef = renderHarness()
    await selectFirstBlock()

    fireEvent.click(screen.getByText('Duplicate'))

    const editor = editorRef.current!
    expect(editor.children).toHaveLength(3)
    const firstTexts = editor.children.filter((n) => (n as BlockNode).children?.[0]?.text === 'first')
    expect(firstTexts).toHaveLength(2)
  })

  it('Turn into -> Heading 1 changes the block type', async () => {
    const editorRef = renderHarness()
    await selectFirstBlock()

    clickNestedItem('turn-into', 'turn-into-h1')

    expect(findBlock(editorRef.current!, 'block-1').type).toBe(KEYS.h1)
  })

  it('Indent increases the block indent', async () => {
    const editorRef = renderHarness()
    await selectFirstBlock()

    fireEvent.click(screen.getByText('Indent'))

    expect(findBlock(editorRef.current!, 'block-1').indent).toBe(1)
  })

  it('Outdent decreases the block indent', async () => {
    const editorRef = renderHarness()
    act(() => {
      editorRef.current!.tf.setNodes({ indent: 2 }, { at: [0] })
    })
    await selectFirstBlock()

    fireEvent.click(screen.getByText('Outdent'))

    expect(findBlock(editorRef.current!, 'block-1').indent).toBe(1)
  })

  it('Align -> Center sets the block alignment', async () => {
    const editorRef = renderHarness()
    await selectFirstBlock()

    clickNestedItem('align', 'align-center')

    expect(findBlock(editorRef.current!, 'block-1').align).toBe('center')
  })
})
