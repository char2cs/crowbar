import { describe, expect, it } from 'vitest'
import {
  ALL_HOSTS,
  landingProject,
  projectsOnHost,
  shownHostOf,
  sidebarHostIds,
} from '@/features/hosts/lib/host-filter'
import type { Host } from '@/features/hosts/types'
import type { Project } from '@/lib/types'

const project = (id: string, hostId: string, lastActivity = '2026-01-01T00:00:00Z'): Project => ({
  id,
  name: id,
  path: `/work/${id}`,
  lastActivity: new Date(lastActivity),
  hostId,
})

const remote = (id: string): Host => ({
  id,
  name: id,
  endpoint: `${id}.example:3737`,
  pairedFingerprint: '',
  clientId: '',
})

const HOSTS = ['local', 'h1', 'h2']
const PROJECTS = [project('a', 'local'), project('b', 'h1'), project('c', 'h2')]

describe('sidebarHostIds', () => {
  it('lists the local machine first, then every paired host', () => {
    expect(sidebarHostIds([remote('h1'), remote('h2')])).toEqual(HOSTS)
  })
})

describe('shownHostOf', () => {
  it('shows every host while the filter says all', () => {
    expect(shownHostOf(ALL_HOSTS, HOSTS, PROJECTS, 'b')).toBe(ALL_HOSTS)
  })

  it('shows the chosen host while the active project is on it', () => {
    expect(shownHostOf('h1', HOSTS, PROJECTS, 'b')).toBe('h1')
  })

  it('shows the chosen host while no project is active', () => {
    expect(shownHostOf('h1', HOSTS, PROJECTS, undefined)).toBe('h1')
  })

  // A deep link, the palette or a notification can activate any project.
  it('follows the active project onto another host', () => {
    expect(shownHostOf('h1', HOSTS, PROJECTS, 'c')).toBe('h2')
  })

  // Nothing to switch to there, so the choice would otherwise never show.
  it('keeps a chosen host that has no project, whichever project is active', () => {
    expect(shownHostOf('h3', [...HOSTS, 'h3'], PROJECTS, 'a')).toBe('h3')
  })

  it('shows all once the chosen host is no longer paired', () => {
    expect(shownHostOf('gone', HOSTS, PROJECTS, 'a')).toBe(ALL_HOSTS)
  })

  it('shows all when there is only one host, whatever was saved', () => {
    expect(shownHostOf('local', ['local'], [project('a', 'local')], 'a')).toBe(ALL_HOSTS)
  })
})

describe('projectsOnHost', () => {
  it('keeps every project for all hosts', () => {
    expect(projectsOnHost(PROJECTS, ALL_HOSTS)).toBe(PROJECTS)
  })

  it('keeps one host’s projects in order', () => {
    const two = [...PROJECTS, project('d', 'h1')]
    expect(projectsOnHost(two, 'h1').map((p) => p.id)).toEqual(['b', 'd'])
  })
})

describe('landingProject', () => {
  it('is the host’s most recently active project', () => {
    const list = [
      project('old', 'h1', '2026-01-01T00:00:00Z'),
      project('new', 'h1', '2026-03-01T00:00:00Z'),
      project('other', 'h2', '2026-09-01T00:00:00Z'),
    ]
    expect(landingProject(list, 'h1')?.id).toBe('new')
  })

  it('is the first when none has an activity time', () => {
    const list = [project('x', 'h1', 'nope'), project('y', 'h1', 'nope')]
    expect(landingProject(list, 'h1')?.id).toBe('x')
  })

  it('is nothing for a host without projects', () => {
    expect(landingProject(PROJECTS, 'h3')).toBeUndefined()
  })
})
