import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, it, expect, vi } from 'vitest'
import type { Project } from '@/lib/types'
import { SidebarProjectSwitcher } from '@/components/layout/sidebar-project-switcher'
import { performRenameProject } from '@/components/sidebar/lib/row-actions'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import type { HostStatus } from '@/features/hosts/types'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

vi.mock('@/components/sidebar/lib/row-actions', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/components/sidebar/lib/row-actions')>()),
  performRenameProject: vi.fn().mockResolvedValue(undefined),
}))

const { apiFetch } = vi.hoisted(() => ({ apiFetch: vi.fn() }))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  apiFetch,
}))

beforeEach(() => {
  vi.clearAllMocks()
  apiFetch.mockResolvedValue(undefined)
})

const project = (id: string, name = id): Project => ({
  id,
  name,
  path: `/repos/${id}`,
  lastActivity: new Date('2026-08-28T00:00:00Z'),
  hostId: 'local',
})

describe('SidebarProjectSwitcher', () => {
  it('names the active project and leaves it out of the icon list', () => {
    render(
      <SidebarProjectSwitcher
        projects={[project('p1', 'Alpha'), project('p2', 'Beta')]}
        activeProjectId="p1"
        onSelectProject={vi.fn()}
        onAddProject={vi.fn()}
      />,
    )
    const heading = screen.getByTestId('active-project-name')
    expect(heading).toHaveTextContent('Alpha')
    expect(screen.queryByRole('button', { name: 'Alpha' })).toBeNull()
    expect(screen.getByRole('button', { name: 'Beta' })).toBeInTheDocument()
  })

  it('lists every other project and selects one on click', async () => {
    const onSelectProject = vi.fn()
    render(
      <SidebarProjectSwitcher
        projects={[project('p1'), project('p2'), project('p3')]}
        activeProjectId="p1"
        onSelectProject={onSelectProject}
        onAddProject={vi.fn()}
      />,
    )
    expect(screen.getAllByTestId('space-mark')).toHaveLength(2)
    await userEvent.click(screen.getByRole('button', { name: 'p3' }))
    expect(onSelectProject).toHaveBeenCalledWith('p3')
  })

  it('lists every project when none is active', () => {
    render(
      <SidebarProjectSwitcher
        projects={[project('p1'), project('p2')]}
        onSelectProject={vi.fn()}
        onAddProject={vi.fn()}
      />,
    )
    expect(screen.queryByTestId('active-project-name')).toBeNull()
    expect(screen.getAllByTestId('space-mark')).toHaveLength(2)
  })

  it('the trailing + calls onAddProject', async () => {
    const onAddProject = vi.fn()
    render(
      <SidebarProjectSwitcher
        projects={[project('p1'), project('p2')]}
        activeProjectId="p1"
        onSelectProject={vi.fn()}
        onAddProject={onAddProject}
      />,
    )
    const row = screen.getByTestId('sidebar-project-switcher')
    const add = within(row).getByRole('button', { name: 'Add space' })
    expect(screen.getByRole('button', { name: 'p2' }).compareDocumentPosition(add)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    )
    await userEvent.click(add)
    expect(onAddProject).toHaveBeenCalledOnce()
  })

  it('renders nothing with no projects and no add handler', () => {
    const { container } = render(<SidebarProjectSwitcher projects={[]} onSelectProject={vi.fn()} />)
    expect(container).toBeEmptyDOMElement()
  })

  // The retired tree project row held the space's rename and icon; the active
  // project's name and mark here are their home now.
  describe('editing the active project', () => {
    const renderActive = () =>
      render(
        <SidebarProjectSwitcher
          projects={[project('p1', 'Alpha'), project('p2', 'Beta')]}
          activeProjectId="p1"
          onSelectProject={vi.fn()}
        />,
      )

    it('double-clicking the name swaps in a focused inline input, not a dialog', () => {
      renderActive()
      fireEvent.doubleClick(screen.getByText('Alpha'))
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
      const input = screen.getByRole('textbox')
      expect(input).toHaveValue('Alpha')
      expect(input).toHaveFocus()
    })

    it('Enter renames the active project and closes the editor', () => {
      renderActive()
      fireEvent.doubleClick(screen.getByText('Alpha'))
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'Gamma' } })
      fireEvent.keyDown(input, { key: 'Enter' })
      expect(performRenameProject).toHaveBeenCalledWith('p1', 'Gamma')
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument()
    })

    it('Escape cancels without renaming', () => {
      renderActive()
      fireEvent.doubleClick(screen.getByText('Alpha'))
      const input = screen.getByRole('textbox')
      fireEvent.change(input, { target: { value: 'Gamma' } })
      fireEvent.keyDown(input, { key: 'Escape' })
      expect(performRenameProject).not.toHaveBeenCalled()
      expect(screen.getByTestId('active-project-name')).toHaveTextContent('Alpha')
    })

    it("the active mark opens the icon picker and an emoji persists to that project's REST base", async () => {
      const user = userEvent.setup()
      renderActive()
      await user.click(screen.getByRole('button', { name: /edit alpha icon/i }))
      await user.click(await screen.findByRole('button', { name: /emoji/i }))
      await user.type(screen.getByPlaceholderText('Type an emoji…'), '🛰️')
      await user.keyboard('{Enter}')
      expect(apiFetch).toHaveBeenCalledWith('/v0/projects/p1/icon/emoji', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ emoji: '🛰️' }),
      })
    })
  })

  // Local is the app itself: only a remote active project has a host to show.
  describe('the active project’s host', () => {
    const onHost = (id: string, name: string, hostId: string): Project => ({
      ...project(id, name),
      hostId,
    })
    const setStatus = (hostId: string, status: HostStatus) =>
      act(() => getOrCreateHostRuntime(hostId).getState().setStatus(status))
    const renderSwitcher = (projects: Project[]) =>
      render(
        <SidebarProjectSwitcher
          projects={projects}
          activeProjectId="p1"
          onSelectProject={vi.fn()}
        />,
      )

    beforeEach(() => {
      resetHostWorld()
      pairRemoteHost('h1', 'homelab-prox')
      pairRemoteHost('h2', 'studio')
    })
    afterEach(() => {
      destroyHostRuntime('h1')
      destroyHostRuntime('h2')
      resetHostWorld()
    })

    it('follows a remote project’s name, quiet while its host is online', () => {
      setStatus('h1', 'up')
      renderSwitcher([onHost('p1', 'Alpha', 'h1')])

      const badge = screen.getByRole('button', { name: 'Host settings: homelab-prox, Online' })
      expect(screen.getByTestId('active-project-name').compareDocumentPosition(badge)).toBe(
        Node.DOCUMENT_POSITION_FOLLOWING,
      )
      expect(badge).toContainElement(screen.getByTestId('host-status-h1'))
      expect(badge.textContent).toBe('')
    })

    it('says its host’s state beside the icon once the host is not online', () => {
      setStatus('h1', 'up')
      renderSwitcher([onHost('p1', 'Alpha', 'h1')])

      setStatus('h1', 'down')

      const badge = screen.getByRole('button', { name: 'Host settings: homelab-prox, Offline' })
      expect(within(badge).getByText('Offline')).toBeVisible()
    })

    it('shows nothing for a local project, even while a remote host is down', () => {
      setStatus('h1', 'down')
      renderSwitcher([project('p1', 'Alpha'), onHost('p2', 'Beta', 'h1')])

      expect(screen.queryByRole('button', { name: /^Host settings/ })).toBeNull()
      expect(screen.queryByText('Offline')).toBeNull()
      expect(screen.queryByTestId('host-status-h1')).toBeNull()
    })

    it('leaves the other projects’ marks undecorated, named for the project alone', () => {
      setStatus('h1', 'up')
      setStatus('h2', 'down')
      renderSwitcher([onHost('p1', 'Alpha', 'h1'), onHost('p2', 'Beta', 'h2')])

      expect(screen.getByRole('button', { name: 'Beta' })).toBeInTheDocument()
      expect(screen.queryByTestId('host-status-h2')).toBeNull()
      expect(screen.queryByText('Offline')).toBeNull()
    })
  })
})
