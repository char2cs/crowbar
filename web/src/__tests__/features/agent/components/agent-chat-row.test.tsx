/**
 * Contract pins for a chat row in the Chats tree.
 *
 * A chat may hold other chats (a child is a THREAD), and rows interleave with
 * folder rows drawn by AgentChatFolderRow — so what is pinned here is the same
 * treeitem/drop/indent contract that file pins for folders, plus the two things
 * that are this row's own: it reads its OWN turn state straight from the
 * workspace store (never a `working` prop), and a hoisted row — pulled out of a
 * folded ancestor because it is on screen — draws itself without a home to drop
 * beside.
 */
import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { AgentChatRow } from '@/features/agent/components/agent-chat-row'
import { ADD_GLYPH_PATH } from '@/components/layout/workspace-row-base'
import {
  ROW_ACTIVE,
  ROW_INDENT_STEP,
  ROW_NEST_TARGET,
} from '@/components/layout/workspace-row-base'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'

// The row reads its OWN turn state from the workspace store (like
// AgentChatTabIcon) rather than a `working` prop, so drive "is working" through
// the store, not props. Keep the store out of IndexedDB.
vi.mock('@/lib/persistence/workspace-layout', () => ({
  saveWorkspaceLayout: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/features/editor/stores/buffer-session-persistence', () => ({
  saveSessionToStore: vi.fn(),
  clearQueuedWorkspaceSessionSave: vi.fn(),
}))

const base = {
  wsId: 'w1',
  chatId: 'c1',
  title: 'My chat',
  providerIcon: '<svg data-icon="claude"/>',
  active: false,
  renaming: false,
  dragging: false,
  nesting: false,
  selected: false,
  depth: 0,
  parentId: 'p1',
  path: '/p1/',
  expanded: false,
  hasChildren: false,
  holding: false,
  ctx: false,
  query: '',
  onSelect: vi.fn(),
  onToggle: vi.fn(),
  onFoldAway: vi.fn(),
  onStartRename: vi.fn(),
  onConfirmRename: vi.fn(),
  onCancelRename: vi.fn(),
  onNewThread: vi.fn(),
  onPointerDownDrag: vi.fn(),
}

const setWorking = (chatId: string, working: boolean) =>
  act(() => getOrCreateWorkspaceStore('w1').getState().setAgentChatWorking(chatId, working))

// The row itself, whether or not it happens to publish drop attributes — the
// treeitem role is unconditional, so it is the one selector every test can rely
// on regardless of `kept`.
const row = () => screen.getByRole('treeitem')
const indentBox = () => row().parentElement!

beforeEach(() => {
  getOrCreateWorkspaceStore('w1')
})

afterEach(() => {
  cleanup()
  destroyWorkspaceStore('w1')
  vi.clearAllMocks()
})

describe('AgentChatRow — glyph and own turn state', () => {
  it('renders the provider icon when idle and the spinner when its own chat is working', () => {
    const { container } = render(<AgentChatRow {...base} />)
    expect(container.querySelector('[data-icon="claude"]')).not.toBeNull()
    expect(screen.queryByRole('status')).toBeNull()

    // Working state comes from the store; the row self-subscribes and swaps in
    // the spinner without any prop change.
    setWorking('c1', true)
    expect(screen.getByRole('status')).toBeTruthy()
    expect(container.querySelector('[data-icon="claude"]')).toBeNull()
  })

  it('re-renders only for its OWN chat’s working state, not a sibling’s', () => {
    // Load-bearing for a long list: a turn_started/turn_stopped frame is the
    // hottest thing on the chat feed, and the narrow store selector must skip
    // every row it does not concern.
    const { container } = render(<AgentChatRow {...base} />)
    setWorking('c2', true)
    expect(screen.queryByRole('status')).toBeNull()
    expect(container.querySelector('[data-icon="claude"]')).not.toBeNull()
  })
})

