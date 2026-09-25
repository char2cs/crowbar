import { describe, it, expect, vi, beforeEach } from 'vitest'

const redirectMock = vi.fn((opts: unknown) => {
  const err = new Error('redirect')
  Object.assign(err, { isRedirect: true, options: opts })
  return err
})

vi.mock('@tanstack/react-router', () => ({
  createFileRoute: () => (config: unknown) => ({ options: config }),
  redirect: redirectMock,
  isRedirect: (e: unknown) => (e as { isRedirect?: boolean })?.isRedirect === true,
}))

vi.mock('@/lib/api', () => ({
  fetchProjects: vi.fn(),
}))

vi.mock('@/lib/store/projects', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/store/projects')>()),
  useProjectStore: { getState: vi.fn(() => ({ activeProjectId: '' })) },
}))

const { fetchProjects } = await import('@/lib/api')
const { useProjectStore } = await import('@/lib/store/projects')

async function runBeforeLoad() {
  // Dynamically import to pick up mocks
  const mod = await import('@/routes/_shell/index')
  const config = (mod as { Route?: { options?: { beforeLoad?: () => Promise<void> } } }).Route
  await config?.options?.beforeLoad?.()
}

describe('_shell/index beforeLoad', () => {
  beforeEach(() => {
    redirectMock.mockClear()
    vi.mocked(fetchProjects).mockReset()
    vi.mocked(useProjectStore.getState).mockReturnValue({
      activeProjectId: '',
    } as ReturnType<typeof useProjectStore.getState>)
  })

  it('redirects to /oobe when no projects', async () => {
    vi.mocked(fetchProjects).mockResolvedValueOnce([])
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
    vi.mocked(fetchProjects).mockResolvedValueOnce([
      { id: 'p1', name: 'Rabbyte', path: '/a', lastActivity: new Date('2024-01-01') },
      { id: 'p2', name: 'Other', path: '/b', lastActivity: new Date('2024-01-01') },
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
    vi.mocked(fetchProjects).mockImplementation(
      () => new Promise((resolve) => answers.push(resolve as (projects: unknown) => void)),
    )
    const outcome = runBeforeLoad().catch((e: unknown) => e)
    await vi.waitFor(() => expect(answers).toHaveLength(1))
    const { useProjectDataStore } = await import('@/lib/store/projects')
    const newer = useProjectDataStore.getState().fetch()
    await vi.waitFor(() => expect(answers).toHaveLength(2))
    const project = { id: 'p3', name: 'Third', path: '/c', lastActivity: new Date('2024-01-01') }
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
    vi.mocked(fetchProjects).mockResolvedValueOnce([
      { id: 'p2', name: 'Other', path: '/b', lastActivity: new Date('2024-01-01') },
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
})
