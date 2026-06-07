import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { vi, expect, test } from 'vitest'

// The real mutation returns only { id }; the modal then re-fetches the full
// project via fetchProject(id). Mocks mirror that contract.
vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    postProject: vi.fn((_name: string, _path: string) =>
      Promise.resolve({ id: 'mock-id' }),
    ),
    fetchProject: vi.fn((id: string) =>
      Promise.resolve({ id, name: 'test-proj', path: 'test-proj', lastActivity: new Date() }),
    ),
  }
})

import * as api from '@/lib/api'
import { ImportProjectModal } from '@/components/projects/ImportProjectModal'

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

test('resets loading and re-enables Import button when postProject rejects', async () => {
  vi.mocked(api.postProject).mockRejectedValueOnce(new Error('disk full'))

  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={() => {}} />)

  const fileInput = document.querySelector('input[type="file"]') as HTMLInputElement
  const file = new File([''], 'my-project', { type: '' })
  Object.defineProperty(file, 'webkitRelativePath', { value: 'my-project/', configurable: true })
  Object.defineProperty(fileInput, 'files', { value: [file], configurable: true })
  fireEvent.change(fileInput)

  fireEvent.click(screen.getByRole('button', { name: /import/i }))

  await waitFor(() => {
    expect(screen.getByRole('button', { name: /import/i })).not.toBeDisabled()
    expect(screen.queryByText('Importing…')).not.toBeInTheDocument()
  })
})
