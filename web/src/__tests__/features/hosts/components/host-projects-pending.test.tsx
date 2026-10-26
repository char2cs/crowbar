import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { HostProjectsPending } from '@/features/hosts/components/host-projects-pending'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { useProjectDataStore, useProjectStore } from '@/lib/store/projects'
import { idle, success } from '@/lib/loadable'
import type { Project } from '@/lib/types'

const project = (id: string): Project =>
  ({ id, name: id, path: `/${id}`, lastActivity: new Date(0), hostId: 'h1' }) as Project

beforeEach(() => {
  useHostConfigStore.setState({
    hosts: [
      { id: 'h1', name: 'homelab', endpoint: 'h1:3737', pairedFingerprint: 'ff', clientId: 'c' },
    ],
  })
  useProjectDataStore.setState({ data: idle() })
  useProjectStore.setState({ activeProjectId: '' })
})

afterEach(() => {
  useHostConfigStore.setState({ hosts: [] })
  useProjectDataStore.setState({ data: idle() })
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

    act(() => useProjectDataStore.setState({ data: success([project('p1'), project('p2')]) }))

    expect(onProject).toHaveBeenCalledWith('p2')
  })

  it('pairs another host on request', () => {
    const onAddHost = vi.fn()
    render(<HostProjectsPending onProject={vi.fn()} onAddHost={onAddHost} />)
    fireEvent.click(screen.getByRole('button', { name: /pair another host/i }))
    expect(onAddHost).toHaveBeenCalledOnce()
  })
})
