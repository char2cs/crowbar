import { act, render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import type { ProjectDTO } from '@/lib/types'
import type { Host } from '@/features/hosts/types'

// Same subscribe-before-POST shape import-project-modal.test.tsx exercises:
// postProject answers 202 (void) and the real ProjectDTO arrives on the
// `/v0/projects` WS stream. We mock postProject (the 202 mutation) and the
// wsManager channel so the test can emit the canonical DTO frame the panel
// is waiting on.
// Keyed by endpoint: the host a project is created on is the host whose stream is awaited.
const subscribers = new Map<string, Set<(data: unknown) => void>>()
function emit(endpoint: string, data: unknown): void {
  subscribers.get(endpoint)?.forEach((cb) => cb(data))
}

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (endpoint: string, cb: (data: unknown) => void) => {
      const forEndpoint = subscribers.get(endpoint) ?? new Set<(data: unknown) => void>()
      subscribers.set(endpoint, forEndpoint)
      forEndpoint.add(cb)
      return () => forEndpoint.delete(cb)
    },
    send: vi.fn(),
  },
}))

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    apiFetch: vi.fn(() => Promise.resolve()),
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
import { useConnectionStore } from '@/lib/ws/connection-store'

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
const folderButton = () => screen.getByRole('button', { name: /folder/i })
const createButton = () => screen.getByRole('button', { name: /create space/i })

// Each remote host's filesystem as its `GET /v0/system/browse` answers it.
let remoteFolders: Record<string, Record<string, string[]>> = {}
const browsed: Array<{ hostId?: string; path: string | null }> = []

function serveApi(url: string, _init?: RequestInit, _retry?: unknown, hostId?: string) {
  if (!url.startsWith('/v0/system/browse')) return Promise.resolve(undefined as never)
  const path = new URL(url, 'http://x').searchParams.get('path')
  browsed.push({ hostId, path })
  const names = remoteFolders[hostId ?? '']?.[path ?? ''] ?? []
  const base = path === '/' ? '' : path
  return Promise.resolve(
    names.map((n) => ({ name: n, path: `${base}/${n}`, isDir: true })) as never,
  )
}

