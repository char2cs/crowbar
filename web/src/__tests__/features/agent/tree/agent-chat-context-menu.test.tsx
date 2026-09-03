/**
 * The Chats tree's right-click menu, driven without a panel around it.
 *
 * The panel's own tests cover the gesture as the user makes it. What is here are
 * the states a panel cannot easily produce: a tree that has not mounted yet, a
 * right-click that lands on something which is not a row, and a target that is
 * not an element at all — every one of them a place where the listener runs
 * before the thing it is about exists.
 */
import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createRef } from 'react'
import { AgentChatContextMenu } from '@/features/agent/tree/agent-chat-context-menu'
import { chatRowProps, type ChatDragSubject } from '@/features/agent/tree/lib/chat-drop'

/** A row that publishes exactly what the real rows publish. */
function mountRow(id: string, kind: 'chat' | 'chatFolder' = 'chat') {
  const el = document.createElement('div')
  el.setAttribute('role', 'treeitem')
  const props = chatRowProps({ kind, id, parentId: 'p1', path: '/p1/', expanded: true })
  for (const [attr, value] of Object.entries(props)) {
    if (value !== undefined) el.setAttribute(attr, value)
  }
  // The leading glyph. A chat row's is an SVG, and an SVGElement is not an
  // HTMLElement — the listener has to reach the row from one anyway.
  el.appendChild(document.createElementNS('http://www.w3.org/2000/svg', 'svg'))
  el.appendChild(document.createTextNode(id))
  return el
}

const menuItems = () =>
  Array.from(document.querySelectorAll('[role="menuitem"]')).map((el) => el.textContent)

function renderMenu(opts: { tree?: HTMLElement | null; selection?: ChatDragSubject[] } = {}) {
  const onNewThread = vi.fn()
  const onGroup = vi.fn()
  const onRemove = vi.fn()
  const treeRef = createRef<HTMLElement>() as { current: HTMLElement | null }
  treeRef.current = opts.tree === undefined ? document.body : opts.tree
  const view = render(
    <AgentChatContextMenu
      treeRef={treeRef}
      selectionSubjects={() => opts.selection ?? []}
      onNewThread={onNewThread}
      onGroup={onGroup}
      onRemove={onRemove}
    />,
  )
  return { ...view, onNewThread, onGroup, onRemove }
}

beforeEach(() => {
  document.body.innerHTML = ''
})

afterEach(() => {
  cleanup()
  vi.restoreAllMocks()
})

describe('AgentChatContextMenu', () => {
  it('opens on a row and acts on it', () => {
    const row = mountRow('c1')
    document.body.appendChild(row)
    const { onRemove } = renderMenu()

    fireEvent.contextMenu(row)

    expect(menuItems()).toEqual(['New thread', 'Group into a folder', 'Delete chat'])
    fireEvent.click(screen.getByText('Delete chat'))
    expect(onRemove).toHaveBeenCalledWith([{ kind: 'chat', id: 'c1', parentId: 'p1' }])
  })

  it('draws the SAME mark the row’s own control wears', () => {
    // Two paths to one action drawn with two different glyphs read as two
    // different actions. The row's control is data-thread-glyph; so is this.
    const row = mountRow('c1')
    document.body.appendChild(row)
    renderMenu()

    fireEvent.contextMenu(row)

    const entry = screen.getByText('New thread').closest('[role="menuitem"]')
    expect(entry?.querySelector('[data-thread-glyph]')).not.toBeNull()
  })

  it('threads a new chat off the row that was right-clicked', () => {
    const row = mountRow('c1')
    document.body.appendChild(row)
    const { onNewThread } = renderMenu()

    fireEvent.contextMenu(row)
    fireEvent.click(screen.getByText('New thread'))

    // The PARENT is what a thread is made from: the new chat hangs off c1 and
    // reads its turns.
    expect(onNewThread).toHaveBeenCalledWith('c1')
  })

  it('opens from the row’s SVG glyph, which is not an HTMLElement', () => {
    const row = mountRow('c1')
    document.body.appendChild(row)
    renderMenu()

    fireEvent.contextMenu(row.querySelector('svg')!)

    // Testing the target for `HTMLElement` meant right-clicking the provider
    // icon — a good half of the row's left edge — opened nothing at all.
    expect(menuItems()).toEqual(['New thread', 'Group into a folder', 'Delete chat'])
  })

  it('groups the whole selection when the clicked row is part of it', () => {
    const row = mountRow('c1')
    document.body.appendChild(row)
    const selection: ChatDragSubject[] = [
      { kind: 'chat', id: 'c1', parentId: 'p1' },
      { kind: 'chat', id: 'c2', parentId: 'p1' },
    ]
    const { onGroup } = renderMenu({ selection })

    fireEvent.contextMenu(row)
    fireEvent.click(screen.getByText('Group 2 into a folder'))

    // dragSubjectsFor is the one rule — the drag's, already tested — so a
    // right-click and a drag can never disagree about what they are holding.
    expect(onGroup).toHaveBeenCalledWith(selection)
  })

  it('ignores a right-click that is not on one of this tree’s rows', () => {
    const stray = document.createElement('div')
    stray.setAttribute('role', 'treeitem') // a row of the tree NEXT DOOR
    document.body.appendChild(stray)
    renderMenu()

    fireEvent.contextMenu(stray)

    expect(menuItems()).toEqual([])
  })

  it('ignores a target that is not an element at all', () => {
    const row = mountRow('c1')
    document.body.appendChild(row)
    renderMenu()

    // Text nodes are event targets too, and `closest` is not on one.
    const text = row.lastChild as Text
    act(() => {
      text.dispatchEvent(new MouseEvent('contextmenu', { bubbles: true }))
    })

    expect(menuItems()).toEqual([])
  })

  it('mounts inert when the tree is not on screen yet', () => {
    const row = mountRow('c1')
    document.body.appendChild(row)
    renderMenu({ tree: null })

    fireEvent.contextMenu(row)

    // The panel renders this beside a scroll container whose ref is null on the
    // first pass; a listener attached to nothing must simply not attach.
    expect(menuItems()).toEqual([])
  })

  it('closes without acting', () => {
    const row = mountRow('f1', 'chatFolder')
    document.body.appendChild(row)
    const { onGroup, onRemove } = renderMenu()

    fireEvent.contextMenu(row)
    expect(menuItems()).toEqual(['Group into a folder', 'Delete folder'])

    fireEvent.keyDown(document.activeElement ?? document.body, { key: 'Escape' })

    expect(onGroup).not.toHaveBeenCalled()
    expect(onRemove).not.toHaveBeenCalled()
  })
})
