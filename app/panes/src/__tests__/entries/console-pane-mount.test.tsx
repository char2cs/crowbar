import { afterEach, describe, expect, it, vi } from 'vitest'
import { useConsoleStore } from '@/features/console/stores/console-store'
import type { LifecycleMessage } from '@/sdk/lifecycle-message'
import { lifecycleDriver } from '../__fixtures__/lifecycle-client'

const stream = vi.hoisted(() => ({ opened: 0, closed: 0 }))

vi.mock('@/features/console/lib/controller', () => ({
  createConsoleController: () => {
    stream.opened += 1
    return { dispose: () => void (stream.closed += 1) }
  },
}))
vi.mock('@/entries/console-pane', () => ({ ConsolePane: () => null }))

const { ConsolePaneMount } = await import('@/entries/console-pane-mount')

function startedMount() {
  const { client, deliver } = lifecycleDriver()
  new ConsolePaneMount(document.createElement('div'), client).start()
  return (event: LifecycleMessage['event']) =>
    deliver({ paneId: 'p1', event, params: {}, state: null })
}

afterEach(() => {
  stream.opened = 0
  stream.closed = 0
  useConsoleStore.setState({ open: false })
})

describe('ConsolePaneMount', () => {
  it('keeps the log stream open exactly while the pane is mounted', () => {
    const send = startedMount()
    send('mount')
    expect(useConsoleStore.getState().open).toBe(true)
    expect(stream.opened).toBe(1)
    send('destroy')
    expect(useConsoleStore.getState().open).toBe(false)
    expect(stream.closed).toBe(1)
  })
})