describe('AgentChatRow — treeitem semantics', () => {
  it('is a treeitem with no aria-expanded when it is a leaf', () => {
    render(<AgentChatRow {...base} hasChildren={false} expanded />)
    expect(row()).toHaveAttribute('role', 'treeitem')
    // A leaf chat has nothing to disclose, so the attribute is absent rather
    // than false — screen readers should not announce a toggle that isn't there.
    expect(row()).not.toHaveAttribute('aria-expanded')
  })

  it('reports its own aria-expanded when it has children', () => {
    const { rerender } = render(<AgentChatRow {...base} hasChildren expanded={false} />)
    expect(row()).toHaveAttribute('aria-expanded', 'false')
    rerender(<AgentChatRow {...base} hasChildren expanded />)
    expect(row()).toHaveAttribute('aria-expanded', 'true')
  })

  it('reflects the ⌘-click multiselection in aria-selected', () => {
    const { rerender } = render(<AgentChatRow {...base} selected={false} />)
    expect(row()).toHaveAttribute('aria-selected', 'false')
    rerender(<AgentChatRow {...base} selected />)
    expect(row()).toHaveAttribute('aria-selected', 'true')
  })
})

describe('AgentChatRow — drop attributes', () => {
  it('publishes id, parent, path, and the expanded/children presence flags', () => {
    render(<AgentChatRow {...base} hasChildren expanded />)
    expect(row()).toHaveAttribute('data-chat-drop', 'c1')
    expect(row()).toHaveAttribute('data-chat-parent', 'p1')
    expect(row()).toHaveAttribute('data-chat-path', '/p1/')
    // Presence flags, not "true"/"false" strings.
    expect(row()).toHaveAttribute('data-chat-expanded', '')
    expect(row()).toHaveAttribute('data-chat-children', '')
  })

  it('omits the expanded/children flags when neither is true', () => {
    render(<AgentChatRow {...base} hasChildren={false} expanded={false} />)
    expect(row()).not.toHaveAttribute('data-chat-expanded')
    expect(row()).not.toHaveAttribute('data-chat-children')
  })

  it('publishes the same attributes whatever the row is drawn UNDER', () => {
    // A kept row — one hoisted out of a folded ancestor because it is on screen —
    // is drawn one step under whichever ancestor is holding it, and the panel
    // hands it its REAL container and chain. There is nothing here to switch on:
    // the row publishes what it is given, and the tree is what knows the
    // difference (chat-rows.ts).
    const onPointerDownDrag = vi.fn()
    render(
      <AgentChatRow
        {...base}
        parentId="deep"
        path="/holder/deep/"
        onPointerDownDrag={onPointerDownDrag}
      />,
    )
    expect(row()).toHaveAttribute('data-chat-drop', 'c1')
    expect(row()).toHaveAttribute('data-chat-parent', 'deep')
    expect(row()).toHaveAttribute('data-chat-path', '/holder/deep/')

    fireEvent.pointerDown(row(), { button: 0 })
    expect(onPointerDownDrag).toHaveBeenCalledWith(
      { kind: 'chat', id: 'c1', parentId: 'deep' },
      expect.anything(),
    )
  })
})

describe('AgentChatRow — indent', () => {
  it('sits flush at depth 0', () => {
    render(<AgentChatRow {...base} />)
    expect(indentBox().style.marginInlineStart).toBe('0px')
  })

  it('steps one ROW_INDENT_STEP per level', () => {
    render(<AgentChatRow {...base} depth={3} />)
    // Same step the workspace tree uses, so the two panels' left edges line up.
    expect(indentBox().style.marginInlineStart).toBe(`${3 * ROW_INDENT_STEP}px`)
  })

  it('indenting does not change the row height', () => {
    // The virtualizer positions rows at index * AGENT_CHAT_ROW_HEIGHT; a wrapper
    // with vertical box would desynchronise the drop geometry from the paint.
    const { rerender } = render(<AgentChatRow {...base} depth={0} />)
    const flat = row().getBoundingClientRect().height
    rerender(<AgentChatRow {...base} depth={5} />)
    expect(row().getBoundingClientRect().height).toBe(flat)
  })
})

