import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { installGlobalReporting, report } from '@/lib/errors/report'
import { UserFacingError } from '@/lib/errors/user-facing-error'
import { ApiError } from '@/lib/errors/api-error'
import { OperationFailedError } from '@/lib/errors/operation-failed'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'
import { toastManager } from '@/lib/toast-manager'

/** What report() shows for a client bug. */
const UNEXPLAINED = 'Something went wrong — details are in the log'

vi.mock('@/lib/toast-manager', () => ({ toastManager: { add: vi.fn() } }))

// Most cases reach report() as an unhandled rejection, the route
// installGlobalReporting gives it.
const target = new EventTarget() as Window
let dispose: () => void

function reject(reason: unknown): void {
  target.dispatchEvent(Object.assign(new Event('unhandledrejection'), { reason }))
}

beforeEach(() => {
  vi.clearAllMocks()
  useDaemonHaltStore.getState().clear()
  dispose = installGlobalReporting(target)
})

afterEach(() => dispose())

describe('report', () => {
  it('toasts the action as the title and the problem message as the description', () => {
    reject(new ApiError('held by another chat', 409, 'chat_busy'))

    expect(toastManager.add).toHaveBeenCalledWith({
      id: 'Something went wrong',
      type: 'error',
      title: 'Something went wrong',
      description: 'held by another chat',
    })
  })

  it('ignores an AbortError and a Monaco Canceled', () => {
    const canceled = new Error('Canceled')
    canceled.name = 'Canceled'

    reject(new DOMException('aborted', 'AbortError'))
    reject(canceled)

    expect(toastManager.add).not.toHaveBeenCalled()
  })

  it('ignores an OperationFailedError, which the operations watcher already showed', () => {
    reject(new OperationFailedError({ code: 'conflict', message: 'held' }, 'op1'))

    expect(toastManager.add).not.toHaveBeenCalled()
  })

  it('a daemon_halted problem halts instead of toasting', () => {
    reject(new ApiError('layout 9 is newer than this Crowbar knows', 503, 'daemon_halted'))

    expect(useDaemonHaltStore.getState().message).toBe('layout 9 is newer than this Crowbar knows')
    expect(toastManager.add).not.toHaveBeenCalled()
  })

  it('an unexpected error shows a generic line and keeps the real one in the console', () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {})
    const bug = new TypeError("cannot read properties of undefined (reading 'id')")

    reject(bug)

    expect(toastManager.add).toHaveBeenCalledWith(
      expect.objectContaining({ description: UNEXPLAINED }),
    )
    expect(logged).toHaveBeenCalledTimes(1)
    expect(logged).toHaveBeenCalledWith(expect.any(String), bug)
    logged.mockRestore()
  })

  it('keeps a client Error’s own text out of an action’s toast, and logs it once', () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {})
    const bug = new Error('no active workspace for paste')

    report(bug, { action: 'Paste failed' })

    expect(toastManager.add).toHaveBeenCalledWith({
      id: 'Paste failed',
      type: 'error',
      title: 'Paste failed',
      description: UNEXPLAINED,
    })
    expect(logged).toHaveBeenCalledTimes(1)
    expect(logged).toHaveBeenCalledWith(expect.any(String), bug)
    logged.mockRestore()
  })

  it('shows a sentence written for the user as it is, logging nothing', () => {
    const logged = vi.spyOn(console, 'error').mockImplementation(() => {})

    report(new UserFacingError('The saved prompts are full.'), { action: 'Save prompt' })

    expect(toastManager.add).toHaveBeenCalledWith(
      expect.objectContaining({ description: 'The saved prompts are full.' }),
    )
    expect(logged).not.toHaveBeenCalled()
    logged.mockRestore()
  })

  it('words a throw that is not an Error with the caller’s fallback, never its String()', () => {
    report({ status: 'gone' }, { action: 'Rename', fallback: 'request failed' })
    report({ status: 'gone' }, { action: 'Move' })
    report(new ApiError('disk full', 507), { action: 'Save', fallback: 'request failed' })

    const descriptions = vi.mocked(toastManager.add).mock.calls.map(([t]) => t.description)
    expect(descriptions).toEqual(['request failed', undefined, 'disk full'])
  })

  it('reports an unhandled rejection once installed, and nothing after dispose', () => {
    reject(new ApiError('disk gone', 500))
    expect(toastManager.add).toHaveBeenCalledWith(
      expect.objectContaining({ description: 'disk gone' }),
    )

    dispose()
    reject(new ApiError('again', 500))
    expect(toastManager.add).toHaveBeenCalledTimes(1)
  })
})
