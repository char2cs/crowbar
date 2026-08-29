import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { RenameDialog } from '@/components/sidebar/rename-dialog'

describe('RenameDialog', () => {
  it('confirms with the edited value', () => {
    const onConfirm = vi.fn()
    render(
      <RenameDialog open initialValue="feature-x" onOpenChange={vi.fn()} onConfirm={onConfirm} />,
    )
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: 'feature-y' } })
    fireEvent.click(screen.getByRole('button', { name: /rename/i }))
    expect(onConfirm).toHaveBeenCalledWith('feature-y')
  })

  it('seeds the input from initialValue', () => {
    render(
      <RenameDialog open initialValue="feature-x" onOpenChange={vi.fn()} onConfirm={vi.fn()} />,
    )
    expect(screen.getByRole('textbox')).toHaveValue('feature-x')
  })

  it('closes without confirming on Cancel', () => {
    const onConfirm = vi.fn()
    const onOpenChange = vi.fn()
    render(
      <RenameDialog
        open
        initialValue="feature-x"
        onOpenChange={onOpenChange}
        onConfirm={onConfirm}
      />,
    )
    fireEvent.click(screen.getByRole('button', { name: /cancel/i }))
    expect(onConfirm).not.toHaveBeenCalled()
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it('does not confirm an empty/whitespace-only name', () => {
    const onConfirm = vi.fn()
    render(
      <RenameDialog open initialValue="feature-x" onOpenChange={vi.fn()} onConfirm={onConfirm} />,
    )
    fireEvent.change(screen.getByRole('textbox'), { target: { value: '   ' } })
    fireEvent.click(screen.getByRole('button', { name: /rename/i }))
    expect(onConfirm).not.toHaveBeenCalled()
  })
})
