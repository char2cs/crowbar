import { act, render } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { OperationsWatcher } from '@/components/layout/operations-watcher'
import { toast } from '@/features/window/stores/toast-store'
import { streamOperation, resetOperations } from '@/__tests__/__fixtures__/operations'
import { type Operation } from '@/lib/store/operations'

vi.mock('@/features/window/stores/toast-store', () => ({ toast: { show: vi.fn() } }))

function op(over: Partial<Operation>): Operation {
  return {
    id: 'op1',
    kind: 'git.push',
    subject: { kind: 'workspace', id: 'ws1' },
    projectId: 'p1',
    label: 'Push main',
    state: 'running',
    items: [],
    dismissed: false,
    version: 1,
    createdAt: '2026-10-28T12:00:00Z',
    updatedAt: '2026-10-28T12:00:00Z',
    ...over,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
  resetOperations()
})

describe('OperationsWatcher', () => {
  it('toasts the label and reason when an operation it saw running fails', () => {
    render(<OperationsWatcher />)
    act(() => streamOperation(op({})))
    act(() =>
      streamOperation(
        op({ state: 'failed', version: 2, error: { code: 'internal', message: 'origin gone' } }),
      ),
    )

    expect(toast.show).toHaveBeenCalledTimes(1)
    expect(toast.show).toHaveBeenCalledWith({
      key: 'op1',
      type: 'error',
      message: 'Push main failed',
      description: 'origin gone',
    })
  })

  it('does not toast an operation that was already failed when first seen', () => {
    streamOperation(op({ state: 'failed', error: { code: 'internal', message: 'old' } }))
    render(<OperationsWatcher />)
    act(() =>
      streamOperation(
        op({
          id: 'op2',
          state: 'failed',
          snapshot: true,
          error: { code: 'internal', message: 'snapshot' },
        }),
      ),
    )

    expect(toast.show).not.toHaveBeenCalled()
  })

  // Work the daemon started on its own (a purge that gave up) is first seen failed.
  it('toasts an operation first seen failed on a live frame', () => {
    render(<OperationsWatcher />)
    act(() =>
      streamOperation(
        op({
          id: 'op3',
          label: 'Remove feature/a',
          state: 'failed',
          error: { code: 'internal', message: 'rm: busy' },
        }),
      ),
    )

    expect(toast.show).toHaveBeenCalledWith(
      expect.objectContaining({
        key: 'op3',
        message: 'Remove feature/a failed',
        description: 'rm: busy',
      }),
    )
  })
})
