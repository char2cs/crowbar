import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { createFileTreeHandlers, revealInFolder } from '@/features/files/lib/file-tree-handlers'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { toastManager } from '@/lib/toast-manager'
import { proxyMarkerError } from '@/lib/api/errors'
import { destroyHostRuntime } from '@/features/hosts/stores/host-runtime-registry'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

const { copyFileNode, revealItemInFinder, resolveWorkspaceRootPath } = vi.hoisted(() => ({
  copyFileNode: vi.fn(),
  revealItemInFinder: vi.fn().mockResolvedValue(undefined),
  resolveWorkspaceRootPath: vi.fn((): string | undefined => '/disk/worktree'),
}))

vi.mock('@/features/files/lib/file-tree-api', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/features/files/lib/file-tree-api')>()
  return { ...actual, copyFileNode }
})
vi.mock('@/lib/crowbar-bridge', () => ({ revealItemInFinder }))
vi.mock('@/lib/workspace/resolve-root-path', () => ({ resolveWorkspaceRootPath }))

const toastAdd = vi.spyOn(toastManager, 'add')

beforeEach(() => {
  vi.clearAllMocks()
  copyFileNode.mockResolvedValue(undefined)
})

afterEach(() => {
  destroyHostRuntime('h1')
  resetHostWorld()
})

const hostFailure = () => (
  pairRemoteHost('h1', 'Studio Mac'),
  proxyMarkerError('host-unreachable', 'h1')
)

describe('revealInFolder', () => {
  // The tab menu passes workspace-relative buffer paths, the explorer absolute
  // ones, and virtual buffers must never reach the OS.
  it('reveals absolute paths as-is and relative ones under the worktree root', () => {
    revealInFolder('ws-a', '/abs/path/file.ts')
    expect(revealItemInFinder).toHaveBeenCalledWith('/abs/path/file.ts')

    revealInFolder('ws-a', 'api/main.go')
    expect(revealItemInFinder).toHaveBeenCalledWith('/disk/worktree/api/main.go')
    // ...resolved against the workspace the menu belongs to, not the one in view.
    expect(resolveWorkspaceRootPath).toHaveBeenCalledExactlyOnceWith('ws-a')
  })

  it('does not reveal a virtual buffer or an unresolvable root', () => {
    revealInFolder('ws-a', 'remote://x/y.ts')
    resolveWorkspaceRootPath.mockReturnValueOnce(undefined)
    revealInFolder('ws-a', 'api/main.go')
    expect(revealItemInFinder).not.toHaveBeenCalled()
  })

  it('surfaces a failure as an error toast', async () => {
    revealItemInFinder.mockRejectedValueOnce(new Error('finder exploded'))
    revealInFolder('ws-a', '/abs/file.ts')
    await vi.waitFor(() =>
      expect(toastAdd).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Reveal in Finder failed',
          description: 'finder exploded',
        }),
      ),
    )
  })
})

describe('createFileTreeHandlers', () => {
  // Duplicate is ONE server-side copy (byte-faithful for binaries) to a
  // collision-free destination derived from the workspace's own loaded tree.
  it('duplicates into the next free "<name> copy" using this workspace’s tree', async () => {
    const store = createWorkspaceStore('ws-test')
    store.getState().fileTreeActions.setFiles([
      { name: 'img.png', path: 'img.png', isDir: false },
      { name: 'img copy.png', path: 'img copy.png', isDir: false },
    ])

    await createFileTreeHandlers('ws-test', store).handleDuplicatePath('img.png')

    expect(copyFileNode).toHaveBeenCalledWith('ws-test', 'img.png', 'img copy 2.png')
  })

  it.each([
    ['in its own words', () => new Error('workspace locked'), 'workspace locked'],
    ['naming the remote host that failed it', hostFailure, 'Couldn’t reach Studio Mac'],
  ])('surfaces a duplicate failure %s', async (_, failure, description) => {
    copyFileNode.mockRejectedValueOnce(failure())
    await createFileTreeHandlers('ws-test', createWorkspaceStore('ws-test')).handleDuplicatePath(
      'a.txt',
    )
    expect(toastAdd).toHaveBeenCalledWith(
      expect.objectContaining({ title: 'Duplicate failed', description }),
    )
  })

  it('starts an inline rename by marking the node editable, idempotently', async () => {
    const store = createWorkspaceStore('ws-test')
    store.getState().fileTreeActions.setFiles([{ name: 'a.ts', path: 'a.ts', isDir: false }])
    const { handleRenamePath } = createFileTreeHandlers('ws-test', store)

    await handleRenamePath('a.ts')
    await handleRenamePath('a.ts')

    expect(store.getState().files[0]).toMatchObject({ isRenaming: true, isEditing: true })
  })
})

describe('handleDuplicatePath destination naming', () => {
  const file = (path: string) => ({ path, name: path.split('/').pop() ?? path, isDir: false })

  async function duplicate(path: string, existing: string[] = []) {
    const store = createWorkspaceStore('ws-test')
    store.getState().fileTreeActions.setFiles([path, ...existing].map(file))
    await createFileTreeHandlers('ws-test', store).handleDuplicatePath(path)
    return copyFileNode.mock.calls[0][2]
  }

  it('inserts " copy" before the extension', async () => {
    expect(await duplicate('api/main.go')).toBe('api/main copy.go')
  })

  it('appends " copy" to an extension-less name', async () => {
    expect(await duplicate('src/utils')).toBe('src/utils copy')
  })

  it('treats a leading dot as part of the stem, not an extension', async () => {
    expect(await duplicate('.env')).toBe('.env copy')
  })

  it('keeps bumping past existing copies', async () => {
    expect(await duplicate('main.go', ['main copy.go', 'main copy 2.go'])).toBe('main copy 3.go')
  })
})
