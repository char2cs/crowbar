import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { vi, expect, test, beforeEach } from 'vitest'
import type { RepoDTO, WorkspaceDTO } from '@/lib/types'

// §3/§4 subscribe-before-POST: postRepo answers 202 (void). The daemon imports
// the repo AND auto-imports its default-branch workspace, broadcasting the
// RepoDTO on the per-project repos stream and the WorkspaceDTO on the per-repo
// workspaces stream. The modal subscribes both streams first (via awaitEntity
// → wsManager), fires the single postRepo POST, resolves the new repo by path,
// then resolves its default workspace and navigates into the IDE.
//
// We mock postRepo (the 202 mutation) and the wsManager so the test can emit
// the canonical DTO frames the modal is waiting on, plus useNavigate to assert
// the post-cache navigation target.
const subscribers = new Map<string, Set<(data: unknown) => void>>()
function emit(endpoint: string, data: unknown): void {
  subscribers.get(endpoint)?.forEach((cb) => cb(data))
}

vi.mock('@/lib/ws/manager', () => ({
  wsManager: {
    subscribe: (endpoint: string, cb: (data: unknown) => void) => {
      let set = subscribers.get(endpoint)
      if (!set) {
        set = new Set()
        subscribers.set(endpoint, set)
      }
      set.add(cb)
      return () => set!.delete(cb)
    },
    send: vi.fn(),
  },
}))

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return {
    ...actual,
    postRepo: vi.fn((_p: string, _name: string, _path: string) => Promise.resolve()),
  }
})

const navigateMock = vi.fn()
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return {
    ...actual,
    useNavigate: () => navigateMock,
  }
})

import * as api from '@/lib/api'
import { AddRepositoryModal } from '@/components/projects/add-repository-modal'
import { useProjectStore } from '@/lib/store/projects'
import { useSidebarStore } from '@/lib/store/sidebar'
import { wipeEntityCache } from '@/lib/persistence/idb'

function repoDTO(over: Partial<RepoDTO> & { id: string; path: string }): RepoDTO {
  return {
    projectId: 'proj-1',
    name: 'my-repo',
    defaultBranch: 'main',
    avatarLabel: 'M',
    avatarColor: 'bg-indigo-700',
    avatarUrl: '',
    avatarEmoji: '',
    ...over,
  }
}

function workspaceDTO(over: Partial<WorkspaceDTO> & { id: string; repoId: string }): WorkspaceDTO {
  return {
    projectId: 'proj-1',
    branch: 'main',
    parentId: '',
    forkPointSha: '',
    status: 'new',
    working: false,
    lastError: '',
    added: 0,
    deleted: 0,
    mergeStrategy: '',
    canMergeLocally: false,
    mergeConflicts: false,
    parentBranch: '',
    prUrl: '',
    prTitle: '',
    prTargetBranch: '',
    ...over,
  }
}

const pathInput = () => screen.getByPlaceholderText('/absolute/path/to/repo')

beforeEach(async () => {
  subscribers.clear()
  navigateMock.mockReset()
  vi.mocked(api.postRepo).mockReset()
  vi.mocked(api.postRepo).mockResolvedValue(undefined)
  useProjectStore.setState({ activeProjectId: 'proj-1' })
  useSidebarStore.setState({ repos: [] })
  await wipeEntityCache()
})

test('Add button is disabled when the path is empty', () => {
  render(<AddRepositoryModal open={true} onOpenChange={() => {}} />)
  expect(screen.getByRole('button', { name: /add repository/i })).toBeDisabled()
})

test('Add stays disabled for a relative path and shows a hint', () => {
  render(<AddRepositoryModal open={true} onOpenChange={() => {}} />)
  fireEvent.change(pathInput(), { target: { value: 'my-repo' } })
  expect(screen.getByRole('button', { name: /add repository/i })).toBeDisabled()
  expect(screen.getByText(/absolute path/i)).toBeInTheDocument()
})

