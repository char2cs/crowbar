import { afterEach, describe, it, expect, vi, beforeEach } from 'vitest'
import type { Project } from '@/lib/types'

const redirectMock = vi.fn((opts: unknown) => {
  const err = new Error('redirect')
  Object.assign(err, { isRedirect: true, options: opts })
  return err
})

vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  createFileRoute: () => (config: unknown) => ({ options: config }),
  redirect: redirectMock,
  isRedirect: (e: unknown) => (e as { isRedirect?: boolean })?.isRedirect === true,
}))

vi.mock('@/lib/store/projects', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/store/projects')>()),
  useProjectStore: { getState: vi.fn(() => ({ activeProjectId: '' })) },
}))

const install = vi.hoisted(() => ({ localDaemon: true }))
vi.mock('@/features/hosts/lib/local-daemon', () => ({ hasLocalDaemon: () => install.localDaemon }))

const { connectProjectList, useProjectStore } = await import('@/lib/store/projects')
// The list's reads, by host; the guard reads nothing else.
const readLocal = vi.fn<(hostId: string) => Promise<Project[]>>()
connectProjectList({ read: (hostId) => readLocal(hostId), subscribe: () => () => {} })
const { useHostConfigStore } = await import('@/features/hosts/stores/host-config-store')
const { destroyHostRuntime, getOrCreateHostRuntime } =
  await import('@/features/hosts/stores/host-runtime-registry')

async function runBeforeLoad() {
  // Dynamically import to pick up mocks
  const mod = await import('@/routes/_shell/index')
  const config = (mod as { Route?: { options?: { beforeLoad?: () => Promise<void> } } }).Route
  await config?.options?.beforeLoad?.()
}

