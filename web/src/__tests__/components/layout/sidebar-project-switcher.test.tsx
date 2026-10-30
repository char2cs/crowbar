import { act, fireEvent, render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, it, expect, vi } from 'vitest'
import type { Project } from '@/lib/types'
import { RowMetricsContext, TOUCH_ROW_METRICS } from '@/components/sidebar/lib/row-metrics'
import { SidebarProjectSwitcher } from '@/components/layout/sidebar-project-switcher'
import { performRenameProject } from '@/components/sidebar/lib/row-actions'
import {
  destroyHostRuntime,
  getOrCreateHostRuntime,
} from '@/features/hosts/stores/host-runtime-registry'
import type { HostStatus } from '@/features/hosts/types'
import { useUIState } from '@/features/window/stores/ui-state-store'
import { reportChannelState, resetConnectionState } from '@/lib/ws/connection-store'
import { loadProjects, pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

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
  // The phone's header draws the active project's icon larger than the desktop's.
  it('draws the active project icon larger under the touch scale', () => {
    const glyph = (touch: boolean) => {
      const { container, unmount } = render(
        <RowMetricsContext value={touch ? TOUCH_ROW_METRICS : null}>
          <SidebarProjectSwitcher
            projects={[project('p1', 'Alpha')]}
            activeProjectId="p1"
            onSelectProject={vi.fn()}
          />
        </RowMetricsContext>,
      )
      const size = Number(container.querySelector('svg')?.getAttribute('width'))
      unmount()
      return size
    }
    expect(glyph(true)).toBeGreaterThan(glyph(false))
  })

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

  it('the leading + calls onAddProject', async () => {
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
      Node.DOCUMENT_POSITION_PRECEDING,
    )
    await userEvent.click(add)
    expect(onAddProject).toHaveBeenCalledOnce()
  })

  describe('the leading + is revealed by hover only once the host has several projects', () => {
    const plusFor = (projects: Project[]) => {
      render(
        <SidebarProjectSwitcher
          projects={projects}
          activeProjectId={projects[0]?.id}
          onSelectProject={vi.fn()}
          onAddProject={vi.fn()}
        />,
      )
      return screen.getByRole('button', { name: 'Add space' })
    }

    it.each([[[]], [[project('p1')]]])('stays shown with %j projects', (projects) => {
      expect(plusFor(projects)).not.toHaveAttribute('data-reveal', 'hover')
    })

    it('is hover-revealed with two projects, and tabs in ahead of the other projects', async () => {
      const add = plusFor([project('p1'), project('p2')])
      expect(add).toHaveAttribute('data-reveal', 'hover')
      await userEvent.tab()
      await userEvent.tab()
      expect(add).toHaveFocus()
      await userEvent.tab()
      expect(screen.getByRole('button', { name: 'p2' })).toHaveFocus()
    })
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
    const hostLine = () => screen.getByRole('button', { name: /^Host settings/ })
    const name = () => screen.getByTestId('active-project-name')

    beforeEach(() => {
      resetHostWorld()
      pairRemoteHost('h1', 'homelab-prox')
      useUIState.setState({ isSettingsOpen: false, settingsInitialTab: 'appearance' })
    })
    afterEach(() => {
      destroyHostRuntime('h1')
      resetConnectionState()
      resetHostWorld()
    })

    it('names a remote project’s host under its name, quiet while it is online', () => {
      setStatus('h1', 'up')
      loadProjects(onHost('p1', 'Alpha', 'h1'))
      renderSwitcher([onHost('p1', 'Alpha', 'h1')])

      expect(hostLine()).toHaveAccessibleName('Host settings: homelab-prox, Online')
      expect(hostLine()).toHaveTextContent(/^homelab-prox$/)
      expect(hostLine()).toContainElement(screen.getByTestId('host-status-h1'))
      expect(hostLine()).not.toHaveAttribute('data-host-tone')
      // Its own line under the name, apart from the active mark: not a badge beside it.
      expect(hostLine().parentElement).toBe(name().parentElement)
      expect(name().compareDocumentPosition(hostLine())).toBe(Node.DOCUMENT_POSITION_FOLLOWING)
      expect(name().parentElement).not.toContainElement(
        screen.getByRole('button', { name: /edit alpha icon/i }),
      )
    })

    it.each([
      ['down', 'offline', 'Offline'],
      ['needs-repair', 'needs-attention', 'Needs pairing again'],
      ['probing', 'unsettled', 'Checking…'],
    ] as const)('a host that is %s tints the line %s and adds %j', (status, tone, word) => {
      setStatus('h1', 'up')
      renderSwitcher([onHost('p1', 'Alpha', 'h1')])

      setStatus('h1', status)

      expect(hostLine()).toHaveAccessibleName(`Host settings: homelab-prox, ${word}`)
      expect(hostLine()).toHaveTextContent(`homelab-prox · ${word}`)
      expect(hostLine()).toHaveAttribute('data-host-tone', tone)
    })

    it('stays quiet for a host that answers while its live streams are closed', () => {
      setStatus('h1', 'up')
      renderSwitcher([onHost('p1', 'Alpha', 'h1')])

      act(() => reportChannelState('crowbar://h1/v0/ws/git', false, 'h1'))

      expect(hostLine()).toHaveAccessibleName(
        'Host settings: homelab-prox, Online — no live stream',
      )
      expect(hostLine()).toHaveTextContent(/^homelab-prox$/)
      expect(hostLine()).not.toHaveAttribute('data-host-tone')
    })

    it('reads a host no longer configured as not available, in red', () => {
      renderSwitcher([onHost('p1', 'Alpha', 'h9')])

      expect(hostLine()).toHaveAccessibleName('Host settings: Host not available')
      expect(hostLine()).toHaveTextContent(/^Host not available$/)
      expect(hostLine()).toHaveAttribute('data-host-tone', 'offline')
    })

    it('opens Settings on the Hosts tab', async () => {
      setStatus('h1', 'down')
      renderSwitcher([onHost('p1', 'Alpha', 'h1')])

      await userEvent.setup().click(hostLine())

      expect(useUIState.getState()).toMatchObject({
        isSettingsOpen: true,
        settingsInitialTab: 'hosts',
      })
    })

    it('says the same on hover as it reads out', async () => {
      setStatus('h1', 'down')
      renderSwitcher([onHost('p1', 'Alpha', 'h1')])

      await userEvent.setup().hover(hostLine())

      expect(await screen.findByText('Host settings: homelab-prox, Offline')).toBeVisible()
    })

    it('keeps the host line under the rename input', () => {
      setStatus('h1', 'up')
      renderSwitcher([onHost('p1', 'Alpha', 'h1')])
      const block = name().parentElement

      fireEvent.doubleClick(name())

      expect(screen.getByRole('textbox').parentElement).toBe(block)
      expect(hostLine().parentElement).toBe(block)
    })

    // Truncation is layout jsdom does not run: this pins the structure it relies on.
    it('keeps a long host name whole on its own line, apart from the project name', () => {
      const long = 'homelab-prox-in-the-basement-rack-behind-the-old-nas-and-the-ups'
      pairRemoteHost('h1', long)
      setStatus('h1', 'up')
      renderSwitcher([onHost('p1', 'Alpha', 'h1')])

      expect(hostLine()).toHaveTextContent(long)
      expect(name()).toHaveTextContent(/^Alpha$/)
    })

    it('shows nothing for a local project, even while a remote host is down', () => {
      setStatus('h1', 'down')
      renderSwitcher([project('p1', 'Alpha'), onHost('p2', 'Beta', 'h1')])

      expect(screen.queryByRole('button', { name: /^Host settings/ })).toBeNull()
      expect(screen.queryByText(/Offline/)).toBeNull()
      // One line, with no room kept for a host line.
      expect(name().parentElement?.children).toHaveLength(1)
    })
  })

  // The other projects' marks: a remote one wears its host's dot, whichever project is active.
  describe('another project’s host', () => {
    const onHost = (id: string, name: string, hostId: string): Project => ({
      ...project(id, name),
      hostId,
    })
    const setStatus = (status: HostStatus) =>
      act(() => getOrCreateHostRuntime('h1').getState().setStatus(status))
    const renderSwitcher = (projects: Project[]) =>
      render(
        <SidebarProjectSwitcher
          projects={projects}
          activeProjectId="p1"
          onSelectProject={vi.fn()}
        />,
      )
    const mark = (name: string | RegExp) => screen.getByRole('button', { name })
    const dotIn = (el: HTMLElement) => within(el).queryByTestId(/^host-status-/)

    beforeEach(() => {
      resetHostWorld()
      pairRemoteHost('h1', 'homelab-prox')
    })
    afterEach(() => {
      destroyHostRuntime('h1')
      resetHostWorld()
    })

    it.each([
      ['up', 'online', 'Online'],
      ['down', 'offline', 'Offline'],
      ['needs-repair', 'needs-attention', 'Needs pairing again'],
      ['probing', 'unsettled', 'Checking…'],
    ] as const)(
      'a remote mark whose host is %s wears an %s dot, said once',
      (status, tone, word) => {
        setStatus(status)
        renderSwitcher([project('p1', 'Alpha'), onHost('p2', 'Beta', 'h1')])

        const beta = mark(`Beta, homelab-prox ${word}`)
        expect(dotIn(beta)).toHaveAttribute('data-host-tone', tone)
        expect(dotIn(beta)).toHaveAttribute('aria-hidden', 'true')
        expect(within(beta).queryByRole('img')).toBeNull()
      },
    )

    it('retints the dot and relabels the mark as the host changes', () => {
      setStatus('up')
      renderSwitcher([project('p1', 'Alpha'), onHost('p2', 'Beta', 'h1')])

      setStatus('down')

      expect(dotIn(mark('Beta, homelab-prox Offline'))).toHaveAttribute('data-host-state', 'down')
    })

    it('names a mark whose host is no longer configured for that', () => {
      renderSwitcher([project('p1', 'Alpha'), onHost('p2', 'Beta', 'h9')])

      expect(dotIn(mark('Beta, Host not available'))).toHaveAttribute('data-host-tone', 'offline')
    })

    it('leaves a local mark plain, and the active project’s own mark undotted', () => {
      setStatus('down')
      loadProjects(onHost('p1', 'Alpha', 'h1'))
      renderSwitcher([
        onHost('p1', 'Alpha', 'h1'),
        project('p2', 'Gamma'),
        onHost('p3', 'Beta', 'h1'),
      ])

      expect(dotIn(mark('Gamma'))).toBeNull()
      expect(dotIn(mark(/edit alpha icon/i))).toBeNull()
      expect(dotIn(mark('Beta, homelab-prox Offline'))).toHaveAttribute('data-host-tone', 'offline')
    })
  })
})
