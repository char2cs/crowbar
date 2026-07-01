import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { vi, expect, test, beforeEach } from 'vitest'
import type { ProjectDTO } from '@/lib/types'

// §4/§6 subscribe-before-POST: postProject answers 202 (void) and the real
// ProjectDTO arrives on the `/v0/projects` WS stream. The modal subscribes
// first, fires the POST, then resolves the project by matching the path.
//
// We mock postProject (the 202 mutation) and the wsManager channel so the test
// can emit the canonical DTO frame the modal is waiting on.
const subscribers = new Set<(data: unknown) => void>()
function emit(data: unknown): void {
  subscribers.forEach((cb) => cb(data))
}

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (_endpoint: string, cb: (data: unknown) => void) => {
      subscribers.add(cb)
      return () => subscribers.delete(cb)
    },
    send: vi.fn(),
  },
}))

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    postProject: vi.fn((_name: string, _path: string) => Promise.resolve()),
  }
})

import * as api from '@/lib/api'
import { ImportProjectModal } from '@/components/projects/import-project-modal'

function projectDTO(over: Partial<ProjectDTO> & { id: string; path: string }): ProjectDTO {
  return {
    name: 'test-proj',
    status: '',
    lastActivity: new Date().toISOString(),
    ...over,
  }
}

const pathInput = () => screen.getByPlaceholderText('/absolute/path/to/project')

beforeEach(() => {
  subscribers.clear()
  vi.mocked(api.postProject).mockReset()
  vi.mocked(api.postProject).mockResolvedValue(undefined)
})

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

test('posts the path then resolves the real project from the /v0/projects WS push', async () => {
  const onImport = vi.fn()
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={onImport} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/test-proj' } })
  fireEvent.click(screen.getByRole('button', { name: /import/i }))

  // The POST fires with the folder-name fallback; no client-fabricated uuid.
  await waitFor(() => {
    const [name, path] = vi.mocked(api.postProject).mock.calls[0]
    expect(name).toBe('test-proj')
    expect(path).toBe('/tmp/test-proj')
  })

  // The canonical DTO (server-assigned id) arrives on the WS and resolves onImport.
  emit(projectDTO({ id: 'server-id', path: '/tmp/test-proj', name: 'test-proj' }))

  await waitFor(() => {
    expect(onImport).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'server-id', name: 'test-proj', path: '/tmp/test-proj' }),
    )
  })
})

test('uses the typed project name over the path fallback', async () => {
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={() => {}} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/test-proj' } })
  fireEvent.change(screen.getByPlaceholderText('My project'), { target: { value: 'Nice Name' } })
  fireEvent.click(screen.getByRole('button', { name: /import/i }))
  await waitFor(() => {
    const [name, path] = vi.mocked(api.postProject).mock.calls[0]
    expect(name).toBe('Nice Name')
    expect(path).toBe('/tmp/test-proj')
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
