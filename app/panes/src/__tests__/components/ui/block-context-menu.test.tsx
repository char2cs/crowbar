import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { KEYS } from 'platejs'
import { Plate, PlateContent, type PlateEditor, usePlateEditor } from 'platejs/react'
import { BlockMenuKit } from '@/components/editor/plugins/block-menu-kit'
import type { ContextMenuItem } from '@/components/ui/context-menu'

type BlockNode = {
  id?: string
  type?: string
  indent?: number
  align?: string
  children?: [{ text?: string }]
}

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

async function selectFirstBlock() {
  const block = screen.getByText('first').closest('[data-slate-node="element"]') as HTMLElement
  fireEvent.contextMenu(block)
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

    expect(menuItemLabels()).toEqual([
      'Delete',
      'Duplicate',
      'Turn into',
      'Indent',
      'Outdent',
      'Align',
    ])
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
    const firstTexts = editor.children.filter(
      (n) => (n as BlockNode).children?.[0]?.text === 'first',
    )
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
