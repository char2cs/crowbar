import { describe, it, expect, beforeEach } from 'vitest'
import { isRedirect } from '@tanstack/react-router'
import { Route } from '@/routes/_shell/index'
import { useProjectStore } from '@/core/entities/active-project'
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
