import { render, screen, fireEvent, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { vi, expect, test, beforeEach, describe, afterEach } from 'vitest'
import type { RepoDTO, WorkspaceDTO } from '@/lib/types'

vi.mock('@/lib/api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/lib/api')>()
  return { ...actual, postRepo: vi.fn(), fetchRepos: vi.fn(), fetchWorkspaces: vi.fn() }
})

const navigateMock = vi.fn()
vi.mock('@tanstack/react-router', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@tanstack/react-router')>()
  return { ...actual, useNavigate: () => navigateMock }
})

import * as api from '@/lib/api'
import { AddRepositoryModal } from '@/components/projects/add-repository-modal'
import { OperationsWatcher } from '@/components/layout/operations-watcher'
import { deriveRows } from '@/features/workspace/tree/rows'
import { inputOf } from '@/__tests__/__fixtures__/tree-rows'
import { streamOperation, resetOperations } from '@/__tests__/__fixtures__/operations'
import { operationsState, type Operation, type OperationItem } from '@/core/entities/operations'
import { toastManager } from '@/lib/toast-manager'
import { useProjectStore } from '@/core/entities/active-project'
import * as fetchApi from '@/lib/api/fetch'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { getOrCreateHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import {
  loadProjects,
  pairRemoteHost,
  projectOn,
  resetHostWorld,
} from '@/__tests__/__fixtures__/remote-host'
import type { Problem } from '@/lib/errors/problem'
import {
  expectMigratingShown,
  expectReported,
  migratingRefusal,
  refusal,
  watchToasts,
} from '@/__tests__/__fixtures__/reports'
import { seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

function repoDTO(over: Partial<RepoDTO> = {}): RepoDTO {
  return {
    id: 'repo-9',
    path: '/tmp/my-repo',
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

function workspaceDTO(over: Partial<WorkspaceDTO> & { id: string }): WorkspaceDTO {
  return {
    repoId: 'repo-9',
    provisioning: 'provisioned',
    projectId: 'proj-1',
    branch: 'main',
    parentId: '',
    forkPointSha: '',
    status: 'new',
    working: false,
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

const repoItem: OperationItem = {
  ref: { kind: 'repo', id: 'repo-9' },
  label: 'Add my-repo',
  state: 'accepted',
}

/** The daemon's row for the add, at `version`. */
function addOp(opId: string, version: number, over: Partial<Operation> = {}): Operation {
  return {
    id: opId,
    kind: 'repo.create',
    subject: { kind: 'project', id: 'proj-1' },
    projectId: 'proj-1',
    label: 'Add my-repo',
    state: 'running',
    items: [repoItem],
    dismissed: false,
    version,
    createdAt: '2026-10-30T10:00:00Z',
    updatedAt: '2026-10-30T10:00:00Z',
    ...over,
  }
}

/** Starts the add; resolves with the operation id the modal sent. */
async function startAdd(onOpenChange: (open: boolean) => void = () => {}): Promise<string> {
  render(
    <>
      <OperationsWatcher />
      <AddRepositoryModal open={true} onOpenChange={onOpenChange} projectId="proj-1" />
    </>,
  )
  fireEvent.change(pathInput(), { target: { value: '/tmp/my-repo' } })
  fireEvent.click(addButton())
  await waitFor(() => expect(vi.mocked(api.postRepo)).toHaveBeenCalledTimes(1))
  return vi.mocked(api.postRepo).mock.calls[0][3]
}

const pathInput = () => screen.getByPlaceholderText('/absolute/path/to/repo')
const addButton = () => screen.getByRole('button', { name: /add repository/i })

const home = workspaceDTO({ id: 'ws-home', isDefault: true })

beforeEach(async () => {
  navigateMock.mockReset()
  vi.mocked(api.postRepo).mockReset()
  vi.mocked(api.postRepo).mockImplementation(async (_p, _n, _path, opId) => ({
    operation: addOp(opId, 1, { state: 'accepted' }),
  }))
  vi.mocked(api.fetchRepos).mockResolvedValue([repoDTO()])
  vi.mocked(api.fetchWorkspaces).mockResolvedValue([
    workspaceDTO({ id: 'ws-dev', branch: 'dev' }),
    home,
  ])
  // The project in view is another one: the modal adds to the project it was opened for.
  useProjectStore.setState({ activeProjectId: 'proj-other' })
  useHostConfigStore.setState({ hosts: [], remoteEverPaired: false })
  seedRepos([])
  resetOperations()
})

test('Add button is disabled when the path is empty', () => {
  render(<AddRepositoryModal open={true} onOpenChange={() => {}} projectId="proj-1" />)
  expect(addButton()).toBeDisabled()
})

test('Add stays disabled for a relative path and shows a hint', () => {
  render(<AddRepositoryModal open={true} onOpenChange={() => {}} projectId="proj-1" />)
  fireEvent.change(pathInput(), { target: { value: 'my-repo' } })
  expect(addButton()).toBeDisabled()
  expect(screen.getByText(/absolute path/i)).toBeInTheDocument()
})

test('posts the repo under its own project with the folder-name fallback', async () => {
  const opId = await startAdd()
  expect(vi.mocked(api.postRepo)).toHaveBeenCalledWith('proj-1', 'my-repo', '/tmp/my-repo', opId)
  expect(operationsState().ops[opId]).toMatchObject({
    kind: 'repo.create',
    label: 'Add my-repo',
  })
})

test('adding waits on the operation, then opens the default workspace', async () => {
  const onOpenChange = vi.fn()
  const opId = await startAdd(onOpenChange)
  await waitFor(() => expect(operationsState().ops[opId]?.version).toBe(1))
  expect(navigateMock).not.toHaveBeenCalled()

  streamOperation(
    addOp(opId, 2, { state: 'succeeded', items: [{ ...repoItem, state: 'succeeded' }] }),
  )

  await waitFor(() =>
    expect(navigateMock).toHaveBeenCalledWith({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: 'proj-1', repoId: 'repo-9', wsId: 'ws-home' },
    }),
  )
  expect(onOpenChange).toHaveBeenCalledWith(false)
})

test('a failed add re-enables the dialog; the reason comes from the operation', async () => {
  const add = vi.spyOn(toastManager, 'add')
  const onOpenChange = vi.fn()
  const opId = await startAdd(onOpenChange)
  await waitFor(() => expect(operationsState().ops[opId]?.version).toBe(1))

  const problem: Problem = { code: 'invalid_argument', message: 'not a git repository' }
  streamOperation(
    addOp(opId, 2, {
      state: 'failed',
      error: problem,
      items: [{ ...repoItem, state: 'failed', error: problem }],
    }),
  )

  await waitFor(() => expect(addButton()).not.toBeDisabled())
  expect(navigateMock).not.toHaveBeenCalled()
  expect(onOpenChange).not.toHaveBeenCalled()
  expect(add).toHaveBeenCalledTimes(1)
  expect(add).toHaveBeenCalledWith(
    expect.objectContaining({ title: 'Add my-repo failed', description: 'not a git repository' }),
  )
  add.mockRestore()
})

test('a partly failed add opens the repo home; the failed branch is drawn with its reason', async () => {
  const opId = await startAdd()
  await waitFor(() => expect(operationsState().ops[opId]?.version).toBe(1))

  const reason: Problem = { code: 'internal', message: 'mkdir: not a directory' }
  const partial = addOp(opId, 2, {
    state: 'failed',
    error: { code: 'partial', message: 'Add my-repo partly failed' },
    items: [
      { ...repoItem, state: 'succeeded' },
      { ref: { kind: 'workspace', id: 'ws-home' }, label: 'main', state: 'succeeded' },
      {
        ref: { kind: 'workspace', id: 'ws-develop' },
        label: 'develop',
        state: 'failed',
        error: reason,
      },
    ],
  })
  streamOperation(partial)

  await waitFor(() =>
    expect(navigateMock).toHaveBeenCalledWith(
      expect.objectContaining({
        params: { projectId: 'proj-1', repoId: 'repo-9', wsId: 'ws-home' },
      }),
    ),
  )
  const present = new Set(['ws-home'])
  const rows = deriveRows(
    inputOf({
      projectId: 'proj-1',
      operations: [partial],
      isPresent: (ref) => present.has(ref.id),
    }),
  )
  expect(rows).toEqual([
    expect.objectContaining({
      id: 'ws-develop',
      kind: 'branch',
      parentId: 'repo-9',
      label: 'develop',
      operation: { opId, state: 'failed', problem: reason },
    }),
  ])
})

test('a refused add re-enables the dialog and reports the daemon’s reason', async () => {
  const toasts = watchToasts()
  vi.mocked(api.postRepo).mockRejectedValueOnce(refusal('path does not exist', 'invalid_argument'))
  await startAdd()

  await waitFor(() => expect(addButton()).not.toBeDisabled())
  expect(navigateMock).not.toHaveBeenCalled()
  await waitFor(() => expectReported(toasts, 'Add my-repo failed', 'path does not exist'))
})

test('an add refused while the daemon migrates shows the migrating state, not a toast', async () => {
  const toasts = watchToasts()
  vi.mocked(api.postRepo).mockRejectedValueOnce(migratingRefusal())
  await startAdd()

  await waitFor(() => expectMigratingShown(toasts))
})

// The repo folder lives on the project's own daemon: Local's comes from the OS dialog as it
// always did, a remote project's from its host's filesystem — and an unknown host's from neither.
describe('the repository folder', () => {
  const invoke = vi.fn()
  const nativeDialogCalls = () => invoke.mock.calls.filter(([cmd]) => cmd === 'plugin:dialog|open')
  const browseButton = () => screen.getByRole('button', { name: 'Browse…' })
  const listed: Record<string, string[]> = { '/': ['srv'], '/srv': ['app'], '/srv/app': [] }
  const apiFetch = vi.spyOn(fetchApi, 'apiFetch')

  beforeEach(() => {
    resetHostWorld()
    invoke.mockReset()
    invoke.mockImplementation((cmd: string) =>
      cmd === 'plugin:dialog|open'
        ? Promise.resolve('/Users/me/code/app')
        : Promise.reject(new Error(`unexpected ${cmd}`)),
    )
    ;(window as unknown as Record<string, unknown>).__TAURI_INTERNALS__ = { invoke }
    loadProjects(projectOn('proj-1', 'local'))
    apiFetch.mockReset()
    apiFetch.mockImplementation((url: string) => {
      const path = new URL(url, 'http://x').searchParams.get('path')
      const base = path === '/' ? '' : path
      const entries = (listed[path ?? ''] ?? []).map((n) => ({
        name: n,
        path: `${base}/${n}`,
        isDir: true,
      }))
      return Promise.resolve(entries as never)
    })
  })

  afterEach(() => {
    delete (window as unknown as Record<string, unknown>).__TAURI_INTERNALS__
    resetHostWorld()
  })

  test("a local project's folder comes from the OS dialog, and no host is browsed", async () => {
    render(<AddRepositoryModal open={true} onOpenChange={() => {}} projectId="proj-1" />)

    fireEvent.click(browseButton())

    await waitFor(() => expect(pathInput()).toHaveValue('/Users/me/code/app'))
    expect(nativeDialogCalls()).toEqual([
      ['plugin:dialog|open', { options: { directory: true, multiple: false } }, undefined],
    ])
    expect(screen.queryByRole('dialog', { name: /choose a folder/i })).not.toBeInTheDocument()
    expect(apiFetch).not.toHaveBeenCalled()
  })

  test("a remote project's folder is browsed on its own host, never through the OS dialog", async () => {
    pairRemoteHost('host-2')
    getOrCreateHostRuntime('host-2').getState().setStatus('up')
    loadProjects(projectOn('proj-2', 'host-2'))
    const user = userEvent.setup()
    render(<AddRepositoryModal open={true} onOpenChange={() => {}} projectId="proj-2" />)

    await user.click(browseButton())
    const picker = await screen.findByRole('dialog', { name: /choose a folder/i })
    expect(within(picker).getByText("Browsing host-2's filesystem")).toBeInTheDocument()
    await user.click(await within(picker).findByRole('treeitem', { name: 'srv' }))
    await user.click(await within(picker).findByRole('treeitem', { name: 'app' }))
    await user.click(within(picker).getByRole('button', { name: /^select/i }))

    await waitFor(() => expect(pathInput()).toHaveValue('/srv/app'))
    expect(screen.getByLabelText(/repository name/i)).toHaveValue('app')
    expect(nativeDialogCalls()).toEqual([])
    const hosts = apiFetch.mock.calls.map((call) => call[3])
    expect(new Set(hosts)).toEqual(new Set(['host-2']))
  })

  test('a project whose host is not resolved browses nothing — not Local, not a guess', () => {
    pairRemoteHost('host-2')
    loadProjects(projectOn('proj-2', 'host-2'))
    render(<AddRepositoryModal open={true} onOpenChange={() => {}} projectId="proj-unknown" />)

    expect(browseButton()).toBeDisabled()
    fireEvent.click(browseButton())

    expect(nativeDialogCalls()).toEqual([])
    expect(screen.queryByRole('dialog', { name: /choose a folder/i })).not.toBeInTheDocument()
    expect(apiFetch).not.toHaveBeenCalled()
  })
})