test('posts the repo under the active project with the folder-name fallback', async () => {
  render(<AddRepositoryModal open={true} onOpenChange={() => {}} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/my-repo' } })
  fireEvent.click(screen.getByRole('button', { name: /add repository/i }))

  await waitFor(() => {
    const [projectId, name, path] = vi.mocked(api.postRepo).mock.calls[0]
    expect(projectId).toBe('proj-1')
    expect(name).toBe('my-repo')
    expect(path).toBe('/tmp/my-repo')
  })
})

test('subscribes the per-project repos stream then resolves the repo by path', async () => {
  render(<AddRepositoryModal open={true} onOpenChange={() => {}} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/my-repo' } })
  fireEvent.click(screen.getByRole('button', { name: /add repository/i }))

  // The repos stream is subscribed before the POST resolves the repo.
  await waitFor(() => {
    expect(subscribers.has('/v0/projects/proj-1/repos')).toBe(true)
    expect(vi.mocked(api.postRepo)).toHaveBeenCalledTimes(1)
  })
})

test('two-stage resolve: RepoDTO by path then default WorkspaceDTO, then navigates into the IDE', async () => {
  const onOpenChange = vi.fn()
  render(<AddRepositoryModal open={true} onOpenChange={onOpenChange} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/my-repo' } })
  fireEvent.click(screen.getByRole('button', { name: /add repository/i }))

  // Stage 1: the canonical RepoDTO (server-assigned id) arrives on the repos stream.
  await waitFor(() => expect(subscribers.has('/v0/projects/proj-1/repos')).toBe(true))
  emit('/v0/projects/proj-1/repos', repoDTO({ id: 'repo-9', path: '/tmp/my-repo' }))

  // Stage 2: the modal subscribes the per-repo workspaces stream for repo-9.
  await waitFor(() =>
    expect(subscribers.has('/v0/projects/proj-1/repos/repo-9/workspaces')).toBe(true),
  )
  emit(
    '/v0/projects/proj-1/repos/repo-9/workspaces',
    workspaceDTO({ id: 'ws-1', repoId: 'repo-9' }),
  )

  // Navigation targets /ide/:p/:r/:w with the server-assigned ids; modal closes.
  await waitFor(() => {
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: 'proj-1', repoId: 'repo-9', wsId: 'ws-1' },
    })
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})

test('seeds the new repo+workspace into the sidebar tree before navigating (no /ide bounce)', async () => {
  render(<AddRepositoryModal open={true} onOpenChange={() => {}} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/my-repo' } })
  fireEvent.click(screen.getByRole('button', { name: /add repository/i }))

  await waitFor(() => expect(subscribers.has('/v0/projects/proj-1/repos')).toBe(true))
  emit('/v0/projects/proj-1/repos', repoDTO({ id: 'repo-9', path: '/tmp/my-repo' }))
  await waitFor(() =>
    expect(subscribers.has('/v0/projects/proj-1/repos/repo-9/workspaces')).toBe(true),
  )
  emit(
    '/v0/projects/proj-1/repos/repo-9/workspaces',
    workspaceDTO({ id: 'ws-1', repoId: 'repo-9' }),
  )

  await waitFor(() => expect(navigateMock).toHaveBeenCalled())

  // By the time navigation fires, the sidebar tree already contains the new
  // workspace, so the /ide route guard won't treat it as unknown and bounce.
  const repo = useSidebarStore.getState().repos.find((r) => r.id === 'repo-9')
  expect(repo).toBeDefined()
  expect(repo!.workspaces.some((w) => w.id === 'ws-1')).toBe(true)
})

test('ignores a deleted-status workspace frame and waits for a live one', async () => {
  render(<AddRepositoryModal open={true} onOpenChange={() => {}} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/my-repo' } })
  fireEvent.click(screen.getByRole('button', { name: /add repository/i }))

  await waitFor(() => expect(subscribers.has('/v0/projects/proj-1/repos')).toBe(true))
  emit('/v0/projects/proj-1/repos', repoDTO({ id: 'repo-9', path: '/tmp/my-repo' }))

  await waitFor(() =>
    expect(subscribers.has('/v0/projects/proj-1/repos/repo-9/workspaces')).toBe(true),
  )
  // A deleted tombstone must NOT satisfy the resolve.
  emit(
    '/v0/projects/proj-1/repos/repo-9/workspaces',
    workspaceDTO({ id: 'ws-old', repoId: 'repo-9', status: 'deleted' }),
  )
  expect(navigateMock).not.toHaveBeenCalled()

  // The live default workspace resolves it.
  emit(
    '/v0/projects/proj-1/repos/repo-9/workspaces',
    workspaceDTO({ id: 'ws-1', repoId: 'repo-9', status: 'new' }),
  )
  await waitFor(() =>
    expect(navigateMock).toHaveBeenCalledWith(
      expect.objectContaining({ params: { projectId: 'proj-1', repoId: 'repo-9', wsId: 'ws-1' } }),
    ),
  )
})

test('resets loading and re-enables Add button when postRepo rejects', async () => {
  vi.mocked(api.postRepo).mockRejectedValueOnce(new Error('not a git repo'))

  render(<AddRepositoryModal open={true} onOpenChange={() => {}} />)
  fireEvent.change(pathInput(), { target: { value: '/tmp/my-repo' } })
  fireEvent.click(screen.getByRole('button', { name: /add repository/i }))

  await waitFor(() => {
    expect(screen.getByRole('button', { name: /add repository/i })).not.toBeDisabled()
    expect(screen.queryByText('Adding…')).not.toBeInTheDocument()
  })
  expect(navigateMock).not.toHaveBeenCalled()
})
