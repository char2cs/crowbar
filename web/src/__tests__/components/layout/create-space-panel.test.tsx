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
import { OperationsWatcher } from '@/components/layout/operations-watcher'
import { useProjectDataStore } from '@/lib/store/projects'
import { streamOperation, resetOperations } from '@/__tests__/__fixtures__/operations'
import { operationsState, type Operation } from '@/lib/store/operations'
import { success } from '@/lib/loadable'
import type { Project } from '@/lib/types'
import {
  expectMigratingShown,
  expectReported,
  migratingRefusal,
  refusal,
  watchToasts,
} from '@/__tests__/__fixtures__/reports'

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
  resetOperations()
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
  await waitFor(() => expect(operationsState().ops[opId]?.version).toBe(1))

  streamOperation(createOp(opId, 2, 'succeeded'))
  const space: Project = {
    id: 'space-id',
    name: 'space',
    path: '/tmp/space',
    lastActivity: new Date(0),
  }
  useProjectDataStore.setState({ data: success([space]) })

  await waitFor(() => expect(onCreate).toHaveBeenCalledWith(space))
})

async function refuseCreate(error: unknown) {
  vi.mocked(api.postProject).mockRejectedValueOnce(error)
  render(
    <>
      <OperationsWatcher />
      <CreateSpacePanel onCreate={() => {}} onCancel={() => {}} />
    </>,
  )
  fireEvent.click(screen.getByRole('button', { name: /folder/i }))
  const create = () => screen.getByRole('button', { name: 'Create Space' })
  await waitFor(() => expect(create()).not.toBeDisabled())
  fireEvent.click(create())
  await waitFor(() => expect(vi.mocked(api.postProject)).toHaveBeenCalledTimes(1))
}

test('a refused create reports the daemon’s reason', async () => {
  const toasts = watchToasts()
  await refuseCreate(refusal('a space already uses that folder'))
  await waitFor(() =>
    expectReported(toasts, 'Add space failed', 'a space already uses that folder'),
  )
})

test('a create refused while the daemon migrates shows the migrating state, not a toast', async () => {
  const toasts = watchToasts()
  await refuseCreate(migratingRefusal())
  await waitFor(() => expectMigratingShown(toasts))
})
