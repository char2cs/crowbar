import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { NativeChromeIntent, NativeSidebar } from '@/lib/native-chrome'
import { getInitialState, useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { useFolderSignalStore } from '@/lib/store/folder-signal'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import { useUIState } from '@/features/window/stores/ui-state-store'
import {
  resetWindowPaneStoreForTests,
  windowPaneStore,
} from '@/features/panes/stores/window-pane-store'
import type { Project } from '@/lib/types'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { getInitialPendingCreatesState, usePendingCreatesStore } from '@/lib/store/pending-creates'
import { useActiveSessionsStore } from '@/lib/store/active-sessions'
import { recordWorkspaceScope } from '@/lib/workspace-scope'

const chrome = vi.hoisted(() => ({
  pushed: [] as NativeSidebar[],
  listeners: new Set<(intent: NativeChromeIntent) => void>(),
  prompt: vi.fn(async (): Promise<string | null> => null),
}))
vi.mock('@/lib/native-chrome', () => ({
  setNativeSidebar: (sidebar: NativeSidebar) => chrome.pushed.push(sidebar),
  onNativeChromeIntent: (cb: (intent: NativeChromeIntent) => void) => {
    chrome.listeners.add(cb)
    return () => chrome.listeners.delete(cb)
  },
  nativePrompt: chrome.prompt,
}))
const kill = vi.hoisted(() => vi.fn(async () => true))
vi.mock('@/features/agent/lib/kill-agent', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/lib/kill-agent')>()),
  killAgent: kill,
}))
const navigate = vi.hoisted(() => vi.fn(async () => {}))
vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  setWorkspaceLock: vi.fn(async () => {}),
  apiFetch: vi.fn(async () => ({ workspace: { id: 'ws-a', order: 1 }, shifted: [] })),
}))
vi.mock('@tanstack/react-router', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@tanstack/react-router')>()),
  useNavigate: () => navigate,
}))

const api = await import('@/lib/api')
const { useNativeSidebar } = await import('@/features/window/hooks/use-native-sidebar')

const repo: Repo = {
  id: 'r1',
  projectId: 'p1',
  name: 'checkout',
  avatarLabel: 'C',
  avatarColor: '',
  defaultWorkspaceId: 'home-1',
  defaultOwningChatId: 'c-home',
  workspaces: [
    { id: 'ws-a', branch: 'alpha', age: '', order: 0, owningChatId: 'c-a' },
    { id: 'ws-b', branch: 'beta', age: '', order: 1, owningChatId: 'c-b' },
  ],
  chats: [
    { id: 'c-home', repoId: 'r1', ownsWorktree: true, workspaceId: 'home-1', title: '', order: 0 },
    { id: 'c-a', repoId: 'r1', ownsWorktree: true, workspaceId: 'ws-a', title: 'Alpha', order: 1 },
    { id: 'c-b', repoId: 'r1', ownsWorktree: true, workspaceId: 'ws-b', title: 'Beta', order: 2 },
  ],
}
const projects = [{ id: 'p1', name: 'Crowbar Seed', hostId: 'h1' } as Project]
const send = (intent: NativeChromeIntent) =>
  act(() => chrome.listeners.forEach((listener) => listener(intent)))
const last = () => chrome.pushed.at(-1)!

beforeEach(() => {
  chrome.pushed.length = 0
  chrome.listeners.clear()
  navigate.mockClear()
  vi.mocked(api.apiFetch).mockClear()
  resetWindowPaneStoreForTests()
  useRemovalTrayStore.setState(getInitialRemovalState())
  usePendingCreatesStore.setState(getInitialPendingCreatesState())
  useSidebarStore.setState({ ...getInitialState(), repos: [repo] })
  useFolderSignalStore.setState({ seededRepoIds: new Set(['r1']) })
  useMobileScreenStore.setState({ screen: 'chat', chatListOpen: true })
  useUIState.setState({ isSettingsOpen: false })
  useActiveSessionsStore.setState({ sessions: {}, attached: {} })
  kill.mockClear()
})

const mount = (onSelectProject = vi.fn()) =>
  renderHook(() => useNativeSidebar({ activeProjectId: 'p1', projects, onSelectProject }))

