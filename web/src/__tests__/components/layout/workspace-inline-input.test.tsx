import { render, screen, fireEvent } from '@testing-library/react'
import { vi, test, expect, describe, it } from 'vitest'
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
  render(
    <WorkspaceInlineInput defaultValue="feat/existing" onConfirm={vi.fn()} onCancel={vi.fn()} />,
  )
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

test('confirms trimmed value on blur', () => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(<WorkspaceInlineInput onConfirm={onConfirm} onCancel={onCancel} />)
  const input = screen.getByRole('textbox')
  fireEvent.change(input, { target: { value: '  feature/blur  ' } })
  fireEvent.blur(input)
  expect(onConfirm).toHaveBeenCalledWith('feature/blur')
  expect(onCancel).not.toHaveBeenCalled()
})

test('cancels on blur with blank value', () => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(<WorkspaceInlineInput onConfirm={onConfirm} onCancel={onCancel} />)
  fireEvent.blur(screen.getByRole('textbox'))
  expect(onCancel).toHaveBeenCalled()
  expect(onConfirm).not.toHaveBeenCalled()
})

test('Escape cancels even when input has a value', () => {
  const onConfirm = vi.fn()
  const onCancel = vi.fn()
  render(<WorkspaceInlineInput onConfirm={onConfirm} onCancel={onCancel} />)
  const input = screen.getByRole('textbox')
  fireEvent.change(input, { target: { value: 'feature/typed' } })
  fireEvent.keyDown(input, { key: 'Escape' })
  expect(onCancel).toHaveBeenCalled()
  expect(onConfirm).not.toHaveBeenCalled()
})

describe('WorkspaceInlineInput collision handling', () => {
  function setup() {
    const onConfirm = vi.fn()
    const onCancel = vi.fn()
    const onOpenExisting = vi.fn()
    const resolveExisting = (b: string) => (b.trim() === 'develop' ? 'ws-default' : null)
    render(
      <WorkspaceInlineInput
        onConfirm={onConfirm}
        onCancel={onCancel}
        resolveExisting={resolveExisting}
        onOpenExisting={onOpenExisting}
      />,
    )
    return { onConfirm, onCancel, onOpenExisting }
  }

  it('shows the hint and suppresses confirm for an existing branch', () => {
    const { onConfirm } = setup()
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: 'develop' } })
    expect(screen.getByText(/already has a workspace/i)).toBeInTheDocument()
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onConfirm).not.toHaveBeenCalled()
  })

  it('clicking the hint opens the existing workspace', () => {
    const { onOpenExisting } = setup()
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: 'develop' } })
    fireEvent.mouseDown(screen.getByText(/already has a workspace/i))
    expect(onOpenExisting).toHaveBeenCalledWith('ws-default')
  })

  it('confirms normally for a free branch', () => {
    const { onConfirm } = setup()
    const input = screen.getByRole('textbox')
    fireEvent.change(input, { target: { value: 'feature/new' } })
    fireEvent.keyDown(input, { key: 'Enter' })
    expect(onConfirm).toHaveBeenCalledWith('feature/new')
  })
})