/** Picks `/a/b/…` in the remote picker by opening each folder of it in turn. */
async function pickRemoteFolder(user: ReturnType<typeof userEvent.setup>, path: string) {
  fireEvent.click(folderButton())
  const dialog = await screen.findByRole('dialog', { name: /choose a folder/i })
  for (const name of path.split('/').filter(Boolean)) {
    await user.click(await within(dialog).findByRole('treeitem', { name }))
  }
  await user.click(within(dialog).getByRole('button', { name: /^select/i }))
  await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument())
}

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
  vi.mocked(api.apiFetch).mockReset()
  vi.mocked(api.apiFetch).mockImplementation(serveApi)
  remoteFolders = {}
  browsed.length = 0
  useHostConfigStore.setState({ hosts: [] })
  useConnectionStore.setState({ status: 'connected' })
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

  // Down, or refusing this client until paired again: listed, not hidden, but not choosable.
  it.each(['down', 'needs-repair'] as const)('lists a %s host but disabled', async (status) => {
    useHostConfigStore.setState({ hosts: [host({ id: 'h1', name: 'office-vm' })] })
    getOrCreateHostRuntime('h1').getState().setStatus(status)
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)

    await user.click(daemonTrigger())
    const option = await screen.findByRole('option', { name: /office-vm/i })
    expect(option).toHaveAttribute('aria-disabled', 'true')
  })

  it("Local's dot says what the app knows of the local daemon, like every other host dot", async () => {
    useConnectionStore.setState({ status: 'disconnected' })
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)

    expect(within(daemonTrigger()).getByTestId('host-status-local')).toHaveAccessibleName('Offline')
    await user.click(daemonTrigger())
    const local = await screen.findByRole('option', { name: /local/i })
    expect(within(local).getByTestId('host-status-local')).toHaveAccessibleName('Offline')
  })

  it('a selected host that stops answering cannot be created on until it answers again', async () => {
    useHostConfigStore.setState({ hosts: [host({ id: 'h1', name: 'homelab-prox' })] })
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    remoteFolders.h1 = { '/': ['later'] }
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await selectDaemonOption(user, /homelab-prox/i)
    await waitFor(() => expect(daemonTrigger()).toHaveTextContent('homelab-prox'))
    await pickRemoteFolder(user, '/later')
    const create = createButton()
    expect(create).toBeEnabled()

    act(() => getOrCreateHostRuntime('h1').getState().setStatus('down'))
    expect(create).toBeDisabled()

    act(() => getOrCreateHostRuntime('h1').getState().setStatus('up'))
    expect(create).toBeEnabled()
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

  // What was typed or confirmed in one add-host attempt must not carry into the next, even one
  // picked while the last is still closing.
  it('opens a fresh add-host attempt each time it is picked, before the last has closed', async () => {
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await selectDaemonOption(user, /add a host/i)
    const option = screen.getByRole('option', { name: /add a host/i, hidden: true })
    const hostName = () => within(screen.getByRole('dialog')).getByLabelText(/name/i)
    fireEvent.change(hostName(), { target: { value: 'homelab' } })

    act(() => {
      fireEvent.click(screen.getByRole('button', { name: /cancel/i }))
      fireEvent.pointerDown(option, { pointerType: 'mouse', buttons: 1 })
      fireEvent.click(option)
    })

    expect(hostName()).toHaveValue('')
  })

  it("creating a space passes the selected daemon's hostId to the create call", async () => {
    useHostConfigStore.setState({ hosts: [host({ id: 'h1', name: 'homelab-prox' })] })
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    remoteFolders.h1 = { '/': ['tmp'], '/tmp': ['my-space'] }
    const onCreate = vi.fn()
    const user = userEvent.setup()

    render(<CreateSpacePanel onCreate={onCreate} onCancel={vi.fn()} />)

    await selectDaemonOption(user, /homelab-prox/i)
    await waitFor(() => expect(daemonTrigger()).toHaveTextContent('homelab-prox'))

    await pickRemoteFolder(user, '/tmp/my-space')
    expect(folderButton()).toHaveTextContent('my-space')

    fireEvent.click(createButton())

    await waitFor(() => {
      expect(api.postProject).toHaveBeenCalledWith(expect.any(String), '/tmp/my-space', 'h1')
    })

    emit(
      'crowbar://h1/v0/projects',
      projectDTO({ id: 'server-id', path: '/tmp/my-space', name: 'my-space' }),
    )

    await waitFor(() => {
      expect(onCreate).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'server-id', path: '/tmp/my-space', hostId: 'h1' }),
      )
    })
  })

  it('the default Local daemon keeps its flow: OS dialog, local create, local stream', async () => {
    vi.mocked(openNativeDialog).mockResolvedValue('/tmp/local-space')
    const onCreate = vi.fn()

    render(<CreateSpacePanel onCreate={onCreate} onCancel={vi.fn()} />)

    fireEvent.click(folderButton())
    await waitFor(() => expect(folderButton()).toHaveTextContent('local-space'))
    expect(openNativeDialog).toHaveBeenCalledWith({ directory: true, multiple: false })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    fireEvent.click(createButton())

    await waitFor(() => {
      expect(api.postProject).toHaveBeenCalledWith(expect.any(String), '/tmp/local-space', 'local')
    })
    expect([...subscribers.keys()]).toEqual(['/v0/projects'])
    expect(browsed).toEqual([])

    emit(
      '/v0/projects',
      projectDTO({ id: 'server-id-2', path: '/tmp/local-space', name: 'local-space' }),
    )

    await waitFor(() =>
      expect(onCreate).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'server-id-2', hostId: 'local' }),
      ),
    )
  })
})

