import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/react'
import { useState } from 'react'
import { ContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'

describe('ContextMenu keyboard dismiss', () => {
  it('calls onClose when Escape is pressed', () => {
    const onClose = vi.fn()
    render(
      <ContextMenu
        isOpen={true}
        position={{ x: 100, y: 100 }}
        items={[{ id: 'item-1', label: 'Item', onClick: vi.fn() }]}
        onClose={onClose}
      />,
    )
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('does not call onClose for non-Escape keys', () => {
    const onClose = vi.fn()
    render(
      <ContextMenu
        isOpen={true}
        position={{ x: 100, y: 100 }}
        items={[{ id: 'item-1', label: 'Item', onClick: vi.fn() }]}
        onClose={onClose}
      />,
    )
    fireEvent.keyDown(document, { key: 'Enter' })
    expect(onClose).not.toHaveBeenCalled()
  })

  it('does not call onClose when menu is closed', () => {
    const onClose = vi.fn()
    render(
      <ContextMenu
        isOpen={false}
        position={{ x: 100, y: 100 }}
        items={[{ id: 'item-1', label: 'Item', onClick: vi.fn() }]}
        onClose={onClose}
      />,
    )
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()
  })
})

// Closes for real on onClose, as every caller's useContextMenu does.
function OwnedMenu({ items }: { items: ContextMenuItem[] }) {
  const [isOpen, setOpen] = useState(true)
  return (
    <ContextMenu
      isOpen={isOpen}
      position={{ x: 0, y: 0 }}
      items={items}
      onClose={() => setOpen(false)}
    />
  )
}

describe('ContextMenu submenus', () => {
  it('opens a submenu on hover and fires the nested item onClick', async () => {
    const onNested = vi.fn()
    const { findByText, getByRole } = render(
      <OwnedMenu
        items={[
          {
            id: 'turn-into',
            label: 'Turn into',
            onClick: () => {},
            items: [{ id: 'h1', label: 'Heading 1', onClick: onNested }],
          },
        ]}
      />,
    )

    // Base UI only opens a submenu on hover after the pointer has actually
    // moved inside the menu (guards against an accidental trigger sitting
    // under the cursor when the menu first mounts) — so move over the popup
    // before hovering the trigger, matching a real mouse gesture.
    const menu = getByRole('menu')
    fireEvent.mouseMove(menu)
    const trigger = (await findByText('Turn into')).closest('[role="menuitem"]')!
    fireEvent.pointerEnter(trigger)
    fireEvent.mouseEnter(trigger)
    const nestedItem = await findByText('Heading 1')
    fireEvent.click(nestedItem)

    expect(onNested).toHaveBeenCalledOnce()
  })
})
