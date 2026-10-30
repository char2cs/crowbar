import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { vi, expect, test, beforeEach } from 'vitest'

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, postProject: vi.fn() }
})
vi.mock('@/lib/native-dialog', () => ({
  openNativeDialog: vi.fn(async () => '/tmp/space'),
}))

import * as api from '@/lib/api'
import { CreateSpacePanel } from '@/components/layout/create-space-panel'
import { useProjectDataStore } from '@/lib/store/projects'
import { applyOperationFrame, useOperationsStore, type Operation } from '@/lib/store/operations'
import { success } from '@/lib/loadable'
import type { Project } from '@/lib/types'

function createOp(opId: string, version: number, state: Operation['state']): Operation {
  return {
    id: opId,
    kind: 'project.create',
    subject: { kind: 'project', id: 'space-id' },
    projectId: 'space-id',
    label: 'Add space',
    state,
    items: [{ ref: { kind: 'project', id: 'space-id' }, label: 'space', state }],
    dismissed: false,
    version,
    createdAt: '2026-10-30T10:00:00Z',
    updatedAt: '2026-10-30T10:00:00Z',
  }
}

beforeEach(() => {
  vi.mocked(api.postProject).mockReset()
  vi.mocked(api.postProject).mockImplementation(async (_n, _p, opId) => ({
    operation: createOp(opId, 1, 'accepted'),
  }))
  useOperationsStore.setState({ ops: {}, latestFor: {} })
  useProjectDataStore.setState({ data: success([]) })
})

test('import waits on the operation and hands over the project from the store', async () => {
  const onCreate = vi.fn()
  render(<CreateSpacePanel onCreate={onCreate} onCancel={() => {}} />)
  fireEvent.click(screen.getByRole('button', { name: /folder/i }))
  await waitFor(() =>
    expect(screen.getByRole('button', { name: 'Create Space' })).not.toBeDisabled(),
  )
  fireEvent.click(screen.getByRole('button', { name: 'Create Space' }))

  await waitFor(() => expect(vi.mocked(api.postProject)).toHaveBeenCalledTimes(1))
  const [name, path, opId] = vi.mocked(api.postProject).mock.calls[0]
  expect([name, path]).toEqual(['space', '/tmp/space'])
  await waitFor(() => expect(useOperationsStore.getState().ops[opId]?.version).toBe(1))

  applyOperationFrame(createOp(opId, 2, 'succeeded'))
  const space: Project = {
    id: 'space-id',
    name: 'space',
    path: '/tmp/space',
    lastActivity: new Date(0),
  }
  useProjectDataStore.setState({ data: success([space]) })

  await waitFor(() => expect(onCreate).toHaveBeenCalledWith(space))
})
