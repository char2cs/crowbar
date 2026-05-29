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
})
