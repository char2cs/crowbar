import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { NativeChromeIntent, NativeFiles } from '@/lib/native-chrome'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useFocusedWorkspaceContextStore } from '@/features/window/stores/focused-workspace-context-store'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { GitStatus } from '@/features/git/types/git-types'

const chrome = vi.hoisted(() => ({
  pushed: [] as NativeFiles[],
  listeners: new Set<(intent: NativeChromeIntent) => void>(),
}))
vi.mock('@/lib/native-chrome', () => ({
  setNativeFiles: (files: NativeFiles) => chrome.pushed.push(files),
  onNativeChromeIntent: (cb: (intent: NativeChromeIntent) => void) => {
    chrome.listeners.add(cb)
    return () => chrome.listeners.delete(cb)
  },
}))
const selected = vi.hoisted(() => vi.fn())
vi.mock('@/features/files/lib/file-tree-handlers', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/files/lib/file-tree-handlers')>()),
  createFileTreeHandlers: () => ({ handleFileSelect: selected }),
}))

const { useNativeFiles } = await import('@/features/window/hooks/use-native-files')

const WS = 'ws-files'
const send = (intent: NativeChromeIntent) =>
  act(() => chrome.listeners.forEach((listener) => listener(intent)))
const last = () => chrome.pushed.at(-1)!

beforeEach(() => {
  chrome.pushed.length = 0
  chrome.listeners.clear()
  selected.mockClear()
  useFileTreeStore.setState({ expandedPathsByWorkspace: {} })
  useFocusedWorkspaceContextStore.setState({ workspaceId: WS, rootPath: '/repos/checkout' })
  useMobileScreenStore.setState({ screen: 'ide', filesOpen: true })
  const store = getOrCreateWorkspaceStore(WS)
  store.getState().fileTreeActions.setFiles([
    {
      name: 'src',
      path: '/repos/checkout/src',
      isDir: true,
      children: [{ name: 'pricing.ts', path: '/repos/checkout/src/pricing.ts', isDir: false }],
    },
    { name: 'README.md', path: '/repos/checkout/README.md', isDir: false },
  ])
  store.setState({
    workspaceGitStatus: {
      files: [{ path: 'src/pricing.ts', status: 'modified', staged: false }],
    } as unknown as GitStatus,
  })
})

describe('useNativeFiles', () => {
  it('pushes the workspace’s tree with its git marks, and an empty one when it goes', () => {
    const { unmount } = renderHook(() => useNativeFiles(WS))
    expect(last().rootName).toBe('checkout')
    const [src, readme] = last().files
    expect(readme).toMatchObject({ name: 'README.md', isDir: false, git: null })
    expect(src.children?.[0]).toMatchObject({ name: 'pricing.ts', git: { statusLetter: 'M' } })
    expect(src.git).toMatchObject({ statusLetter: 'M' })
    unmount()
    expect(last().files).toEqual([])
  })

  it('folds a folder the drawer asks to open, and says so in the next push', () => {
    renderHook(() => useNativeFiles(WS))
    send({ type: 'toggleDir', path: '/repos/checkout/src' })
    expect(last().expanded).toEqual(['/repos/checkout/src'])
  })

  it('opens a picked file and slides the drawer away', () => {
    renderHook(() => useNativeFiles(WS))
    send({ type: 'openFile', path: '/repos/checkout/README.md' })
    expect(selected).toHaveBeenCalledWith('/repos/checkout/README.md', false, expect.anything())
    expect(useMobileScreenStore.getState().filesOpen).toBe(false)
  })
})
