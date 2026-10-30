import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { vi, expect, test, beforeEach } from 'vitest'

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, postProject: vi.fn() }
})

import * as api from '@/lib/api'
import { ImportProjectModal } from '@/components/projects/import-project-modal'
import { useProjectDataStore } from '@/lib/store/projects'
import { applyOperationFrame, useOperationsStore, type Operation } from '@/lib/store/operations'
import { success } from '@/lib/loadable'
import type { Project } from '@/lib/types'

/** The daemon's row for the add: the project it names under `server-id`. */
function createOp(opId: string, version: number, state: Operation['state']): Operation {
  return {
    id: opId,
    kind: 'project.create',
    subject: { kind: 'project', id: 'server-id' },
    projectId: 'server-id',
    label: 'Add test-proj',
    state,
    items: [{ ref: { kind: 'project', id: 'server-id' }, label: 'test-proj', state }],
    dismissed: false,
    version,
    createdAt: '2026-10-30T10:00:00Z',
    updatedAt: '2026-10-30T10:00:00Z',
  }
}

const project: Project = {
  id: 'server-id',
  name: 'test-proj',
  path: '/tmp/test-proj',
  lastActivity: new Date(0),
}
const pathInput = () => screen.getByPlaceholderText('/absolute/path/to/project')
const importButton = () => screen.getByRole('button', { name: /import/i })

beforeEach(() => {
  vi.mocked(api.postProject).mockReset()
  vi.mocked(api.postProject).mockImplementation(async (_n, _p, opId) => ({
    operation: createOp(opId, 1, 'accepted'),
  }))
  useOperationsStore.setState({ ops: {}, latestFor: {} })
  useProjectDataStore.setState({ data: success([]) })
})

test('Import button is disabled when the path is empty', () => {
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={() => {}} />)
  expect(importButton()).toBeDisabled()
})

test('Import stays disabled for a relative path and shows a hint', () => {
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={() => {}} />)
  fireEvent.change(pathInput(), { target: { value: 'my-project' } })
  expect(importButton()).toBeDisabled()
  expect(screen.getByText(/absolute path/i)).toBeInTheDocument()
})

test('import waits on the operation and hands over the project from the store', async () => {
  const onImport = vi.fn()
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={onImport} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/test-proj' } })
  fireEvent.click(importButton())

  await waitFor(() => expect(vi.mocked(api.postProject)).toHaveBeenCalledTimes(1))
  const [name, path, opId] = vi.mocked(api.postProject).mock.calls[0]
  expect([name, path]).toEqual(['test-proj', '/tmp/test-proj'])
  await waitFor(() => expect(useOperationsStore.getState().ops[opId]?.version).toBe(1))

  applyOperationFrame(createOp(opId, 2, 'succeeded'))
  expect(onImport).not.toHaveBeenCalled()
  useProjectDataStore.setState({ data: success([project]) })

  await waitFor(() => expect(onImport).toHaveBeenCalledWith(project))
})

test('uses the typed project name over the path fallback', async () => {
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={() => {}} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/test-proj' } })
  fireEvent.change(screen.getByPlaceholderText('My project'), { target: { value: 'Nice Name' } })
  fireEvent.click(importButton())
  await waitFor(() =>
    expect(vi.mocked(api.postProject)).toHaveBeenCalledWith(
      'Nice Name',
      '/tmp/test-proj',
      expect.any(String),
    ),
  )
})

test('a failed import re-enables the Import button', async () => {
  const onImport = vi.fn()
  render(<ImportProjectModal open={true} onOpenChange={() => {}} onImport={onImport} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/test-proj' } })
  fireEvent.click(importButton())
  await waitFor(() => expect(vi.mocked(api.postProject)).toHaveBeenCalledTimes(1))
  const opId = vi.mocked(api.postProject).mock.calls[0][2]
  await waitFor(() => expect(useOperationsStore.getState().ops[opId]?.version).toBe(1))

  applyOperationFrame({
    ...createOp(opId, 2, 'failed'),
    error: { code: 'invalid_argument', message: 'disk full' },
  })

  await waitFor(() => expect(importButton()).not.toBeDisabled())
  expect(onImport).not.toHaveBeenCalled()
})