describe('AgentChatRow — visual state tokens', () => {
  it('paints ROW_NEST_TARGET when a drop would land inside it', () => {
    render(<AgentChatRow {...base} nesting />)
    for (const token of ROW_NEST_TARGET.split(' ')) expect(row().className).toContain(token)
  })

  it('nesting outranks the active treatment', () => {
    // Two signals never coexist: the nest fill and the raised active surface
    // both claim the row's own chrome, and only one move is happening.
    render(<AgentChatRow {...base} nesting active />)
    for (const token of ROW_NEST_TARGET.split(' ')) expect(row().className).toContain(token)
    expect(row().className).not.toContain('bg-background')
  })

  it('paints ROW_ACTIVE when active and not nesting', () => {
    render(<AgentChatRow {...base} active />)
    for (const token of ROW_ACTIVE.split(' ')) expect(row().className).toContain(token)
  })

  it('falls back to the inactive hover treatment otherwise', () => {
    render(<AgentChatRow {...base} />)
    expect(row().className).toContain('hover:bg-accent')
  })

  it('fades the source row while dragging', () => {
    render(<AgentChatRow {...base} dragging />)
    expect(row().className).toContain('opacity-40')
  })

  it('does not fade an ordinary row', () => {
    render(<AgentChatRow {...base} />)
    expect(row().className).not.toContain('opacity-40')
  })

  it('dims a row kept only as search context', () => {
    render(<AgentChatRow {...base} ctx />)
    expect(row().className).toContain('opacity-45')
  })

  it('does not dim an ordinary row', () => {
    render(<AgentChatRow {...base} />)
    expect(row().className).not.toContain('opacity-45')
  })
})

describe('AgentChatRow — selection and drag', () => {
  it('a plain click selects with meta=false', () => {
    const onSelect = vi.fn()
    render(<AgentChatRow {...base} onSelect={onSelect} />)
    fireEvent.click(row())
    expect(onSelect).toHaveBeenCalledWith('c1', false)
  })

  it('⌘-click is a selection gesture: onSelect(id, true)', () => {
    const onSelect = vi.fn()
    render(<AgentChatRow {...base} onSelect={onSelect} />)
    fireEvent.click(row(), { metaKey: true })
    expect(onSelect).toHaveBeenCalledWith('c1', true)
  })

  it('Ctrl-click is the same selection gesture on platforms without ⌘', () => {
    const onSelect = vi.fn()
    render(<AgentChatRow {...base} onSelect={onSelect} />)
    fireEvent.click(row(), { ctrlKey: true })
    expect(onSelect).toHaveBeenCalledWith('c1', true)
  })

  it('double-click starts rename', () => {
    const onStartRename = vi.fn()
    render(<AgentChatRow {...base} onStartRename={onStartRename} />)
    fireEvent.doubleClick(row())
    expect(onStartRename).toHaveBeenCalledWith('c1')
  })

  // The row hands over the SUBJECT, not its id: the drag needs the container to
  // plan a drop, and a panel-side closure that built it per row was a new
  // function identity every render, which defeated this component's memo.
  it('pointer-down forwards this row’s own drag subject', () => {
    const onPointerDownDrag = vi.fn()
    render(<AgentChatRow {...base} parentId="p9" onPointerDownDrag={onPointerDownDrag} />)
    fireEvent.pointerDown(row())
    expect(onPointerDownDrag).toHaveBeenCalledTimes(1)
    expect(onPointerDownDrag.mock.calls[0][0]).toEqual({ kind: 'chat', id: 'c1', parentId: 'p9' })
  })
})

