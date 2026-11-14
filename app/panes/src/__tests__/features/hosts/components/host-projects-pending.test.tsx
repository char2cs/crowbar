import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { HostProjectsPending } from '@/features/hosts/components/host-projects-pending'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { useProjectStore } from '@/core/entities/active-project'
import { projectDTO, resetEntities, seedProjects } from '@/__tests__/__fixtures__/sidebar-entities'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'
import { streamDaemonFor } from '@/__tests__/__fixtures__/stream-daemon'
import * as localDaemon from '@/features/hosts/lib/local-daemon'

beforeEach(() => {
  useHostConfigStore.setState({
    hosts: [
      { id: 'h1', name: 'homelab', endpoint: 'h1:3737', pairedFingerprint: 'ff', clientId: 'c' },
    ],
  })
  resetEntities()
  useProjectStore.setState({ activeProjectId: '' })
})

afterEach(() => {
  useHostConfigStore.setState({ hosts: [] })
  resetEntities()
})

describe('HostProjectsPending', () => {
  it('names each paired host while none has listed a project', () => {
    render(<HostProjectsPending onProject={vi.fn()} onAddHost={vi.fn()} />)
    expect(screen.getByText('homelab')).toBeInTheDocument()
  })

  it('opens the active project as soon as a host lists it', () => {
    useProjectStore.setState({ activeProjectId: 'p2' })
    const onProject = vi.fn()
    render(<HostProjectsPending onProject={onProject} onAddHost={vi.fn()} />)
    expect(onProject).not.toHaveBeenCalled()

    act(() =>
      seedProjects([projectDTO({ id: 'p1', order: 0 }), projectDTO({ id: 'p2', order: 1 })]),
    )

    expect(onProject).toHaveBeenCalledWith('p2')
  })

  it('pairs another host on request', () => {
    const onAddHost = vi.fn()
    render(<HostProjectsPending onProject={vi.fn()} onAddHost={onAddHost} />)
    fireEvent.click(screen.getByRole('button', { name: /pair another host/i }))
    expect(onAddHost).toHaveBeenCalledOnce()
  })
})

// The phone: no daemon of its own, so every project it lists is one a host streamed.
describe('on the phone', () => {
  const h1 = streamDaemonFor('host-1')
  beforeEach(() => {
    vi.spyOn(localDaemon, 'hasLocalDaemon').mockReturnValue(false)
    resetHostWorld()
    pairRemoteHost('host-1', 'Studio')
  })
  afterEach(() => {
    vi.restoreAllMocks()
    resetHostWorld()
  })

  it('lands on the first project host-1 streams', async () => {
    const onProject = vi.fn()
    render(<HostProjectsPending onProject={onProject} onAddHost={vi.fn()} />)
    await act(async () => {})

    act(() => {
      for (const [id, order] of [
        ['p-b', 1],
        ['p-a', 0],
      ] as const) {
        h1.deliver({
          t: 'entity',
          scope: 'projects',
          kind: 'project',
          id,
          v: 1,
          data: projectDTO({ id, order }),
        })
      }
      h1.deliver({ t: 'synced', scope: 'projects' })
    })

    expect(onProject).toHaveBeenCalledWith('p-a')
  })

  it('says a host that has synced holds no project, and shows the status of one that has not', async () => {
    pairRemoteHost('host-2', 'Laptop')
    render(<HostProjectsPending onProject={vi.fn()} onAddHost={vi.fn()} />)
    await act(async () => {})

    act(() => h1.deliver({ t: 'synced', scope: 'projects' }))

    expect(screen.getByText('Studio').closest('li')).toHaveTextContent('No projects')
    expect(screen.getByText('Laptop').closest('li')).not.toHaveTextContent('No projects')
  })
})
