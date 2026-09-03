/**
 * Contract pins for a grouping folder in the Chats tree.
 *
 * Same treeitem/drop/indent contract AgentChatRow pins (they are visual
 * siblings, built from the same ROW_BASE tokens so the two interleave without a
 * seam), minus the two things only a chat has (own turn state, `kept`) and plus
 * the two things only a folder has: it toggles on a plain row click rather than
 * selecting, and it carries its own "+" (new child folder) control.
 */
import { fireEvent, render, screen, within } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { AgentChatFolderRow } from '@/features/agent/tree/agent-chat-folder-row'
import { ADD_GLYPH_PATH } from '@/components/layout/workspace-row-base'
import { ROW_INDENT_STEP, ROW_NEST_TARGET } from '@/components/layout/workspace-row-base'

const base = {
  id: 'f1',
  name: 'Spikes',
  depth: 0,
  parentId: 'p1',
  path: '/p1/',
  expanded: false,
  hasChildren: false,
  holding: false,
  renaming: false,
  dragging: false,
  nesting: false,
  selected: false,
  ctx: false,
  query: '',
  onSelect: vi.fn(),
  onToggle: vi.fn(),
  onFoldAway: vi.fn(),
  onStartRename: vi.fn(),
  onConfirmRename: vi.fn(),
  onCancelRename: vi.fn(),
  onAddChat: vi.fn(),
  onPointerDownDrag: vi.fn(),
}

const row = () => screen.getByRole('treeitem')
const indentBox = () => row().parentElement!
/** The leading glyph's first path — Folder and FolderOpen are two whole marks
 *  swapped, not one tweening between them, so comparing the outline is how a
 *  test tells them apart without knowing Phosphor's internal path data. */
const glyphPath = () => row().querySelector('svg > path')!.getAttribute('d')

describe('AgentChatFolderRow — treeitem semantics', () => {
  it('is a treeitem carrying its own aria-expanded', () => {
    render(<AgentChatFolderRow {...base} expanded={false} />)
    expect(row()).toHaveAttribute('role', 'treeitem')
    expect(row()).toHaveAttribute('aria-expanded', 'false')
  })

  it('reports aria-expanded=true once open', () => {
    render(<AgentChatFolderRow {...base} expanded />)
    expect(row()).toHaveAttribute('aria-expanded', 'true')
  })

  it('reflects the ⌘-click multiselection in aria-selected', () => {
    const { rerender } = render(<AgentChatFolderRow {...base} selected={false} />)
    expect(row()).toHaveAttribute('aria-selected', 'false')
    rerender(<AgentChatFolderRow {...base} selected />)
    expect(row()).toHaveAttribute('aria-selected', 'true')
  })
})

describe('AgentChatFolderRow — drop attributes', () => {
  it('publishes the folder id, parent, path, and the presence flags', () => {
    render(<AgentChatFolderRow {...base} hasChildren expanded />)
    expect(row()).toHaveAttribute('data-chat-folder-drop', 'f1')
    expect(row()).toHaveAttribute('data-chat-parent', 'p1')
    expect(row()).toHaveAttribute('data-chat-path', '/p1/')
    expect(row()).toHaveAttribute('data-chat-expanded', '')
    expect(row()).toHaveAttribute('data-chat-children', '')
    // A chat row's id attribute belongs to a different kind entirely — the two
    // trees' rows must stay mutually invisible.
    expect(row()).not.toHaveAttribute('data-chat-drop')
  })

  it('omits the expanded/children flags when neither is true', () => {
    render(<AgentChatFolderRow {...base} hasChildren={false} expanded={false} />)
    expect(row()).not.toHaveAttribute('data-chat-expanded')
    expect(row()).not.toHaveAttribute('data-chat-children')
  })
})

describe('AgentChatFolderRow — indent', () => {
  it('sits flush at depth 0', () => {
    render(<AgentChatFolderRow {...base} />)
    expect(indentBox().style.marginInlineStart).toBe('0px')
  })

  it('steps one ROW_INDENT_STEP per level', () => {
    render(<AgentChatFolderRow {...base} depth={3} />)
    expect(indentBox().style.marginInlineStart).toBe(`${3 * ROW_INDENT_STEP}px`)
  })

  it('indenting does not change the row height', () => {
    // Same fixed-pitch virtualizer contract the chat row keeps: a wrapper with
    // vertical box would desynchronise the drop geometry from the paint.
    const { rerender } = render(<AgentChatFolderRow {...base} depth={0} />)
    const flat = row().getBoundingClientRect().height
    rerender(<AgentChatFolderRow {...base} depth={5} />)
    expect(row().getBoundingClientRect().height).toBe(flat)
  })
})

