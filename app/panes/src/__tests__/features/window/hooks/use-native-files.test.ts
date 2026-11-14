import { act, renderHook } from '@testing-library/react'
import { createElement, type ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { NativeChromeIntent, NativeFiles } from '@/lib/native-chrome'
import { useFileTreeStore } from '@/features/file-explorer/stores/file-explorer-tree-store'
import { useFocusedWorkspaceContextStore } from '@/features/window/stores/focused-workspace-context-store'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { getOrCreateWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import type { GitStatus } from '@/features/git/types/git-types'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'
import * as documents from '@/features/editor/documents/open-document'
import * as editorSurface from '@/features/panes/lib/editor-surface-loader'
import { workspaceRef } from '@/lib/api/documents'
import {
  projectDTO,
  resetEntities,
  seedHomeTree,
  seedProjects,
} from '@/__tests__/__fixtures__/sidebar-entities'

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
const opened = { settle: null as null | (() => void) }
const open = vi
  .spyOn(documents, 'openDocument')
  .mockImplementation(() => new Promise((resolve) => (opened.settle = () => resolve(null))))

const surface = { settle: null as null | (() => void) }
vi.spyOn(editorSurface, 'preloadEditorFor').mockImplementation(
  () => new Promise<void>((resolve) => (surface.settle = resolve)),
)

const { useNativeFiles } = await import('@/features/window/hooks/use-native-files')

const WS = 'ws-files'
const CHAT = 'chat-1'
const wrapper = ({ children }: { children: ReactNode }) =>
  createElement(WorkspaceStoreContext.Provider, { value: getOrCreateWorkspaceStore(WS) }, children)
const mount = () => renderHook(() => useNativeFiles(WS, CHAT), { wrapper })
const send = (intent: NativeChromeIntent) =>
  act(() => chrome.listeners.forEach((listener) => listener(intent)))
const last = () => chrome.pushed.at(-1)!

beforeEach(() => {
  chrome.pushed.length = 0
  chrome.listeners.clear()
  open.mockClear()
  useFileTreeStore.setState({ expandedPathsByWorkspace: {} })
  resetEntities()
  seedProjects([projectDTO({ id: 'p1', name: 'Rabbyte Local' })])
  seedHomeTree('p1', { chats: [{ id: CHAT, repoId: '', title: 'Nightly bug', order: 0 }] })
  useFocusedWorkspaceContextStore.setState({ workspaceId: WS, rootPath: '/repos/checkout' })
  useMobileScreenStore.setState({ screen: 'ide', filesOpen: true })
  const store = getOrCreateWorkspaceStore(WS)
  store.getState().fileTreeActions.setFiles([
    {
      name: 'src',
      path: 'src',
      isDir: true,
      children: [{ name: 'pricing.ts', path: 'src/pricing.ts', isDir: false }],
    },
    { name: 'README.md', path: 'README.md', isDir: false },
  ])
  store.setState({
    workspaceGitStatus: {
      files: [{ path: 'src/pricing.ts', status: 'modified', staged: false }],
    } as unknown as GitStatus,
  })
})

describe('useNativeFiles', () => {
  it('pushes the workspace’s tree with its git marks, and an empty one when it goes', () => {
    const { unmount } = mount()
    expect(last().header).toMatchObject({
      isThread: true,
      lead: { kind: 'project', project: { name: 'Rabbyte Local' } },
      route: { kind: 'project', name: 'Rabbyte Local' },
    })
    const [src, readme] = last().files
    expect(readme).toMatchObject({ name: 'README.md', isDir: false, git: null })
    expect(src.children?.[0]).toMatchObject({ name: 'pricing.ts', git: { statusLetter: 'M' } })
    expect(src.git).toMatchObject({ statusLetter: 'M' })
    unmount()
    expect(last().files).toEqual([])
    expect(last().header).toBeNull()
  })

  it('folds a folder the drawer asks to open, and says so in the next push', () => {
    mount()
    send({ type: 'toggleDir', path: 'src' })
    expect(last().expanded).toEqual(['src'])
  })

  // The drawer is the only thing on screen until the tab exists: closing it on the tap would
  // show the IDE's empty pane for as long as the file takes to load.
  it('opens a picked file as a preview and slides the drawer away once its tab can draw', async () => {
    mount()
    send({ type: 'openFile', path: 'README.md' })
    expect(open).toHaveBeenCalledWith(
      workspaceRef(WS, 'README.md'),
      expect.objectContaining({ preview: true }),
    )
    expect(useMobileScreenStore.getState().filesOpen).toBe(true)
    await act(async () => opened.settle!())
    // The tab exists, but its editor is still loading: the pane would be blank.
    expect(useMobileScreenStore.getState().filesOpen).toBe(true)
    await act(async () => surface.settle!())
    expect(useMobileScreenStore.getState().filesOpen).toBe(false)
  })
})
