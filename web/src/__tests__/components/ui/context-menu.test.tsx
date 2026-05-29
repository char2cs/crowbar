import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/react'
import { ContextMenu } from '@/components/ui/context-menu'

describe('ContextMenu keyboard dismiss', () => {
  it('calls onClose when Escape is pressed', () => {
    const onClose = vi.fn()
    render(
      <ContextMenu
        isOpen={true}
        position={{ x: 100, y: 100 }}
        items={[{ id: 'item-1', label: 'Item', onClick: vi.fn() }]}
        onClose={onClose}
      />
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
      />
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
      />
    )
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()
  })
})
