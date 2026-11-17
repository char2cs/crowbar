import { afterEach, describe, it, expect, beforeEach } from 'vitest'
import { isRedirect } from '@tanstack/react-router'
import { Route } from '@/routes/_shell/index'
import { useProjectStore } from '@/core/entities/active-project'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { projectDTO, resetEntities, seedProjects } from '@/__tests__/__fixtures__/sidebar-entities'

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
  afterEach(() => {
    useHostConfigStore.setState({ hosts: [] })
  })

  it('redirects to home route of active project', () => {
    seedProjects([projectDTO({ id: 'p1', order: 0 }), projectDTO({ id: 'p2', order: 1 })])
    useProjectStore.setState({ activeProjectId: 'p2' })
    const err = runBeforeLoad()
    expect(isRedirect(err)).toBe(true)
    expect(target(err)).toMatchObject({
      to: '/ide/$projectId/home',
      params: { projectId: 'p2' },
    })
  })

  it('falls back to first project when no active project', () => {
    seedProjects([projectDTO({ id: 'p2', order: 0 })])
    expect(target(runBeforeLoad())).toMatchObject({ params: { projectId: 'p2' } })
  })

  it('sends an install with no host to the add-host screen', () => {
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
