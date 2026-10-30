import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useSidebarHostFilter } from '@/components/layout/use-sidebar-host-filter'
import { useSidebarStore } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'
import { getVisibleProjectIds } from '@/lib/store/project-visibility'
import {
  destroyHostRuntime,
  getHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'

vi.mock('@/lib/persistence/sidebar-ui', () => ({
  saveSidebarUI: vi.fn().mockResolvedValue(undefined),
  loadSidebarUI: vi.fn().mockResolvedValue(null),
}))

const at = (project: Project, lastActivity: string): Project => ({
  ...project,
  lastActivity: new Date(lastActivity),
})

const PROJECTS = [
  projectOn('a', 'local'),
  at(projectOn('b-old', 'h1'), '2026-01-01T00:00:00Z'),
  at(projectOn('b-new', 'h1'), '2026-06-01T00:00:00Z'),
  projectOn('c', 'h2'),
]

function setup(activeProjectId: string | undefined, projects = PROJECTS) {
  const onSelectProject = vi.fn()
  const view = renderHook(
    (props: { active: string | undefined }) =>
      useSidebarHostFilter(projects, props.active, onSelectProject),
    { initialProps: { active: activeProjectId } },
  )
  return { ...view, onSelectProject }
}

beforeEach(() => {
  resetHostWorld()
  useSidebarStore.setState({ hostFilter: '' })
})

describe('useSidebarHostFilter', () => {
  it('changes nothing with the local machine alone', () => {
    const { result } = setup('a', [projectOn('a', 'local'), projectOn('a2', 'local')])
    expect(result.current.hostIds).toEqual(['local'])
    expect(result.current.shown).toBeUndefined()
    expect(result.current.shownProjects.map((p) => p.id)).toEqual(['a', 'a2'])
  })

  describe('with two paired hosts', () => {
    beforeEach(() => {
      pairRemoteHost('h1', 'Build box')
      pairRemoteHost('h2', 'Laptop')
    })

    it('lists the local machine, then each paired host', () => {
      expect(setup('a').result.current.hostIds).toEqual(['local', 'h1', 'h2'])
    })

    it('lists the active project’s host before any host is chosen', () => {
      const { result } = setup('a')
      expect(result.current.shown).toBe('local')
      expect(result.current.shownProjects.map((p) => p.id)).toEqual(['a'])
    })

    it('lists only the chosen host’s projects', () => {
      useSidebarStore.setState({ hostFilter: 'h1' })
      const { result } = setup('b-old')
      expect(result.current.shown).toBe('h1')
      expect(result.current.shownProjects.map((p) => p.id)).toEqual(['b-old', 'b-new'])
    })

    it('choosing a host opens its most recently active project', () => {
      const { result, onSelectProject } = setup('a')
      act(() => result.current.selectHost('h1'))
      expect(useSidebarStore.getState().hostFilter).toBe('h1')
      expect(onSelectProject).toHaveBeenCalledExactlyOnceWith('b-new')
    })

    it('keeps the active project when it is already on the chosen host', () => {
      const { result, onSelectProject } = setup('b-old')
      act(() => result.current.selectHost('h1'))
      expect(onSelectProject).not.toHaveBeenCalled()
    })

    it('opens the host’s project even when the saved filter already names it', () => {
      useSidebarStore.setState({ hostFilter: 'h1' })
      const { result, onSelectProject } = setup('c')
      expect(result.current.shown).toBe('h2')
      act(() => result.current.selectHost('h1'))
      expect(onSelectProject).toHaveBeenCalledExactlyOnceWith('b-new')
    })

    // A deep link, the palette, the Recents view or a notification all just move the route.
    it('follows the active project to another host without writing the saved filter', () => {
      useSidebarStore.setState({ hostFilter: 'h1' })
      const { result, rerender } = setup('b-old')
      rerender({ active: 'c' })
      expect(result.current.shown).toBe('h2')
      expect(result.current.shownProjects.map((p) => p.id)).toEqual(['c'])
      expect(useSidebarStore.getState().hostFilter).toBe('h1')
    })

    // The filter is a view: the project streams and a host's connection don't read it.
    it('leaves the hidden hosts’ projects subscribed and their connections alone', () => {
      loadProjects(...PROJECTS)
      getOrCreateHostRuntime('h2').getState().setStatus('up')
      const { result } = setup('b-old')
      act(() => result.current.selectHost('h1'))
      expect(result.current.shownProjects.map((p) => p.id)).not.toContain('c')
      expect(getVisibleProjectIds().has('c')).toBe(true)
      expect(getHostRuntime('h2')?.getState().status).toBe('up')
      destroyHostRuntime('h2')
    })

    it('shows a chosen host without projects as empty and opens nothing', () => {
      pairRemoteHost('h3', 'Empty')
      const { result, onSelectProject } = setup('a')
      act(() => result.current.selectHost('h3'))
      expect(onSelectProject).not.toHaveBeenCalled()
      expect(result.current.shown).toBe('h3')
      expect(result.current.shownProjects).toEqual([])
    })
  })
})
