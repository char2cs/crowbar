import { afterEach, describe, expect, it, vi } from 'vitest'
import { forwardToastsToShell } from '@/entries/forward-toasts-to-shell'
import { toastManager } from '@/lib/toast-manager'

let stop = () => {}

const PROMISED = '00000000-0000-4000-8000-000000000001'

afterEach(() => {
  stop()
  vi.restoreAllMocks()
})

const cases = [
  {
    name: 'sends a titled toast with its detail, and a pinned one for a toast that never times out',
    raise: () => {
      toastManager.add({ id: 'a', type: 'error', title: 'Save the file', description: 'disk full' })
      toastManager.add({ id: 'b', type: 'loading', title: 'Taking the prompt back…', timeout: 0 })
      toastManager.add({ id: 'c', description: 'Only a detail' })
    },
    shown: [
      { key: 'a', tone: 'error', title: 'Save the file', description: 'disk full', pinned: false },
      { key: 'b', tone: 'loading', title: 'Taking the prompt back…', pinned: true },
      { key: 'c', tone: 'info', title: 'Only a detail', pinned: false },
    ],
    closed: [],
  },
  {
    name: 'rewrites an updated toast under its key and takes down closed ones',
    raise: () => {
      toastManager.add({ id: 'a', type: 'loading', title: 'Saving', timeout: 0 })
      toastManager.add({ id: 'b', type: 'info', title: 'Other' })
      toastManager.update('a', { type: 'success', title: 'Saved', timeout: 5000 })
      toastManager.close('a')
      toastManager.close()
    },
    shown: [
      { key: 'a', tone: 'loading', title: 'Saving', pinned: true },
      { key: 'b', tone: 'info', title: 'Other', pinned: false },
      { key: 'a', tone: 'success', title: 'Saved', pinned: false },
    ],
    closed: [['a'], ['b']],
  },
  {
    name: 'shows a promise as loading, then as its outcome under the same key',
    raise: () => {
      vi.spyOn(crypto, 'randomUUID').mockReturnValueOnce(PROMISED)
      return toastManager.promise(Promise.resolve(3), {
        loading: 'Counting',
        success: (n: number) => `Counted ${n}`,
        error: 'Could not count',
      })
    },
    shown: [
      { key: PROMISED, tone: 'loading', title: 'Counting', pinned: true },
      { key: PROMISED, tone: 'success', title: 'Counted 3', pinned: false },
    ],
    closed: [],
  },
]

describe('forwardToastsToShell', () => {
  it.each(cases)('$name', async ({ raise, shown, closed }) => {
    const host = { showToast: vi.fn(), closeToast: vi.fn() }
    stop = forwardToastsToShell(host)

    await raise()

    await vi.waitFor(() => expect(host.showToast).toHaveBeenCalledTimes(shown.length))
    expect(host.showToast.mock.calls.map(([toast]) => toast)).toEqual(shown)
    expect(host.closeToast.mock.calls).toEqual(closed)
  })
})
