import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { vi, expect, test } from 'vitest'

// The real mutation returns only { id }; the modal then re-fetches the full
// project via fetchProject(id). Mocks mirror that contract.
vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    postProject: vi.fn((_name: string, _path: string) => Promise.resolve({ id: 'mock-id' })),
    fetchProject: vi.fn((id: string) =>
      Promise.resolve({ id, name: 'test-proj', path: '/tmp/test-proj', lastActivity: new Date() }),
    ),
  }
})

import * as api from '@/lib/api'
import { ImportProjectModal } from '@/components/projects/ImportProjectModal'

const pathInput = () => screen.getByPlaceholderText('/absolute/path/to/project')

test('Import button is disabled when the path is empty', () => {
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={() => {}} />)
  expect(screen.getByRole('button', { name: /import/i })).toBeDisabled()
})

test('Import stays disabled for a relative path and shows a hint', () => {
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={() => {}} />)
  fireEvent.change(pathInput(), { target: { value: 'my-project' } })
  expect(screen.getByRole('button', { name: /import/i })).toBeDisabled()
  expect(screen.getByText(/absolute path/i)).toBeInTheDocument()
})

test('posts the absolute path with the folder name as fallback project name', async () => {
  const onImport = vi.fn()
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={onImport} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/test-proj' } })
  fireEvent.click(screen.getByRole('button', { name: /import/i }))
  await waitFor(() => {
    expect(api.postProject).toHaveBeenCalledWith('test-proj', '/tmp/test-proj')
    expect(onImport).toHaveBeenCalledWith(expect.objectContaining({ name: 'test-proj' }))
  })
})

test('uses the typed project name over the path fallback', async () => {
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={() => {}} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/test-proj' } })
  fireEvent.change(screen.getByPlaceholderText('My project'), { target: { value: 'Nice Name' } })
  fireEvent.click(screen.getByRole('button', { name: /import/i }))
  await waitFor(() => {
    expect(api.postProject).toHaveBeenCalledWith('Nice Name', '/tmp/test-proj')
  })
})

test('resets loading and re-enables Import button when postProject rejects', async () => {
  vi.mocked(api.postProject).mockRejectedValueOnce(new Error('disk full'))

  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={() => {}} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/my-project' } })
  fireEvent.click(screen.getByRole('button', { name: /import/i }))

  await waitFor(() => {
    expect(screen.getByRole('button', { name: /import/i })).not.toBeDisabled()
    expect(screen.queryByText('Importing…')).not.toBeInTheDocument()
  })
})
