import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { HostProjectsPending } from '@/features/hosts/components/host-projects-pending'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { useProjectStore } from '@/core/entities/active-project'
import { projectDTO, resetEntities, seedProjects } from '@/__tests__/__fixtures__/sidebar-entities'

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