describe('AgentChatRow — keyboard', () => {
  it('Enter on the focused row selects', () => {
    const onSelect = vi.fn()
    render(<AgentChatRow {...base} onSelect={onSelect} />)
    fireEvent.keyDown(row(), { key: 'Enter' })
    expect(onSelect).toHaveBeenCalledWith('c1', false)
  })

  it('Space on the focused row selects', () => {
    const onSelect = vi.fn()
    render(<AgentChatRow {...base} onSelect={onSelect} />)
    fireEvent.keyDown(row(), { key: ' ' })
    expect(onSelect).toHaveBeenCalledWith('c1', false)
  })

  it('ignores other keys', () => {
    const onSelect = vi.fn()
    render(<AgentChatRow {...base} onSelect={onSelect} />)
    fireEvent.keyDown(row(), { key: 'Tab' })
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('a keydown bubbling from a child (the disclosure chevron) does not select', () => {
    // The row's own onKeyDown only fires Enter/Space it received directly — a
    // key that started on a nested control (which has its own handling) must
    // not ALSO be read as "select this row".
    const onSelect = vi.fn()
    render(<AgentChatRow {...base} hasChildren onSelect={onSelect} />)
    const chevron = within(row()).getByLabelText('Expand')
    fireEvent.keyDown(chevron, { key: 'Enter', bubbles: true })
    expect(onSelect).not.toHaveBeenCalled()
  })
})

describe('AgentChatRow — while renaming', () => {
  it('renders the inline input seeded with the title', () => {
    render(<AgentChatRow {...base} renaming />)
    expect(screen.getByDisplayValue('My chat')).toBeTruthy()
    expect(screen.queryByText('My chat')).toBeNull()
  })

  it('renames in the row’s own face, not monospace — a chat title is prose', () => {
    render(<AgentChatRow {...base} renaming />)
    expect(screen.getByDisplayValue('My chat').className).not.toContain('font-mono')
  })

  it('confirming calls onConfirmRename with the chat id and the new title', () => {
    const onConfirmRename = vi.fn()
    render(<AgentChatRow {...base} renaming onConfirmRename={onConfirmRename} />)
    const input = screen.getByDisplayValue('My chat')
    fireEvent.change(input, { target: { value: 'Renamed chat' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onConfirmRename).toHaveBeenCalledWith('c1', 'Renamed chat')
  })

  it('cancelling (Escape) calls onCancelRename', () => {
    const onCancelRename = vi.fn()
    render(<AgentChatRow {...base} renaming onCancelRename={onCancelRename} />)
    fireEvent.keyDown(screen.getByDisplayValue('My chat'), { key: 'Escape' })
    expect(onCancelRename).toHaveBeenCalledTimes(1)
  })

  it('does not select or re-trigger rename on click/double-click', () => {
    const onSelect = vi.fn()
    const onStartRename = vi.fn()
    render(<AgentChatRow {...base} renaming onSelect={onSelect} onStartRename={onStartRename} />)
    fireEvent.click(row())
    fireEvent.doubleClick(row())
    expect(onSelect).not.toHaveBeenCalled()
    expect(onStartRename).not.toHaveBeenCalled()
  })

  it('does not start a drag on pointer-down', () => {
    const onPointerDownDrag = vi.fn()
    render(<AgentChatRow {...base} renaming onPointerDownDrag={onPointerDownDrag} />)
    fireEvent.pointerDown(row())
    expect(onPointerDownDrag).not.toHaveBeenCalled()
  })

  it('Enter/Space do not select', () => {
    const onSelect = vi.fn()
    render(<AgentChatRow {...base} renaming onSelect={onSelect} />)
    fireEvent.keyDown(row(), { key: 'Enter' })
    fireEvent.keyDown(row(), { key: ' ' })
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('never marks a match inside the editor', () => {
    render(<AgentChatRow {...base} title="Drag rules" query="rul" renaming />)
    expect(row().querySelector('mark')).toBeNull()
    expect(screen.getByRole('textbox')).toHaveValue('Drag rules')
  })
})

describe('AgentChatRow — holding', () => {
  it('draws the three holding dots and a fold-away control', () => {
    render(<AgentChatRow {...base} holding />)
    expect(row().querySelector('[data-holding-rows]')).not.toBeNull()
    expect(within(row()).getByLabelText(/Fold away/)).toBeTruthy()
  })

  it('draws neither when not holding', () => {
    render(<AgentChatRow {...base} />)
    expect(row().querySelector('[data-holding-rows]')).toBeNull()
    expect(screen.queryByLabelText(/Fold away/)).toBeNull()
  })

  it('pressing fold-away calls onFoldAway with the row id, and neither selects nor drags', () => {
    const onFoldAway = vi.fn()
    const onSelect = vi.fn()
    const onPointerDownDrag = vi.fn()
    render(
      <AgentChatRow
        {...base}
        holding
        onFoldAway={onFoldAway}
        onSelect={onSelect}
        onPointerDownDrag={onPointerDownDrag}
      />,
    )
    fireEvent.click(within(row()).getByLabelText(/Fold away/))
    expect(onFoldAway).toHaveBeenCalledWith('c1')
    expect(onSelect).not.toHaveBeenCalled()
    expect(onPointerDownDrag).not.toHaveBeenCalled()
  })
})

describe('AgentChatRow — the trailing "+", which makes a thread', () => {
  const plus = () => within(row()).getByLabelText('New thread in My chat')

  it('offers it on every row that is drawn where it lives', () => {
    render(<AgentChatRow {...base} />)
    expect(plus()).toBeTruthy()
  })

  it('wears the reply-in-thread elbow, never a "+"', () => {
    // "+" means "add another one of these", which is what a new chat in a FOLDER
    // is. A thread is not another chat alongside this one — it hangs off it and
    // reads it — so the two controls must not share a mark. (Nor a git-branch
    // glyph: a thread copies nothing.)
    render(<AgentChatRow {...base} />)
    expect(plus().querySelector('[data-thread-glyph]')).not.toBeNull()
    expect(plus().querySelector(`path[d="${ADD_GLYPH_PATH}"]`)).toBeNull()
  })

  it('starts a chat UNDER this one — the discoverable path to a thread', () => {
    const onNewThread = vi.fn()
    const onSelect = vi.fn()
    const onPointerDownDrag = vi.fn()
    render(
      <AgentChatRow
        {...base}
        onNewThread={onNewThread}
        onSelect={onSelect}
        onPointerDownDrag={onPointerDownDrag}
      />,
    )

    fireEvent.click(plus())

    // The chat id is the PARENT: a chat's child chat reads its turns, which is
    // what makes it a thread. Same signature and same slot as the folder row's
    // "+", because it is the same gesture.
    expect(onNewThread).toHaveBeenCalledWith('c1')
    // …and pressing it neither opens the row nor grabs it.
    expect(onSelect).not.toHaveBeenCalled()
    expect(onPointerDownDrag).not.toHaveBeenCalled()
  })

  it('a pointerdown on it never arms the row’s drag', () => {
    const onPointerDownDrag = vi.fn()
    render(<AgentChatRow {...base} onPointerDownDrag={onPointerDownDrag} />)
    fireEvent.pointerDown(plus())
    expect(onPointerDownDrag).not.toHaveBeenCalled()
  })
})

describe('AgentChatRow — disclosure', () => {
  it('renders the chevron only when it has children', () => {
    render(<AgentChatRow {...base} hasChildren={false} />)
    expect(screen.queryByLabelText(/Expand|Collapse/)).toBeNull()
  })

  it('pressing the chevron toggles without opening the chat or starting a drag', () => {
    const onToggle = vi.fn()
    const onSelect = vi.fn()
    const onPointerDownDrag = vi.fn()
    render(
      <AgentChatRow
        {...base}
        hasChildren
        onToggle={onToggle}
        onSelect={onSelect}
        onPointerDownDrag={onPointerDownDrag}
      />,
    )
    fireEvent.click(within(row()).getByLabelText('Expand'))
    expect(onToggle).toHaveBeenCalledWith('c1')
    expect(onSelect).not.toHaveBeenCalled()
    expect(onPointerDownDrag).not.toHaveBeenCalled()
  })
})

describe('AgentChatRow — search highlight', () => {
  it('marks the matched substring', () => {
    render(<AgentChatRow {...base} title="Drag rules" query="rul" />)
    expect(row().querySelector('mark')!.textContent).toBe('rul')
  })

  it('keeps the title’s own casing under the mark', () => {
    render(<AgentChatRow {...base} title="Drag rules" query="DRAG" />)
    expect(row().querySelector('mark')!.textContent).toBe('Drag')
  })

  it('renders the whole title when the query does not hit it', () => {
    render(<AgentChatRow {...base} title="Drag rules" query="zzz" />)
    expect(row().querySelector('mark')).toBeNull()
    expect(row().textContent).toContain('Drag rules')
  })

  it('renders no mark with no query', () => {
    render(<AgentChatRow {...base} title="Drag rules" />)
    expect(row().querySelector('mark')).toBeNull()
  })
})