describe('_shell/index beforeLoad', () => {
  beforeEach(() => {
    redirectMock.mockClear()
    readLocal.mockReset()
    vi.mocked(useProjectStore.getState).mockReturnValue({
      activeProjectId: '',
    } as ReturnType<typeof useProjectStore.getState>)
  })

  it('redirects to /oobe when no projects', async () => {
    readLocal.mockResolvedValueOnce([])
    try {
      await runBeforeLoad()
    } catch (e) {
      expect(redirectMock).toHaveBeenCalledWith({ to: '/oobe' })
      return
    }
    // redirect should have thrown
    expect.fail('expected redirect to throw')
  })

  it('redirects to home route of active project', async () => {
    readLocal.mockResolvedValueOnce([
      {
        id: 'p1',
        name: 'Rabbyte',
        path: '/a',
        lastActivity: new Date('2024-01-01'),
        hostId: 'local',
      },
      {
        id: 'p2',
        name: 'Other',
        path: '/b',
        lastActivity: new Date('2024-01-01'),
        hostId: 'local',
      },
    ])
    vi.mocked(useProjectStore.getState).mockReturnValue({
      activeProjectId: 'p1',
    } as ReturnType<typeof useProjectStore.getState>)
    try {
      await runBeforeLoad()
    } catch (e) {
      expect(redirectMock).toHaveBeenCalledWith({
        to: '/ide/$projectId/home',
        params: { projectId: 'p1' },
      })
      return
    }
    expect.fail('expected redirect to throw')
  })

  // The sync engine asks for the list a moment after the guard did; the guard
  // must land on the newer answer, not on the store its own read left unpublished.
  it('redirects to a project when a newer read supersedes its own', async () => {
    const answers: Array<(projects: unknown) => void> = []
    readLocal.mockImplementation(
      () => new Promise((resolve) => answers.push(resolve as (projects: unknown) => void)),
    )
    const outcome = runBeforeLoad().catch((e: unknown) => e)
    await vi.waitFor(() => expect(answers).toHaveLength(1))
    const { useProjectDataStore } = await import('@/lib/store/projects')
    const newer = useProjectDataStore.getState().fetch()
    await vi.waitFor(() => expect(answers).toHaveLength(2))
    const project = {
      id: 'p3',
      name: 'Third',
      path: '/c',
      lastActivity: new Date('2024-01-01'),
      hostId: 'local',
    }
    answers[0]([project])
    answers[1]([project])
    await newer
    await outcome
    expect(redirectMock).toHaveBeenCalledWith({
      to: '/ide/$projectId/home',
      params: { projectId: 'p3' },
    })
  })

  it('falls back to first project when no active project', async () => {
    readLocal.mockResolvedValueOnce([
      {
        id: 'p2',
        name: 'Other',
        path: '/b',
        lastActivity: new Date('2024-01-01'),
        hostId: 'local',
      },
    ])
    vi.mocked(useProjectStore.getState).mockReturnValue({
      activeProjectId: '',
    } as ReturnType<typeof useProjectStore.getState>)
    try {
      await runBeforeLoad()
    } catch (e) {
      expect(redirectMock).toHaveBeenCalledWith({
        to: '/ide/$projectId/home',
        params: { projectId: 'p2' },
      })
      return
    }
    expect.fail('expected redirect to throw')
  })

  // Offline cold start: the cached list is what boot seeded, and the failed
  // read must not turn it into an error page.
  it('lands on the cached project when the list read fails', async () => {
    const { useProjectDataStore } = await import('@/lib/store/projects')
    const { loading, success, idle } = await import('@/lib/loadable')
    const cached = {
      id: 'p9',
      name: 'Cached',
      path: '/c',
      lastActivity: new Date(0),
      hostId: 'local',
    }
    useProjectDataStore.setState({ data: loading(success([cached])) })
    readLocal.mockRejectedValue(new Error('connection refused'))
    try {
      await expect(runBeforeLoad()).rejects.toMatchObject({
        isRedirect: true,
        options: { to: '/ide/$projectId/home', params: { projectId: 'p9' } },
      })
    } finally {
      useProjectDataStore.setState({ data: idle() })
    }
  })

  it('lands as soon as the local list is read, while a paired host that is up never answers', async () => {
    useHostConfigStore.getState().addHost({
      id: 'h1',
      name: 'h1',
      endpoint: 'h1.lan:9443',
      pairedFingerprint: 'ff',
      clientId: 'client-h1',
    })
    getOrCreateHostRuntime('h1').getState().setStatus('up')
    const local = {
      id: 'p2',
      name: 'Other',
      path: '/b',
      lastActivity: new Date(0),
      hostId: 'local',
    }
    readLocal.mockImplementation((hostId) =>
      hostId === 'local' ? Promise.resolve([local]) : new Promise<Project[]>(() => {}),
    )
    try {
      await expect(runBeforeLoad()).rejects.toMatchObject({
        isRedirect: true,
        options: { to: '/ide/$projectId/home', params: { projectId: 'p2' } },
      })
      expect(readLocal).toHaveBeenCalledWith('h1')
    } finally {
      useHostConfigStore.getState().removeHost('h1')
      destroyHostRuntime('h1')
    }
  })
})

// The phone app has no local daemon, so the local first-run flow (OOBE) does not exist there.
describe('_shell/index beforeLoad in the phone app', () => {
  beforeEach(() => {
    redirectMock.mockClear()
    readLocal.mockReset()
    install.localDaemon = false
  })
  afterEach(() => {
    install.localDaemon = true
    useHostConfigStore.setState({ hosts: [] })
  })

  it('sends an install with no host to the add-host screen, never to /oobe', async () => {
    readLocal.mockResolvedValue([])
    await expect(runBeforeLoad()).rejects.toMatchObject({ options: { to: '/add-host' } })
    expect(redirectMock).not.toHaveBeenCalledWith({ to: '/oobe' })
  })

  it('stays on the landing while a paired host has listed no project yet', async () => {
    useHostConfigStore.getState().addHost({
      id: 'h1',
      name: 'h1',
      endpoint: 'h1.lan:9443',
      pairedFingerprint: 'ff',
      clientId: 'client-h1',
    })
    readLocal.mockResolvedValue([])
    await runBeforeLoad()
    expect(redirectMock).not.toHaveBeenCalled()
  })
})
