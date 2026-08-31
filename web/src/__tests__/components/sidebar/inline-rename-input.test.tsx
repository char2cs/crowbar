import { describe, expect, it, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'

describe('InlineRenameInput', () => {
  it('renders a real, focused, pre-selected input seeded with defaultValue', () => {
    render(<InlineRenameInput defaultValue="feature-x" onConfirm={vi.fn()} onCancel={vi.fn()} />)
    const input = screen.getByRole('textbox') as HTMLInputElement
    expect(input).toHaveValue('feature-x')
    expect(input).toHaveFocus()
    expect(input.selectionStart).toBe(0)
    expect(input.selectionEnd).toBe('feature-x'.length)
  })

  it('Enter confirms with the trimmed value', () => {
    const onConfirm = vi.fn()
    render(<InlineRenameInput defaultValue="old" onConfirm={onConfirm} onCancel={vi.fn()} />)
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: '  new-name  ' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onConfirm).toHaveBeenCalledWith('new-name')
  })

  it('Enter with an empty/whitespace-only value cancels instead of confirming', () => {
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<InlineRenameInput defaultValue="old" onConfirm={onConfirm} onCancel={onCancel} />)
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: '   ' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onConfirm).not.toHaveBeenCalled()
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('Escape cancels with no confirm call', () => {
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<InlineRenameInput defaultValue="old" onConfirm={onConfirm} onCancel={onCancel} />)
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: 'new-name' } })
    fireEvent.keyDown(input, { key: 'Escape' })
    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('blur confirms with the current value when Enter/Escape never fired', () => {
    const onConfirm = vi.fn()
    render(<InlineRenameInput defaultValue="old" onConfirm={onConfirm} onCancel={vi.fn()} />)
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: 'blurred-name' } })
    fireEvent.blur(input)
    expect(onConfirm).toHaveBeenCalledWith('blurred-name')
  })

  it('blur after Escape does not also fire confirm or a second cancel', () => {
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<InlineRenameInput defaultValue="old" onConfirm={onConfirm} onCancel={onCancel} />)
    const input = screen.getByRole('textbox')
    fireEvent.keyDown(input, { key: 'Escape' })
    fireEvent.blur(input)
    expect(onCancel).toHaveBeenCalledTimes(1)
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('blur after Enter does not also fire a second confirm', () => {
    const onConfirm = vi.fn()
    render(<InlineRenameInput defaultValue="old" onConfirm={onConfirm} onCancel={vi.fn()} />)
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: 'new-name' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    fireEvent.blur(input)
    expect(onConfirm).toHaveBeenCalledTimes(1)
  })

  it('blur with an empty value cancels instead of confirming', () => {
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    render(<InlineRenameInput defaultValue="old" onConfirm={onConfirm} onCancel={onCancel} />)
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: '' } })
    fireEvent.blur(input)
    expect(onConfirm).not.toHaveBeenCalled()
    expect(onCancel).toHaveBeenCalledTimes(1)
  })

  it('carries font-mono only when mono is set', () => {
    const { rerender } = render(
      <InlineRenameInput defaultValue="a" onConfirm={vi.fn()} onCancel={vi.fn()} />,
    )
    expect(screen.getByRole('textbox')).not.toHaveClass('font-mono')
    rerender(<InlineRenameInput defaultValue="a" mono onConfirm={vi.fn()} onCancel={vi.fn()} />)
    expect(screen.getByRole('textbox')).toHaveClass('font-mono')
  })
})