describe('useNativeSidebar', () => {
  it('pushes the active project’s desktop rows, and an empty list when it goes', () => {
    const { unmount } = mount()
    expect(last().activeProjectId).toBe('p1')
    expect(last().projects).toEqual(projects)
    expect(last().rows.map((r) => r.label)).toContain('Alpha')
    unmount()
    expect(last()).toEqual({
      focusedChatId: null,
      activeProjectId: null,
      projects: [],
      hosts: [],
      rows: [],
      active: [],
      collapsed: [],
      actions: {},
      drop: null,
    })
  })

  // The page draws the desktop row's own "has a view" ground on the chat on screen.
  it('marks the focused chat as the one with a view', () => {
    mount()
    act(() =>
      windowPaneStore
        .getState()
        .paneActions.openChat('c-a', { projectId: 'p1', workspaceId: 'ws-a' }),
    )
    expect(last().focusedChatId).toBe('c-a')
    expect(
      last()
        .rows.filter((r) => r.hasView)
        .map((r) => r.id),
    ).toEqual(['c-a'])
  })

  it('draws a create in flight as a spinning row with no menu, until it clears or fails', () => {
    mount()
    act(() =>
      usePendingCreatesStore.getState().addCreating({
        tempId: 'tmp-1',
        kind: 'branch',
        projectId: 'p1',
        parentId: 'r1',
        order: 3,
        workspaceId: null,
        ownsWorktree: true,
        label: 'gamma',
      }),
    )
    const pending = last().rows.find((r) => r.id === 'tmp-1')
    expect(pending?.pending?.status).toBe('creating')
    expect(pending?.working).toBe(true)
    expect(last().actions['tmp-1']).toEqual([])

    act(() => usePendingCreatesStore.getState().setError('tmp-1', 'boom'))
    expect(last().rows.some((r) => r.id === 'tmp-1')).toBe(false)

    act(() => usePendingCreatesStore.getState().clear('tmp-1'))
    expect(last().rows.some((r) => r.pending)).toBe(false)
  })

  it('folds a row the page asks to fold, and says so in the next push', () => {
    mount()
    send({ type: 'toggleFold', id: 'c-home' })
    expect(useSidebarStore.getState().collapsedChatRows.has('c-home')).toBe(true)
    expect(last().collapsed).toEqual(['c-home'])
  })

  it('opens a chosen chat and puts the list away', () => {
    mount()
    const alpha = last().rows.find((r) => r.label === 'Alpha')!
    send({ type: 'selectRow', id: alpha.id })
    expect(navigate).toHaveBeenCalledWith({
      to: '/ide/$projectId/$repoId/$wsId',
      params: { projectId: 'p1', repoId: 'r1', wsId: 'ws-a' },
    })
    expect(useMobileScreenStore.getState().chatListOpen).toBe(false)
  })

  // The phone folds only from a row's disclosure: a tap on a container's body leaves it open.
  it('leaves a container open when its body is picked', () => {
    mount()
    send({ type: 'selectRow', id: 'c-home' })
    expect(useSidebarStore.getState().collapsedChatRows.has('c-home')).toBe(false)
  })

  it('switches project', () => {
    const onSelectProject = vi.fn()
    mount(onSelectProject)
    send({ type: 'selectProject', id: 'p1' })
    expect(onSelectProject).toHaveBeenCalledWith('p1')
  })

  it('opens Settings over a closed list', () => {
    mount()
    send({ type: 'openSettings' })
    expect(useUIState.getState().isSettingsOpen).toBe(true)
    expect(useMobileScreenStore.getState().chatListOpen).toBe(false)
  })
  // The long-press menu holds every desktop verb, the row menu's and its hover buttons'.
  it('gives each row every desktop verb, Remove last and asked first', () => {
    mount()
    const alpha = last().rows.find((r) => r.label === 'Alpha')!
    expect(last().actions[alpha.id].map((a) => a.id)).toEqual([
      'rename',
      'lock',
      'new-folder',
      'new-thread-terminal',
      'new-thread',
      'fork',
      'remove',
    ])
    expect(last().actions[alpha.id].at(-1)).toMatchObject({
      title: 'Remove',
      destructive: true,
      confirm: { title: 'Remove branch?', message: expect.stringContaining('Alpha') },
    })
    expect(last().actions[alpha.id][1]).toEqual({
      id: 'lock',
      title: 'Lock',
      symbol: 'lock',
      destructive: false,
      confirm: null,
    })
  })

  it('carries out a verb picked from a row menu', () => {
    mount()
    const alpha = last().rows.find((r) => r.label === 'Alpha')!
    send({ type: 'rowAction', rowId: alpha.id, actionId: 'lock' })
    expect(api.setWorkspaceLock).toHaveBeenCalledWith('ws-a', true)
  })

  it('asks for a new name natively, starting from the current one', async () => {
    mount()
    const alpha = last().rows.find((r) => r.label === 'Alpha')!
    send({ type: 'rowAction', rowId: alpha.id, actionId: 'rename' })
    await waitFor(() =>
      expect(chrome.prompt).toHaveBeenCalledWith(expect.objectContaining({ value: 'Alpha' })),
    )
  })

  it('holds a removed row in the removal tray, as the desktop X does', () => {
    mount()
    const alpha = last().rows.find((r) => r.label === 'Alpha')!
    send({ type: 'rowAction', rowId: alpha.id, actionId: 'remove' })
    expect(useRemovalTrayStore.getState().entries.map((e) => e.label)).toContain('alpha')
  })

  // The page draws the countdown and Keep in place: the row stays, marked, until it drains.
  it('keeps a removed row in the list, marked as on its way out and without a menu', () => {
    mount()
    const alpha = last().rows.find((r) => r.label === 'Alpha')!
    send({ type: 'rowAction', rowId: alpha.id, actionId: 'remove' })
    const held = last().rows.find((r) => r.id === alpha.id)!
    expect(held.removal).toMatchObject({ deadlineAt: expect.any(Number) })
    expect(last().actions[alpha.id]).toEqual([])
  })

  // Focus leaves the doomed chat at once, so nothing slides into it when the delete lands.
  it('moves the window off a chat the moment it is held for removal', () => {
    mount()
    act(() =>
      windowPaneStore
        .getState()
        .paneActions.openChat('c-a', { projectId: 'p1', workspaceId: 'ws-a' }),
    )
    expect(last().focusedChatId).toBe('c-a')
    const alpha = last().rows.find((r) => r.label === 'Alpha')!
    send({ type: 'rowAction', rowId: alpha.id, actionId: 'remove' })
    expect(last().focusedChatId).not.toBe('c-a')
  })

  it('puts a row back when the page keeps it', () => {
    mount()
    const alpha = last().rows.find((r) => r.label === 'Alpha')!
    send({ type: 'rowAction', rowId: alpha.id, actionId: 'remove' })
    send({
      type: 'keepRemoval',
      entryId: last().rows.find((r) => r.id === alpha.id)!.removal!.entryId,
    })
    expect(useRemovalTrayStore.getState().entries).toEqual([])
    expect(last().rows.find((r) => r.id === alpha.id)!.removal).toBeUndefined()
  })

  it('asks for the branch name of a fork before creating it', async () => {
    mount()
    const alpha = last().rows.find((r) => r.label === 'Alpha')!
    send({ type: 'rowAction', rowId: alpha.id, actionId: 'fork' })
    await waitFor(() =>
      expect(chrome.prompt).toHaveBeenCalledWith(expect.objectContaining({ title: 'New branch' })),
    )
  })

  describe('running agents', () => {
    const run = () =>
      act(() => {
        useActiveSessionsStore.setState({
          sessions: { 'c-a': { chatId: 'c-a', workspaceId: 'ws-a', title: 'Alpha', scope: 'r1' } },
          attached: { 'c-a': { scope: 'r1', count: 2 } },
        })
      })

    it('lists a running chat in the Active section with its viewers, and offers Kill agent', () => {
      mount()
      expect(last().active).toEqual([])
      expect(last().actions['c-a'].some((a) => a.id === 'kill-agent')).toBe(false)
      run()
      expect(last().active).toEqual([{ id: 'c-a', attached: 2 }])
      const kill = last().actions['c-a'].find((a) => a.id === 'kill-agent')
      expect(kill).toMatchObject({ destructive: true })
      expect(kill?.confirm?.message).toContain('2 clients are attached')
      expect(last().actions['c-b'].some((a) => a.id === 'kill-agent')).toBe(false)
    })

    it('kills the agent the menu entry names', () => {
      mount()
      run()
      send({ type: 'rowAction', rowId: 'c-a', actionId: 'kill-agent' })
      expect(kill).toHaveBeenCalledExactlyOnceWith(
        expect.objectContaining({ chatId: 'c-a', workspaceId: 'ws-a', attached: 2 }),
      )
    })
  })

  // The page draws the drag; where a row may land is the desktop's rule, answered here.
  it('says where a dragged row may land, by the desktop’s drop rules', () => {
    mount()
    const alpha = last().rows.find((r) => r.label === 'Alpha')!
    expect(last().drop).toBeNull()
    send({ type: 'dragRow', rowId: alpha.id })
    const drop = last().drop!
    expect(drop.subjectId).toBe(alpha.id)
    expect(drop.targets[alpha.id].modes).toEqual({ before: false, after: false, into: false })
    const beta = last().rows.find((r) => r.label === 'Beta')!
    expect(drop.targets[beta.id].modes.after).toBe(true)
    send({ type: 'dragRow', rowId: null })
    expect(last().drop).toBeNull()
  })

  it('moves a dropped row through the desktop’s drop, and ends the drag', async () => {
    recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws-a' })
    mount()
    const alpha = last().rows.find((r) => r.label === 'Alpha')!
    const beta = last().rows.find((r) => r.label === 'Beta')!
    send({ type: 'dragRow', rowId: alpha.id })
    send({ type: 'moveRow', rowId: alpha.id, targetId: beta.id, position: 'after' })
    expect(last().drop).toBeNull()
    await waitFor(() =>
      expect(api.apiFetch).toHaveBeenCalledWith(
        expect.stringContaining('ws-a/placement'),
        expect.anything(),
      ),
    )
  })

  // Every drop the phone's drag can end in goes through the one drop the desktop's tree uses.
  it.each(['before', 'into'] as const)(
    'carries a chat dropped %s a chat out as a move',
    async (position) => {
      const thread = (id: string, order: number) => ({
        id,
        repoId: 'r1',
        ownsWorktree: false,
        workspaceId: 'ws-a',
        title: id,
        order,
      })
      useSidebarStore.setState({
        repos: [{ ...repo, chats: [...(repo.chats ?? []), thread('c-t1', 3), thread('c-t2', 4)] }],
      })
      mount()
      send({ type: 'dragRow', rowId: 'c-t2' })
      expect(last().drop!.targets['c-t1'].modes[position]).toBe(true)
      send({ type: 'moveRow', rowId: 'c-t2', targetId: 'c-t1', position })
      await waitFor(() => expect(api.apiFetch).toHaveBeenCalled())
    },
  )

  it('refuses a drop the desktop’s rules refuse', () => {
    mount()
    const alpha = last().rows.find((r) => r.label === 'Alpha')!
    send({ type: 'moveRow', rowId: alpha.id, targetId: alpha.id, position: 'into' })
    expect(api.apiFetch).not.toHaveBeenCalledWith(
      expect.stringContaining('placement'),
      expect.anything(),
    )
  })

  // The list page has no route to a daemon, so it is handed each icon as data, not as a URL.
  it('hands the page each repo and project icon as data', async () => {
    const png = new Blob([new Uint8Array([137, 80, 78, 71])], { type: 'image/png' })
    vi.stubGlobal(
      'fetch',
      vi.fn(async () => ({ ok: true, blob: async () => png })),
    )
    useSidebarStore.setState({
      repos: [{ ...repo, avatarURL: 'crowbar://h1/v0/projects/p1/repos/r1/icon' }],
    })
    const withIcon = [{ ...projects[0], avatarUrl: '/v0/projects/p1/icon' }]
    renderHook(() =>
      useNativeSidebar({ activeProjectId: 'p1', projects: withIcon, onSelectProject: vi.fn() }),
    )
    await waitFor(() =>
      expect(last().rows.find((r) => r.repoIcon)?.repoIcon?.avatarURL).toBe(
        'data:image/png;base64,iVBORw==',
      ),
    )
    expect(last().projects[0].avatarUrl).toBe('data:image/png;base64,iVBORw==')
    vi.unstubAllGlobals()
  })
})