describe('AgentChatFolderRow — visual state tokens', () => {
  it('paints ROW_NEST_TARGET when a drop would land inside it', () => {
    render(<AgentChatFolderRow {...base} nesting />)
    for (const token of ROW_NEST_TARGET.split(' ')) expect(row().className).toContain(token)
  })

  it('falls back to the inactive treatment when not nesting', () => {
    render(<AgentChatFolderRow {...base} />)
    expect(row().className).toContain('hover:bg-accent')
  })

  it('fades the source row while dragging', () => {
    render(<AgentChatFolderRow {...base} dragging />)
    expect(row().className).toContain('opacity-40')
  })

  it('does not fade an ordinary row', () => {
    render(<AgentChatFolderRow {...base} />)
    expect(row().className).not.toContain('opacity-40')
  })

  it('dims a row kept only as search context', () => {
    render(<AgentChatFolderRow {...base} ctx />)
    expect(row().className).toContain('opacity-45')
  })

  it('does not dim an ordinary row', () => {
    render(<AgentChatFolderRow {...base} />)
    expect(row().className).not.toContain('opacity-45')
  })
})

describe('AgentChatFolderRow — glyph', () => {
  it('swaps between two marks for open and closed rather than tweening one', () => {
    // At 16px an open folder is a few pixels of shear from a closed one, so a
    // tween would spend its whole duration saying what the swap says at once.
    const { rerender } = render(<AgentChatFolderRow {...base} expanded={false} />)
    const closedPath = glyphPath()
    rerender(<AgentChatFolderRow {...base} expanded />)
    expect(glyphPath()).not.toBe(closedPath)
  })

  it('draws the holding dots inside the closed glyph when holding', () => {
    render(<AgentChatFolderRow {...base} expanded={false} holding />)
    expect(row().querySelector('[data-holding-rows]')).not.toBeNull()
  })

  it('draws no dots when not holding', () => {
    render(<AgentChatFolderRow {...base} expanded={false} holding={false} />)
    expect(row().querySelector('[data-holding-rows]')).toBeNull()
  })

  it('never draws dots on the open glyph — a row can hold others only while folded', () => {
    render(<AgentChatFolderRow {...base} expanded holding />)
    expect(row().querySelector('[data-holding-rows]')).toBeNull()
  })
})

