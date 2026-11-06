import { act, render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, expect, it, test, vi, beforeEach, afterEach } from 'vitest'
import type { Host } from '@/features/hosts/types'

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, postProject: vi.fn() }
})
vi.mock('@/lib/api/fetch', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api/fetch')>()
  return { ...actual, apiFetch: vi.fn() }
})
vi.mock('@/lib/native-dialog', () => ({
  openNativeDialog: vi.fn(),
}))

import * as api from '@/lib/api'
import * as fetchApi from '@/lib/api/fetch'
import { openNativeDialog } from '@/lib/native-dialog'
import { CreateSpacePanel } from '@/components/layout/create-space-panel'
import { OperationsWatcher } from '@/components/layout/operations-watcher'
import { streamOperation, resetOperations } from '@/__tests__/__fixtures__/operations'
import { operationsState, type Operation } from '@/core/entities/operations'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { useConnectionStore } from '@/lib/ws/connection-store'
import type { Project } from '@/lib/types'
import {
  expectMigratingShown,
  expectReported,
  migratingRefusal,
  refusal,
  watchToasts,
} from '@/__tests__/__fixtures__/reports'
import { seedProjects } from '@/__tests__/__fixtures__/sidebar-entities'

function createOp(opId: string, version: number, state: Operation['state'], id = 'space-id') {
  return {
    id: opId,
    kind: 'project.create',
    subject: { kind: 'project', id },
    projectId: id,
    label: 'Add space',
    state,
    items: [{ ref: { kind: 'project', id }, label: 'space', state }],
    dismissed: false,
    version,
    createdAt: '2026-10-30T10:00:00Z',
    updatedAt: '2026-10-30T10:00:00Z',
  } satisfies Operation
}

function space(over: Partial<Project> & { id: string; path: string }): Project {
  return { name: 'space', lastActivity: new Date(0), hostId: 'local', ...over }
}

/** The create the daemon accepted as `opId` succeeds, its project `id` landing in the store
 *  from the host the create was posted to. */
async function settleCreate(id: string, path: string, others: Project[] = []) {
  await waitFor(() => expect(vi.mocked(api.postProject)).toHaveBeenCalled())
  const [, , opId, hostId] = vi.mocked(api.postProject).mock.calls[0]
  await waitFor(() => expect(operationsState().ops[opId]?.version).toBe(1))
  act(() => {
    seedProjects([...others, space({ id, path, hostId })])
    streamOperation(createOp(opId, 2, 'succeeded', id))
  })
}

