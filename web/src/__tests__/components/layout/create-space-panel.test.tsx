import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi, beforeEach } from 'vitest'
import type { ProjectDTO } from '@/lib/types'
import type { Host } from '@/features/hosts/types'

// Same subscribe-before-POST shape import-project-modal.test.tsx exercises:
// postProject answers 202 (void) and the real ProjectDTO arrives on the
// `/v0/projects` WS stream. We mock postProject (the 202 mutation) and the
// wsManager channel so the test can emit the canonical DTO frame the panel
// is waiting on.
const subscribers = new Set<(data: unknown) => void>()
function emit(data: unknown): void {
  subscribers.forEach((cb) => cb(data))
}

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (_endpoint: string, cb: (data: unknown) => void) => {
      subscribers.add(cb)
      return () => subscribers.delete(cb)
    },
    send: vi.fn(),
  },
}))

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    postProject: vi.fn((_name: string, _path: string, _hostId?: string) => Promise.resolve()),
  }
})

vi.mock('@/lib/native-dialog', () => ({
  openNativeDialog: vi.fn(),
}))

import * as api from '@/lib/api'
import { openNativeDialog } from '@/lib/native-dialog'
import { CreateSpacePanel } from '@/components/layout/create-space-panel'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'

function host(over: Partial<Host> & { id: string; name: string }): Host {
  return { endpoint: 'x', pairedFingerprint: 'x', clientId: 'c', ...over }
}

function projectDTO(over: Partial<ProjectDTO> & { id: string; path: string }): ProjectDTO {
  return {
    name: 'test-space',
    status: '',
    lastActivity: new Date().toISOString(),
    ...over,
  }
}

const daemonTrigger = () => screen.getByRole('combobox', { name: /daemon/i })

/** Waits for the popup's item to become the real, clickable thing (base-ui
 *  keeps every item mounted for measurement but `pointer-events: none`s the
 *  whole popup until `open` has actually propagated — see
 *  providers-settings.test.tsx's identical wait on the same quirk). */
async function selectDaemonOption(user: ReturnType<typeof userEvent.setup>, name: RegExp) {
  await user.click(daemonTrigger())
  const option = await screen.findByRole('option', { name })
  await waitFor(() => expect(getComputedStyle(option).pointerEvents).not.toBe('none'))
  await user.click(option)
}

beforeEach(() => {
  subscribers.clear()
  vi.mocked(api.postProject).mockReset()
  vi.mocked(api.postProject).mockResolvedValue(undefined)
  vi.mocked(openNativeDialog).mockReset()
  useHostConfigStore.setState({ hosts: [] })
})

describe('CreateSpacePanel — daemon picker', () => {
  it('defaults the daemon selection to Local', () => {
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    expect(daemonTrigger()).toHaveTextContent('Local')
  })

  it('lists a saved, reachable host as selectable', async () => {
    useHostConfigStore.setState({ hosts: [host({ id: 'h1', name: 'homelab-prox' })] })
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)

    await user.click(daemonTrigger())
    const option = await screen.findByRole('option', { name: /homelab-prox/i })
    expect(option).not.toHaveAttribute('aria-disabled', 'true')
  })

  it('lists an unreachable host but disabled, not hidden', async () => {
    useHostConfigStore.setState({ hosts: [host({ id: 'h1', name: 'office-vm' })] })
    getOrCreateHostRuntime('h1').getState().setStatus('down')
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)

    await user.click(daemonTrigger())
    const option = await screen.findByRole('option', { name: /office-vm/i })
    expect(option).toHaveAttribute('aria-disabled', 'true')
  })

  it('a trailing "+ Add a host…" entry is offered without becoming the selection', async () => {
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)

    await user.click(daemonTrigger())
    expect(await screen.findByRole('option', { name: /add a host/i })).toBeInTheDocument()
  })

  it('picking "+ Add a host…" opens AddHostDialog and leaves the daemon selection on Local', async () => {
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)

    await selectDaemonOption(user, /add a host/i)

    expect(screen.getByRole('dialog', { name: /add remote host/i })).toBeInTheDocument()

    // The dialog hides the rest of the page from the accessibility tree while
    // open — close it before checking the trigger never picked up the
    // sentinel value as a real selection.
    await user.click(screen.getByRole('button', { name: /cancel/i }))
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
    expect(daemonTrigger()).toHaveTextContent('Local')
  })

  it("creating a space passes the selected daemon's hostId to the create call", async () => {
    useHostConfigStore.setState({ hosts: [host({ id: 'h1', name: 'homelab-prox' })] })
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    vi.mocked(openNativeDialog).mockResolvedValue('/tmp/my-space')
    const onCreate = vi.fn()
    const user = userEvent.setup()

    render(<CreateSpacePanel onCreate={onCreate} onCancel={vi.fn()} />)

    await selectDaemonOption(user, /homelab-prox/i)
    await waitFor(() => expect(daemonTrigger()).toHaveTextContent('homelab-prox'))

    fireEvent.click(screen.getByRole('button', { name: /folder/i }))
    await waitFor(() => expect(screen.getByText('my-space')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /create space/i }))

    await waitFor(() => {
      expect(api.postProject).toHaveBeenCalledWith(expect.any(String), '/tmp/my-space', 'h1')
    })

    emit(projectDTO({ id: 'server-id', path: '/tmp/my-space', name: 'my-space' }))

    await waitFor(() => {
      expect(onCreate).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'server-id', path: '/tmp/my-space' }),
      )
    })
  })

  it('creating a space with the default Local daemon still works (regression: hostId threading has no effect on the local path)', async () => {
    vi.mocked(openNativeDialog).mockResolvedValue('/tmp/local-space')
    const onCreate = vi.fn()

    render(<CreateSpacePanel onCreate={onCreate} onCancel={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: /folder/i }))
    await waitFor(() => expect(screen.getByText('local-space')).toBeInTheDocument())

    fireEvent.click(screen.getByRole('button', { name: /create space/i }))

    await waitFor(() => {
      expect(api.postProject).toHaveBeenCalledWith(expect.any(String), '/tmp/local-space', 'local')
    })

    emit(projectDTO({ id: 'server-id-2', path: '/tmp/local-space', name: 'local-space' }))

    await waitFor(() => expect(onCreate).toHaveBeenCalled())
  })
})