// A folder lives on the daemon the space is created on: Local's comes from the OS dialog as it
// always did, a remote host's from that host's own filesystem — never the other way round.
describe('CreateSpacePanel — folder', () => {
  beforeEach(() => {
    useHostConfigStore.setState({
      hosts: [host({ id: 'h1', name: 'homelab-prox' }), host({ id: 'h2', name: 'office-vm' })],
    })
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    getOrCreateHostRuntime('h2').getState().setStatus('up')
  })

  it("a remote daemon's folder is browsed on that host, never through the OS dialog", async () => {
    remoteFolders.h2 = { '/': ['srv'] }
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await selectDaemonOption(user, /office-vm/i)

    await pickRemoteFolder(user, '/srv')

    expect(folderButton()).toHaveTextContent('srv')
    expect(openNativeDialog).not.toHaveBeenCalled()
    expect(new Set(browsed.map((b) => b.hostId))).toEqual(new Set(['h2']))
  })

  it('a remote space needs a folder: Create waits for one and says so', async () => {
    remoteFolders.h1 = { '/': ['srv'] }
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await selectDaemonOption(user, /homelab-prox/i)

    expect(createButton()).toBeDisabled()
    expect(createButton()).toHaveAccessibleDescription('Choose a folder on homelab-prox first.')

    await pickRemoteFolder(user, '/srv')
    expect(createButton()).toBeEnabled()
    expect(screen.queryByText('Choose a folder on homelab-prox first.')).not.toBeInTheDocument()
  })

  it('a folder belongs to the daemon it was picked on, whichever way the daemon changes', async () => {
    vi.mocked(openNativeDialog).mockResolvedValue('/Users/me/code')
    remoteFolders.h1 = { '/': ['srv'] }
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    fireEvent.click(folderButton())
    await waitFor(() => expect(createButton()).toBeEnabled())

    await selectDaemonOption(user, /homelab-prox/i)
    expect(folderButton()).toHaveTextContent('Choose…')
    await pickRemoteFolder(user, '/srv')
    await selectDaemonOption(user, /local/i)

    expect(folderButton()).toHaveTextContent('Choose…')
    expect(createButton()).toBeDisabled()
    await selectDaemonOption(user, /homelab-prox/i)
    expect(folderButton()).toHaveTextContent('srv')
  })

  it('an OS dialog that answers after the daemon changed never lands on the new daemon', async () => {
    let answer!: (path: string) => void
    vi.mocked(openNativeDialog).mockReturnValue(new Promise((resolve) => (answer = resolve)))
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    fireEvent.click(folderButton())

    await selectDaemonOption(user, /homelab-prox/i)
    await act(async () => answer('/Users/me/code'))

    expect(folderButton()).toHaveTextContent('Choose…')
    expect(createButton()).toBeDisabled()
    fireEvent.click(createButton())
    expect(api.postProject).not.toHaveBeenCalled()
  })

  it('a selected host that is removed never leaves its folder to be created anywhere', async () => {
    remoteFolders.h1 = { '/': ['srv'] }
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await selectDaemonOption(user, /homelab-prox/i)
    await pickRemoteFolder(user, '/srv')

    act(() => useHostConfigStore.setState({ hosts: [host({ id: 'h2', name: 'office-vm' })] }))

    expect(daemonTrigger()).not.toHaveTextContent('homelab-prox')
    expect(createButton()).toBeDisabled()
    fireEvent.click(createButton())
    expect(api.postProject).not.toHaveBeenCalled()
  })
})

const PICKED = '/Users/me/icon.png'

/** Stages an image picked on this computer, as the desktop app does (OS dialog, asset URL). */
async function stageImage(user: ReturnType<typeof userEvent.setup>) {
  Object.assign(window, { __TAURI_INTERNALS__: {} })
  const image = Object.assign(new Response(null, { headers: { 'content-length': '6' } }), {
    blob: async () => new Blob(['pixels'], { type: 'image/png' }),
  })
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => image),
  )
  vi.mocked(openNativeDialog).mockResolvedValueOnce(PICKED)
  await user.click(screen.getByRole('button', { name: /edit space icon/i }))
  await user.click(await screen.findByRole('button', { name: /upload/i }))
  await user.keyboard('{Escape}')
}

