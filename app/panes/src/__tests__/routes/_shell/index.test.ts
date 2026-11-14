import { afterEach, describe, it, expect, beforeEach, vi } from 'vitest'
import { isRedirect } from '@tanstack/react-router'
import { Route } from '@/routes/_shell/index'
import { useProjectStore } from '@/core/entities/active-project'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { projectDTO, resetEntities, seedProjects } from '@/__tests__/__fixtures__/sidebar-entities'

const install = vi.hoisted(() => ({ localDaemon: true }))
vi.mock('@/features/hosts/lib/local-daemon', () => ({ hasLocalDaemon: () => install.localDaemon }))

function runBeforeLoad(): unknown {
  try {
    ;(Route.options.beforeLoad as () => void)()
    return null
  } catch (err) {
    return err
  }
}

const target = (err: unknown) => (err as { options: unknown }).options

describe('_shell/index beforeLoad', () => {
  beforeEach(() => {
    resetEntities()
    useProjectStore.setState({ activeProjectId: '' })
  })

  it('leaves the landing to wait while the projects scope has not answered', () => {
    expect(runBeforeLoad()).toBeNull()
  })

  it('redirects to /oobe when no projects', () => {
    seedProjects([])
    const err = runBeforeLoad()
    expect(isRedirect(err)).toBe(true)
    expect(target(err)).toMatchObject({ to: '/oobe' })
  })

  it('redirects to home route of active project', () => {
    seedProjects([projectDTO({ id: 'p1', order: 0 }), projectDTO({ id: 'p2', order: 1 })])
    useProjectStore.setState({ activeProjectId: 'p2' })
    expect(target(runBeforeLoad())).toMatchObject({
      to: '/ide/$projectId/home',
      params: { projectId: 'p2' },
    })
  })

  it('falls back to first project when no active project', () => {
    seedProjects([projectDTO({ id: 'p2', order: 0 })])
    expect(target(runBeforeLoad())).toMatchObject({ params: { projectId: 'p2' } })
  })
})

// The phone app has no local daemon, so the local first-run flow (OOBE) does not exist there.
describe('_shell/index beforeLoad in the phone app', () => {
  beforeEach(() => {
    resetEntities()
    install.localDaemon = false
  })
  afterEach(() => {
    install.localDaemon = true
    useHostConfigStore.setState({ hosts: [] })
  })

  it('sends an install with no host to the add-host screen, never to /oobe', () => {
    expect(target(runBeforeLoad())).toMatchObject({ to: '/add-host' })
  })

  it('stays on the landing while a paired host has listed no project yet', () => {
    useHostConfigStore.getState().addHost({
      id: 'h1',
      name: 'h1',
      endpoint: 'h1.lan:9443',
      pairedFingerprint: 'ff',
      clientId: 'client-h1',
    })
    expect(runBeforeLoad()).toBeNull()
  })
})
