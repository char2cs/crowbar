import { useState } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { Tree, TreeItem } from '@/components/ui/tree'

// src/ (open: src/app), docs, notes.txt (a leaf). `src` and `docs` can be expanded.
function visible(open: ReadonlySet<string>) {
  return [
    { key: 'src', level: 1, expanded: open.has('src') },
    ...(open.has('src') ? [{ key: 'src/app', level: 2, expanded: false }] : []),
    { key: 'docs', level: 1, expanded: open.has('docs') },
    { key: 'notes.txt', level: 1 },
  ]
}

function FolderTree({ onSelect }: { onSelect: (key: string) => void }) {
  const [activeKey, setActiveKey] = useState<string | null>(null)
  const [open, setOpen] = useState<ReadonlySet<string>>(() => new Set())
  const items = visible(open)
  return (
    <Tree
      aria-label="Folders"
      items={items}
      activeKey={activeKey}
      onActiveKeyChange={setActiveKey}
      onExpandedChange={(key, expanded) => {
        const next = new Set(open)
        if (expanded) next.add(key)
        else next.delete(key)
        setOpen(next)
      }}
      onSelect={onSelect}
      itemId={(key) => `row-${key}`}
    >
      {items.map((item) => (
        <TreeItem
          key={item.key}
          id={`row-${item.key}`}
          level={item.level}
          expanded={item.expanded}
          active={item.key === activeKey}
          inert={item.expanded === undefined}
          onClick={() => setActiveKey(item.key)}
        >
          {item.key}
        </TreeItem>
      ))}
    </Tree>
  )
}

const tree = () => screen.getByRole('tree', { name: 'Folders' })
const row = (name: string) => screen.getByRole('treeitem', { name })
const active = () => tree().getAttribute('aria-activedescendant')
const press = (key: string) => fireEvent.keyDown(tree(), { key })

let scrolled: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  scrolled = vi.spyOn(HTMLElement.prototype, 'scrollIntoView')
})

afterEach(() => vi.restoreAllMocks())

function renderTree() {
  const onSelect = vi.fn()
  render(<FolderTree onSelect={onSelect} />)
  fireEvent.focus(tree())
  return onSelect
}

describe('Tree', () => {
  it('is one focusable tree whose rows say their level, whether they open, and which is active', () => {
    renderTree()

    expect(tree()).toHaveAttribute('tabindex', '0')
    expect(active()).toBe('row-src')
    expect(row('src')).toHaveAttribute('aria-level', '1')
    expect(row('src')).toHaveAttribute('aria-expanded', 'false')
    expect(row('src')).toHaveAttribute('tabindex', '-1')
    expect(row('notes.txt')).not.toHaveAttribute('aria-expanded')
    expect(row('notes.txt')).toHaveAttribute('aria-disabled', 'true')
  })

  it('arrows move the active row, Home and End jump, and neither runs past an end', () => {
    renderTree()

    press('ArrowUp')
    expect(active()).toBe('row-src')
    press('ArrowDown')
    expect(active()).toBe('row-docs')
    press('End')
    expect(active()).toBe('row-notes.txt')
    press('ArrowDown')
    expect(active()).toBe('row-notes.txt')
    press('Home')
    expect(active()).toBe('row-src')
  })

  it('Right opens a folder then steps into it; Left steps out then closes it', () => {
    renderTree()

    press('ArrowRight')
    expect(row('src')).toHaveAttribute('aria-expanded', 'true')
    expect(active()).toBe('row-src')
    press('ArrowRight')
    expect(active()).toBe('row-src/app')
    expect(row('src/app')).toHaveAttribute('aria-level', '2')
    press('ArrowLeft')
    expect(active()).toBe('row-src')
    press('ArrowLeft')
    expect(screen.queryByRole('treeitem', { name: 'src/app' })).not.toBeInTheDocument()
  })

  it.each(['Enter', ' '])('%j chooses the active row', (key) => {
    const onSelect = renderTree()

    press('ArrowDown')
    press(key)

    expect(onSelect).toHaveBeenCalledExactlyOnceWith('docs')
  })

  // aria-activedescendant never scrolls: the active row is brought into view as it moves, only
  // as far as it takes.
  it('brings the row it moves to into view, and nothing when it does not move', () => {
    renderTree()

    press('End')
    expect(scrolled).toHaveBeenLastCalledWith({ block: 'nearest' })
    expect(scrolled.mock.contexts.at(-1)).toBe(row('notes.txt'))
    press('ArrowUp')
    expect(scrolled.mock.contexts.at(-1)).toBe(row('docs'))
    const before = scrolled.mock.calls.length
    press('Enter')
    press('a')
    expect(scrolled).toHaveBeenCalledTimes(before)
  })

  it('a click on a row leaves focus on the tree, and the arrows carry on from that row', async () => {
    const user = userEvent.setup()
    renderTree()

    await user.click(row('docs'))
    expect(tree()).toHaveFocus()
    await user.keyboard('{ArrowDown}')

    expect(tree()).toHaveFocus()
    expect(active()).toBe('row-notes.txt')
  })
})