afterEach(() => {
  delete (window as { __TAURI_INTERNALS__?: unknown }).__TAURI_INTERNALS__
  vi.unstubAllGlobals()
})

// The DTO is stamped with the host it was created on, so it must be read from that host's
// own stream: a local project at the same path must never be mistaken for the new remote one.
describe('CreateSpacePanel — provenance of the created project', () => {
  async function createOn(user: ReturnType<typeof userEvent.setup>) {
    await selectDaemonOption(user, /homelab-prox/i)
    await waitFor(() => expect(daemonTrigger()).toHaveTextContent('homelab-prox'))
    await pickRemoteFolder(user, '/tmp/shared')
    fireEvent.click(createButton())
    await waitFor(() => expect(api.postProject).toHaveBeenCalled())
  }

  beforeEach(() => {
    useHostConfigStore.setState({ hosts: [host({ id: 'h1', name: 'homelab-prox' })] })
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    remoteFolders.h1 = { '/': ['tmp'], '/tmp': ['shared'] }
  })

  it('awaits the selected host’s stream, not the local one', async () => {
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await createOn(user)

    expect([...subscribers.keys()]).toEqual(['crowbar://h1/v0/projects'])
  })

  it('resolves to the remote project when a local one has the same path', async () => {
    const onCreate = vi.fn()
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={onCreate} onCancel={vi.fn()} />)
    await createOn(user)

    emit('/v0/projects', projectDTO({ id: 'local-twin', path: '/tmp/shared' }))
    emit('crowbar://h1/v0/projects', projectDTO({ id: 'remote-id', path: '/tmp/shared' }))

    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(1))
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'remote-id', path: '/tmp/shared', hostId: 'h1' }),
    )
  })

  it('applies a staged icon to the project on the host it was created on', async () => {
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await user.click(screen.getByRole('button', { name: /edit space icon/i }))
    await user.click(await screen.findByRole('button', { name: /emoji/i }))
    await user.type(screen.getByPlaceholderText('Type an emoji…'), '🛰️')
    await user.keyboard('{Enter}')
    await user.keyboard('{Escape}')
    await createOn(user)

    emit('crowbar://h1/v0/projects', projectDTO({ id: 'remote-id', path: '/tmp/shared' }))

    await waitFor(() =>
      expect(api.apiFetch).toHaveBeenCalledWith(
        'crowbar://h1/v0/projects/remote-id/icon/emoji',
        expect.objectContaining({ method: 'PUT' }),
      ),
    )
  })

  it('sends a staged image to the host as its bytes, never its path on this computer', async () => {
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await stageImage(user)
    await createOn(user)

    emit('crowbar://h1/v0/projects', projectDTO({ id: 'remote-id', path: '/tmp/shared' }))

    const url = 'crowbar://h1/v0/projects/remote-id/icon'
    const put = () => vi.mocked(api.apiFetch).mock.calls.find(([called]) => called === url)?.[1]
    await waitFor(() => expect(put()).toBeDefined())
    const body = new TextDecoder().decode(put()?.body as Uint8Array)
    expect(body).toContain('name="icon"; filename="icon.png"')
    expect(body).toContain('pixels')
    expect(body).not.toContain(PICKED)
  })

  it('still hands the local daemon a staged image’s path, exactly as before', async () => {
    const user = userEvent.setup()
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await stageImage(user)
    vi.mocked(openNativeDialog).mockResolvedValueOnce('/tmp/mine')
    fireEvent.click(folderButton())
    await waitFor(() => expect(screen.getByText('mine')).toBeInTheDocument())
    fireEvent.click(createButton())
    await waitFor(() => expect(api.postProject).toHaveBeenCalled())

    emit('/v0/projects', projectDTO({ id: 'local-id', path: '/tmp/mine' }))

    await waitFor(() =>
      expect(api.apiFetch).toHaveBeenCalledWith('/v0/projects/local-id/icon', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: PICKED }),
      }),
    )
    expect(fetch).not.toHaveBeenCalled()
  })
})