describe('AgentChatFolderRow — click toggles, ⌘-click selects', () => {
  it('a plain click toggles — a folder has nowhere to open', () => {
    const onToggle = vi.fn()
    const onSelect = vi.fn()
    render(<AgentChatFolderRow {...base} onToggle={onToggle} onSelect={onSelect} />)
    fireEvent.click(row())
    expect(onToggle).toHaveBeenCalledWith('f1')
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('⌘-click selects and does NOT also fold/toggle the row it is collecting', () => {
    const onSelect = vi.fn()
    const onToggle = vi.fn()
    render(<AgentChatFolderRow {...base} onSelect={onSelect} onToggle={onToggle} />)
    fireEvent.click(row(), { metaKey: true })
    expect(onSelect).toHaveBeenCalledWith('f1')
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('Ctrl-click is the same selection gesture on platforms without ⌘', () => {
    const onSelect = vi.fn()
    render(<AgentChatFolderRow {...base} onSelect={onSelect} />)
    fireEvent.click(row(), { ctrlKey: true })
    expect(onSelect).toHaveBeenCalledWith('f1')
  })

  it('while renaming, a click neither toggles nor selects', () => {
    const onToggle = vi.fn()
    const onSelect = vi.fn()
    render(<AgentChatFolderRow {...base} renaming onToggle={onToggle} onSelect={onSelect} />)
    fireEvent.click(row())
    fireEvent.click(row(), { metaKey: true })
    expect(onToggle).not.toHaveBeenCalled()
    expect(onSelect).not.toHaveBeenCalled()
  })
})

describe('AgentChatFolderRow — drag', () => {
  // The row hands over the SUBJECT, not its id — see AgentChatRow's own test for
  // the memo this protects.
  it('pointer-down forwards this row’s own drag subject', () => {
    const onPointerDownDrag = vi.fn()
    render(<AgentChatFolderRow {...base} parentId="p9" onPointerDownDrag={onPointerDownDrag} />)
    fireEvent.pointerDown(row())
    expect(onPointerDownDrag).toHaveBeenCalledTimes(1)
    expect(onPointerDownDrag.mock.calls[0][0]).toEqual({
      kind: 'chatFolder',
      id: 'f1',
      parentId: 'p9',
    })
  })

  it('does not start a drag while renaming', () => {
    const onPointerDownDrag = vi.fn()
    render(<AgentChatFolderRow {...base} renaming onPointerDownDrag={onPointerDownDrag} />)
    fireEvent.pointerDown(row())
    expect(onPointerDownDrag).not.toHaveBeenCalled()
  })
})

describe('AgentChatFolderRow — keyboard', () => {
  it('Enter on the focused row toggles', () => {
    const onToggle = vi.fn()
    render(<AgentChatFolderRow {...base} onToggle={onToggle} />)
    fireEvent.keyDown(row(), { key: 'Enter' })
    expect(onToggle).toHaveBeenCalledWith('f1')
  })

  it('Space on the focused row toggles', () => {
    const onToggle = vi.fn()
    render(<AgentChatFolderRow {...base} onToggle={onToggle} />)
    fireEvent.keyDown(row(), { key: ' ' })
    expect(onToggle).toHaveBeenCalledWith('f1')
  })

  it('ignores other keys', () => {
    const onToggle = vi.fn()
    render(<AgentChatFolderRow {...base} onToggle={onToggle} />)
    fireEvent.keyDown(row(), { key: 'Tab' })
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('a keydown bubbling from a child (the "+" control) does not toggle', () => {
    const onToggle = vi.fn()
    render(<AgentChatFolderRow {...base} onToggle={onToggle} />)
    const addButton = within(row()).getByLabelText('New chat in Spikes')
    fireEvent.keyDown(addButton, { key: 'Enter', bubbles: true })
    expect(onToggle).not.toHaveBeenCalled()
  })
})

describe('AgentChatFolderRow — holding', () => {
  it('draws a fold-away control when holding', () => {
    render(<AgentChatFolderRow {...base} holding />)
    expect(within(row()).getByLabelText(/Fold away/)).toBeTruthy()
  })

  it('draws no fold-away control when not holding', () => {
    render(<AgentChatFolderRow {...base} />)
    expect(screen.queryByLabelText(/Fold away/)).toBeNull()
  })

  it('pressing fold-away calls onFoldAway with the row id, and neither selects nor toggles', () => {
    const onFoldAway = vi.fn()
    const onSelect = vi.fn()
    const onToggle = vi.fn()
    render(
      <AgentChatFolderRow
        {...base}
        holding
        onFoldAway={onFoldAway}
        onSelect={onSelect}
        onToggle={onToggle}
      />,
    )
    fireEvent.click(within(row()).getByLabelText(/Fold away/))
    expect(onFoldAway).toHaveBeenCalledWith('f1')
    expect(onSelect).not.toHaveBeenCalled()
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('keeps the "+" — a new chat in a folder genuinely IS an add', () => {
    // Only the CHAT row's control changed to the reply-in-thread elbow. A folder
    // holds no turns for a new chat to read, so "one more of these" is exactly
    // what its control does, and "+" is exactly what that means.
    render(<AgentChatFolderRow {...base} />)
    const add = within(row()).getByLabelText('New chat in Spikes')
    expect(add.querySelector(`path[d="${ADD_GLYPH_PATH}"]`)).not.toBeNull()
    expect(add.querySelector('[data-thread-glyph]')).toBeNull()
  })

  it('is still a full drop target, and still arms the drag, while it is holding', () => {
    // A folder that has folded over a chat you are reading is not a different
    // kind of row. This row has never had a `kept`-style gate on its attributes
    // and must not grow one: the chat row had exactly that, and it made the
    // hoisted row inert.
    const onPointerDownDrag = vi.fn()
    render(<AgentChatFolderRow {...base} holding onPointerDownDrag={onPointerDownDrag} />)

    expect(row()).toHaveAttribute('data-chat-folder-drop', 'f1')
    expect(row()).toHaveAttribute('data-chat-parent', 'p1')
    expect(row()).toHaveAttribute('data-chat-path', '/p1/')

    fireEvent.pointerDown(row(), { button: 0 })
    expect(onPointerDownDrag).toHaveBeenCalledWith(
      { kind: 'chatFolder', id: 'f1', parentId: 'p1' },
      expect.anything(),
    )
  })
})

describe('AgentChatFolderRow — disclosure', () => {
  it('renders the chevron only when it has children', () => {
    render(<AgentChatFolderRow {...base} hasChildren={false} />)
    expect(screen.queryByLabelText(/Expand|Collapse/)).toBeNull()
  })

  it('pressing the chevron toggles without selecting or starting a drag', () => {
    const onToggle = vi.fn()
    const onSelect = vi.fn()
    const onPointerDownDrag = vi.fn()
    render(
      <AgentChatFolderRow
        {...base}
        hasChildren
        onToggle={onToggle}
        onSelect={onSelect}
        onPointerDownDrag={onPointerDownDrag}
      />,
    )
    fireEvent.click(within(row()).getByLabelText('Expand'))
    expect(onToggle).toHaveBeenCalledWith('f1')
    expect(onSelect).not.toHaveBeenCalled()
    expect(onPointerDownDrag).not.toHaveBeenCalled()
  })
})

describe('AgentChatFolderRow — new child folder', () => {
  // A CHAT, not a folder: "+" on a row means "add a child of the tree's primary
  // kind", and the primary kind here is a conversation. Folders are made around
  // a selection, from the right-click menu.
  it('the "+" control starts a chat inside this folder, without toggling it', () => {
    const onAddChat = vi.fn()
    const onToggle = vi.fn()
    render(<AgentChatFolderRow {...base} onAddChat={onAddChat} onToggle={onToggle} />)
    fireEvent.click(within(row()).getByLabelText('New chat in Spikes'))
    expect(onAddChat).toHaveBeenCalledWith('f1')
    // Its click stops propagation — the row beneath it must not also toggle.
    expect(onToggle).not.toHaveBeenCalled()
  })

  it('its pointer-down does not reach the row and arm a drag', () => {
    const onPointerDownDrag = vi.fn()
    render(<AgentChatFolderRow {...base} onPointerDownDrag={onPointerDownDrag} />)
    fireEvent.pointerDown(within(row()).getByLabelText('New chat in Spikes'))
    expect(onPointerDownDrag).not.toHaveBeenCalled()
  })
})

describe('AgentChatFolderRow — rename', () => {
  it('double-click opens the inline input seeded with the current name', () => {
    const onStartRename = vi.fn()
    render(<AgentChatFolderRow {...base} onStartRename={onStartRename} />)
    fireEvent.doubleClick(row())
    expect(onStartRename).toHaveBeenCalledWith('f1')
  })

  it('renders the input seeded with the name while renaming', () => {
    render(<AgentChatFolderRow {...base} renaming />)
    expect(screen.getByDisplayValue('Spikes')).toBeTruthy()
    expect(screen.queryByText('Spikes')).toBeNull()
  })

  it('names in the UI face, not the chat/branch rows’ mono — a folder name is prose', () => {
    render(<AgentChatFolderRow {...base} renaming />)
    expect(screen.getByDisplayValue('Spikes').className).not.toContain('font-mono')
  })

  it('confirming calls onConfirmRename with the folder id and the new name', () => {
    const onConfirmRename = vi.fn()
    render(<AgentChatFolderRow {...base} renaming onConfirmRename={onConfirmRename} />)
    const input = screen.getByDisplayValue('Spikes')
    fireEvent.change(input, { target: { value: 'Renamed folder' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onConfirmRename).toHaveBeenCalledWith('f1', 'Renamed folder')
  })

  it('cancelling (Escape) calls onCancelRename', () => {
    const onCancelRename = vi.fn()
    render(<AgentChatFolderRow {...base} renaming onCancelRename={onCancelRename} />)
    fireEvent.keyDown(screen.getByDisplayValue('Spikes'), { key: 'Escape' })
    expect(onCancelRename).toHaveBeenCalledTimes(1)
  })

  it('never marks a match inside the editor', () => {
    render(<AgentChatFolderRow {...base} name="Drag rules" query="rul" renaming />)
    expect(row().querySelector('mark')).toBeNull()
    expect(screen.getByRole('textbox')).toHaveValue('Drag rules')
  })

  // Regression: the handler used to run unguarded. The editor is a nested
  // <input> that does not stop `dblclick`, so double-clicking a WORD inside it
  // bubbled to the row and re-opened the rename that was already open —
  // throwing the caret back to the start of the field mid-edit.
  it('double-click while renaming does not re-open the editor', () => {
    const onStartRename = vi.fn()
    render(<AgentChatFolderRow {...base} renaming onStartRename={onStartRename} />)
    fireEvent.doubleClick(row())
    fireEvent.doubleClick(screen.getByDisplayValue('Spikes'))
    expect(onStartRename).not.toHaveBeenCalled()
  })
})

describe('AgentChatFolderRow — search highlight', () => {
  it('marks the matched substring', () => {
    render(<AgentChatFolderRow {...base} name="Drag rules" query="rul" />)
    expect(row().querySelector('mark')!.textContent).toBe('rul')
  })

  it('keeps the name’s own casing under the mark', () => {
    render(<AgentChatFolderRow {...base} name="Drag rules" query="DRAG" />)
    expect(row().querySelector('mark')!.textContent).toBe('Drag')
  })

  it('renders the whole name when the query does not hit it', () => {
    render(<AgentChatFolderRow {...base} name="Drag rules" query="zzz" />)
    expect(row().querySelector('mark')).toBeNull()
    expect(row().textContent).toContain('Drag rules')
  })

  it('renders no mark with no query', () => {
    render(<AgentChatFolderRow {...base} name="Drag rules" />)
    expect(row().querySelector('mark')).toBeNull()
  })
})