function host(over: Partial<Host> & { id: string; name: string }): Host {
  return { endpoint: 'x', pairedFingerprint: 'x', clientId: 'c', ...over }
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

beforeEach(() => {
  vi.mocked(api.postProject).mockReset()
  vi.mocked(api.postProject).mockImplementation(async (_n, _p, opId) => ({
    operation: createOp(opId, 1, 'accepted'),
  }))
  vi.mocked(openNativeDialog).mockReset()
  vi.mocked(fetchApi.apiFetch).mockReset()
  vi.mocked(fetchApi.apiFetch).mockImplementation(serveApi)
  remoteFolders = {}
  browsed.length = 0
  resetOperations()
  seedProjects([])
  useHostConfigStore.setState({ hosts: [] })
  useConnectionStore.setState({ status: 'connected' })
})

test('import waits on the operation and hands over the project from the store', async () => {
  vi.mocked(openNativeDialog).mockResolvedValue('/tmp/space')
  const onCreate = vi.fn()
  render(<CreateSpacePanel onCreate={onCreate} onCancel={() => {}} />)
  fireEvent.click(folderButton())
  await waitFor(() => expect(createButton()).not.toBeDisabled())
  fireEvent.click(createButton())

  await waitFor(() => expect(vi.mocked(api.postProject)).toHaveBeenCalledTimes(1))
  const [name, path] = vi.mocked(api.postProject).mock.calls[0]
  expect([name, path]).toEqual(['space', '/tmp/space'])
  expect(onCreate).not.toHaveBeenCalled()
  await settleCreate('space-id', '/tmp/space')

  await waitFor(() =>
    expect(onCreate).toHaveBeenCalledWith(space({ id: 'space-id', path: '/tmp/space' })),
  )
})

async function refuseCreate(error: unknown) {
  vi.mocked(openNativeDialog).mockResolvedValue('/tmp/space')
  vi.mocked(api.postProject).mockRejectedValueOnce(error)
  render(
    <>
      <OperationsWatcher />
      <CreateSpacePanel onCreate={() => {}} onCancel={() => {}} />
    </>,
  )
  fireEvent.click(folderButton())
  await waitFor(() => expect(createButton()).not.toBeDisabled())
  fireEvent.click(createButton())
  await waitFor(() => expect(vi.mocked(api.postProject)).toHaveBeenCalledTimes(1))
}

test('a refused create reports the daemon’s reason', async () => {
  const toasts = watchToasts()
  await refuseCreate(refusal('a space already uses that folder'))
  await waitFor(() =>
    expectReported(toasts, 'Add “space” failed', 'a space already uses that folder'),
  )
})

test('a create refused while the daemon migrates shows the migrating state, not a toast', async () => {
  const toasts = watchToasts()
  await refuseCreate(migratingRefusal())
  await waitFor(() => expectMigratingShown(toasts))
})

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

/** Waits for the popup's item to become clickable: base-ui keeps every item
 *  mounted but sets `pointer-events: none` inline on the positioner until it is
 *  open. Reading that style, not the computed one, keeps each pick cheap. */
async function selectDaemonOption(user: ReturnType<typeof userEvent.setup>, name: RegExp) {
  await user.click(daemonTrigger())
  const option = await screen.findByRole('option', { name })
  await waitFor(() =>
    expect(option.closest<HTMLElement>('[role=presentation]')?.style.pointerEvents).not.toBe(
      'none',
    ),
  )
  await user.click(option)
}

describe('CreateSpacePanel — daemon picker', () => {
  it('a selected host that stops answering cannot be created on until it answers again, and says so', async () => {
    useHostConfigStore.setState({ hosts: [host({ id: 'h1', name: 'homelab-prox' })] })
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    remoteFolders.h1 = { '/': ['later'] }
    const user = userEvent.setup({ delay: null, pointerEventsCheck: 0 })
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await selectDaemonOption(user, /homelab-prox/i)
    await waitFor(() => expect(daemonTrigger()).toHaveTextContent('homelab-prox'))
    await pickRemoteFolder(user, '/later')
    const create = createButton()
    expect(create).toBeEnabled()

    act(() => getOrCreateHostRuntime('h1').getState().setStatus('down'))
    expect(create).toBeDisabled()
    expect(create).toHaveAccessibleDescription('homelab-prox is not answering.')
    act(() => getOrCreateHostRuntime('h1').getState().setStatus('needs-repair'))
    expect(create).toHaveAccessibleDescription('homelab-prox needs pairing again.')

    act(() => getOrCreateHostRuntime('h1').getState().setStatus('up'))
    expect(create).toBeEnabled()
    expect(create).not.toHaveAccessibleDescription()
  })

  it("creating a space passes the selected daemon's hostId to the create call", async () => {
    useHostConfigStore.setState({ hosts: [host({ id: 'h1', name: 'homelab-prox' })] })
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    remoteFolders.h1 = { '/': ['tmp'], '/tmp': ['my-space'] }
    const onCreate = vi.fn()
    const user = userEvent.setup({ delay: null, pointerEventsCheck: 0 })

    render(<CreateSpacePanel onCreate={onCreate} onCancel={vi.fn()} />)

    await selectDaemonOption(user, /homelab-prox/i)
    await waitFor(() => expect(daemonTrigger()).toHaveTextContent('homelab-prox'))

    await pickRemoteFolder(user, '/tmp/my-space')
    expect(folderButton()).toHaveTextContent('my-space')

    fireEvent.click(createButton())

    await waitFor(() => {
      expect(api.postProject).toHaveBeenCalledWith(
        expect.any(String),
        '/tmp/my-space',
        expect.any(String),
        'h1',
      )
    })

    await settleCreate('server-id', '/tmp/my-space')

    await waitFor(() => {
      expect(onCreate).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'server-id', path: '/tmp/my-space', hostId: 'h1' }),
      )
    })
  })

  it('the default Local daemon keeps its flow: OS dialog, local create', async () => {
    vi.mocked(openNativeDialog).mockResolvedValue('/tmp/local-space')
    const onCreate = vi.fn()

    render(<CreateSpacePanel onCreate={onCreate} onCancel={vi.fn()} />)

    fireEvent.click(folderButton())
    await waitFor(() => expect(folderButton()).toHaveTextContent('local-space'))
    expect(openNativeDialog).toHaveBeenCalledWith({ directory: true, multiple: false })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    fireEvent.click(createButton())

    await waitFor(() => {
      expect(api.postProject).toHaveBeenCalledWith(
        expect.any(String),
        '/tmp/local-space',
        expect.any(String),
        'local',
      )
    })
    expect(browsed).toEqual([])

    await settleCreate('server-id-2', '/tmp/local-space')

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
    const user = userEvent.setup({ delay: null, pointerEventsCheck: 0 })
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await selectDaemonOption(user, /office-vm/i)

    await pickRemoteFolder(user, '/srv')

    expect(folderButton()).toHaveTextContent('srv')
    expect(openNativeDialog).not.toHaveBeenCalled()
    expect(new Set(browsed.map((b) => b.hostId))).toEqual(new Set(['h2']))
  })

  it('a remote space needs a folder: Create waits for one and says so', async () => {
    remoteFolders.h1 = { '/': ['srv'] }
    const user = userEvent.setup({ delay: null, pointerEventsCheck: 0 })
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
    const user = userEvent.setup({ delay: null, pointerEventsCheck: 0 })
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
    const user = userEvent.setup({ delay: null, pointerEventsCheck: 0 })
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
    const user = userEvent.setup({ delay: null, pointerEventsCheck: 0 })
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
async function stageImage(user: ReturnType<typeof userEvent.setup>, size = 6) {
  Object.assign(window, { __TAURI_INTERNALS__: {} })
  const image = Object.assign(new Response(null, { headers: { 'content-length': `${size}` } }), {
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
  vi.restoreAllMocks()
})

// The project handed over is the one the operation names, on the host it was created on.
describe('CreateSpacePanel — the created project', () => {
  async function createOn(user: ReturnType<typeof userEvent.setup>) {
    await selectDaemonOption(user, /homelab-prox/i)
    await waitFor(() => expect(daemonTrigger()).toHaveTextContent('homelab-prox'))
    await pickRemoteFolder(user, '/tmp/shared')
    fireEvent.click(createButton())
    await settleCreate('remote-id', '/tmp/shared', [
      space({ id: 'local-twin', path: '/tmp/shared' }),
    ])
  }

  beforeEach(() => {
    useHostConfigStore.setState({ hosts: [host({ id: 'h1', name: 'homelab-prox' })] })
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    remoteFolders.h1 = { '/': ['tmp'], '/tmp': ['shared'] }
  })

  it('is the remote project the operation names when a local one has the same path', async () => {
    const onCreate = vi.fn()
    const user = userEvent.setup({ delay: null, pointerEventsCheck: 0 })
    render(<CreateSpacePanel onCreate={onCreate} onCancel={vi.fn()} />)
    await createOn(user)

    await waitFor(() => expect(onCreate).toHaveBeenCalledTimes(1))
    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'remote-id', path: '/tmp/shared', hostId: 'h1' }),
    )
  })

  it('applies a staged icon to the project on the host it was created on', async () => {
    const user = userEvent.setup({ delay: null, pointerEventsCheck: 0 })
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await user.click(screen.getByRole('button', { name: /edit space icon/i }))
    await user.click(await screen.findByRole('button', { name: /emoji/i }))
    await user.type(screen.getByPlaceholderText('Type an emoji…'), '🛰️')
    await user.keyboard('{Enter}')
    await user.keyboard('{Escape}')
    await createOn(user)

    await waitFor(() =>
      expect(fetchApi.apiFetch).toHaveBeenCalledWith(
        'crowbar://h1/v0/projects/remote-id/icon/emoji',
        expect.objectContaining({ method: 'PUT' }),
      ),
    )
  })

  it('sends a staged image to the host as its bytes, never its path on this computer', async () => {
    const user = userEvent.setup({ delay: null, pointerEventsCheck: 0 })
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await stageImage(user)
    await createOn(user)

    const url = 'crowbar://h1/v0/projects/remote-id/icon'
    const put = () =>
      vi.mocked(fetchApi.apiFetch).mock.calls.find(([called]) => called === url)?.[1]
    await waitFor(() => expect(put()).toBeDefined())
    const body = new TextDecoder().decode(put()?.body as Uint8Array)
    expect(body).toContain('name="icon"; filename="icon.png"')
    expect(body).toContain('pixels')
    expect(body).not.toContain(PICKED)
  })

  it('says why a staged image could not be sent to the host: its only clue', async () => {
    const toasts = watchToasts()
    const user = userEvent.setup({ delay: null, pointerEventsCheck: 0 })
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await stageImage(user, 3 * 1024 * 1024)
    await createOn(user)

    await waitFor(() =>
      expectReported(toasts, 'Failed to set icon', 'icon.png is larger than the 2 MB icon limit'),
    )
  })

  it('still hands the local daemon a staged image’s path, and reports its refusal', async () => {
    const toasts = watchToasts()
    vi.mocked(fetchApi.apiFetch).mockImplementation((url, ...rest) =>
      url.endsWith('/icon') ? Promise.reject(refusal('too big')) : serveApi(url, ...rest),
    )
    const user = userEvent.setup({ delay: null, pointerEventsCheck: 0 })
    render(<CreateSpacePanel onCreate={vi.fn()} onCancel={vi.fn()} />)
    await stageImage(user)
    vi.mocked(openNativeDialog).mockResolvedValueOnce('/tmp/mine')
    fireEvent.click(folderButton())
    await waitFor(() => expect(screen.getByText('mine')).toBeInTheDocument())
    fireEvent.click(createButton())
    await settleCreate('local-id', '/tmp/mine')

    await waitFor(() =>
      expect(fetchApi.apiFetch).toHaveBeenCalledWith('/v0/projects/local-id/icon', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: PICKED }),
      }),
    )
    await waitFor(() => expectReported(toasts, 'Failed to set icon', 'too big'))
    expect(fetch).not.toHaveBeenCalled()
  })
})
