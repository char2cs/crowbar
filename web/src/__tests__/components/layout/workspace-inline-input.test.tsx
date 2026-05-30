import { render, screen, fireEvent } from '@testing-library/react'
import { vi, test, expect } from 'vitest'
import { WorkspaceInlineInput } from '@/components/layout/workspace-inline-input'

test('confirms trimmed value on Enter', () => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(<WorkspaceInlineInput onConfirm={onConfirm} onCancel={onCancel} />)
  fireEvent.change(screen.getByRole('textbox'), { target: { value: '  feature/foo  ' } })
  fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Enter' })
  expect(onConfirm).toHaveBeenCalledWith('feature/foo')
  expect(onCancel).not.toHaveBeenCalled()
})

test('calls onCancel on Escape', () => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(<WorkspaceInlineInput onConfirm={onConfirm} onCancel={onCancel} />)
  fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Escape' })
  expect(onCancel).toHaveBeenCalled()
  expect(onConfirm).not.toHaveBeenCalled()
})

test('calls onCancel when Enter pressed with blank value', () => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(<WorkspaceInlineInput onConfirm={onConfirm} onCancel={onCancel} />)
  fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Enter' })
  expect(onCancel).toHaveBeenCalled()
  expect(onConfirm).not.toHaveBeenCalled()
})

test('pre-fills input with defaultValue', () => {
  render(<WorkspaceInlineInput defaultValue="feat/existing" onConfirm={vi.fn()} onCancel={vi.fn()} />)
  expect(screen.getByRole('textbox')).toHaveValue('feat/existing')
})

test('does not double-fire after Enter then blur', () => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(<WorkspaceInlineInput onConfirm={onConfirm} onCancel={onCancel} />)
  const input = screen.getByRole('textbox')
  fireEvent.change(input, { target: { value: 'feature/test' } })
  fireEvent.keyDown(input, { key: 'Enter' })
  fireEvent.blur(input)
  expect(onConfirm).toHaveBeenCalledTimes(1)
})
