import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { ImportProjectModal } from '@/components/projects/ImportProjectModal'
import { vi, expect, test } from 'vitest'

test('Import button is disabled when no folder is selected', () => {
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={() => {}} />)
  expect(screen.getByRole('button', { name: /import/i })).toBeDisabled()
})

test('shows selected folder name after pick', () => {
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={() => {}} />)
  // Simulate file selection via the hidden input
  const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement
  const file = new File([''], 'my-project', { type: '' })
  Object.defineProperty(file, 'webkitRelativePath', { value: 'my-project/', configurable: true })
  Object.defineProperty(fileInput, 'files', { value: [file], configurable: true })
  fireEvent.change(fileInput)
  // Get the readonly input (Project folder) which shows the selected path
  const readOnlyInputs = screen.getAllByDisplayValue('my-project')
  expect(readOnlyInputs.some(input => (input as HTMLInputElement).readOnly)).toBe(true)
})

test('calls onImport with name and path on submit', async () => {
  const onImport = vi.fn()
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={onImport} />)
  const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement
  const file = new File([''], 'test-proj', { type: '' })
  Object.defineProperty(file, 'webkitRelativePath', { value: 'test-proj/', configurable: true })
  Object.defineProperty(fileInput, 'files', { value: [file], configurable: true })
  fireEvent.change(fileInput)
  fireEvent.click(screen.getByRole('button', { name: /import/i }))
  await waitFor(() => {
    expect(onImport).toHaveBeenCalledWith(expect.objectContaining({ name: 'test-proj' }))
  })
})
