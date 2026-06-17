import { describe, it, expect, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { failed, success, idle } from '@/lib/loadable'
import { useProjectDataStore } from '@/lib/store/projects'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { ProjectListPage } from '@/components/projects/project-list-page'
import type { Project } from '@/lib/types'
import type { Repo } from '@/lib/store/sidebar'

const mockProject: Project = {
  id: 'p1',
  name: 'Rabbyte',
  path: '/dev/rabbyte',
  lastActivity: new Date(),
}

const mockRepo: Repo = {
  id: 'r1',
  projectId: 'p1',
  name: 'rabbyte',
  avatarLabel: 'R',
  avatarColor: '#000',
  workspaces: [],
}

beforeEach(() => {
  useProjectDataStore.setState({ data: success([]) })
  useWorkspaceListStore.setState({ data: idle() })
})

describe('ProjectListPage error vs empty', () => {
  it('shows inline error (not onboarding) when the fetch failed with no cache', () => {
    useProjectDataStore.setState({ data: failed(new Error('500'), idle()) })
    render(<ProjectListPage onSelect={() => {}} />)
    expect(screen.getByText(/failed to load/i)).toBeInTheDocument()
    expect(screen.queryByText(/no projects yet/i)).toBeNull()
  })

  it('shows project grid header when the fetch succeeded but list is empty', () => {
    useProjectDataStore.setState({ data: success([]) })
    render(<ProjectListPage onSelect={() => {}} />)
    expect(screen.getByText('Projects')).toBeInTheDocument()
    expect(screen.queryByText(/no projects yet/i)).toBeNull()
  })
})

describe('ProjectListPage no-repos empty state', () => {
  it('shows "No repositories yet" when projects exist but no repos', () => {
    useProjectDataStore.setState({ data: success([mockProject]) })
    useWorkspaceListStore.setState({ data: success([]) })

    render(<ProjectListPage onSelect={() => {}} />)
    expect(screen.getByText('No repositories yet')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /add repository/i })).toBeInTheDocument()
  })

  it('shows project grid when projects and repos both exist', () => {
    useProjectDataStore.setState({ data: success([mockProject]) })
    useWorkspaceListStore.setState({ data: success([mockRepo]) })

    render(<ProjectListPage onSelect={() => {}} />)
    expect(screen.queryByText('No repositories yet')).not.toBeInTheDocument()
    expect(screen.getByText('Projects')).toBeInTheDocument()
  })
})
