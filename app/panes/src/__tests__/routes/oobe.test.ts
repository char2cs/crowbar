import { afterEach, describe, expect, it, vi } from 'vitest'
import { isRedirect } from '@tanstack/react-router'

const install = vi.hoisted(() => ({ localDaemon: true }))
vi.mock('@/features/hosts/lib/local-daemon', () => ({ hasLocalDaemon: () => install.localDaemon }))

const { Route } = await import('@/routes/oobe')

function runBeforeLoad(): unknown {
  const beforeLoad = Route.options.beforeLoad as (() => void) | undefined
  try {
    beforeLoad?.()
    return undefined
  } catch (err) {
    return err
  }
}

afterEach(() => {
  install.localDaemon = true
})

describe('/oobe', () => {
  it('opens in the desktop app, which runs its own daemon', () => {
    expect(runBeforeLoad()).toBeUndefined()
  })

  // OOBE sets up the local daemon; the phone app has none, so it connects to a host instead.
  it('is unreachable in the phone app, which lands on the add-host screen', () => {
    install.localDaemon = false
    const result = runBeforeLoad()
    expect(isRedirect(result)).toBe(true)
    expect((result as { options: { to: string } }).options.to).toBe('/add-host')
  })
})
