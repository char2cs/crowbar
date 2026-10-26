import { act, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { MobileSidebarPage } from '@/features/window/components/mobile-sidebar-page'
import type { NativeSidebar } from '@/lib/native-chrome'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import type { Project } from '@/lib/types'

type Shell = { webkit?: { messageHandlers: { sidebar: { postMessage: (m: unknown) => void } } } }
type Receiver = { __crowbarSidebar?: (snapshot: NativeSidebar) => void }

const posted = vi.fn()
const row = (over: Partial<SidebarRow> & { id: string; label: string }): SidebarRow => ({
  kind: 'chat',
  parentId: null,
  order: 0,
  ownsWorktree: false,
  workspaceId: 'ws-1',
  working: false,
  hasView: false,
  ...over,
})

const snapshot: NativeSidebar = {
  focusedChatId: 'c-branch',
  activeProjectId: 'p1',
  projects: [{ id: 'p1', name: 'Crowbar Seed', hostId: 'h1' } as Project],
  hosts: [{ id: 'h1', name: 'dockerlab', state: 'up' }],
  rows: [
    row({
      id: 'c-repo',
      kind: 'branch',
      label: 'checkout',
      ownsWorktree: true,
      repoIcon: {
        repoId: 'r1',
        projectId: 'p1',
        name: 'checkout',
        avatarLabel: 'C',
        avatarColor: '',
      },
    }),
    row({
      id: 'c-branch',
      kind: 'branch',
      parentId: 'c-repo',
      label: 'Fix pricing',
      branchName: 'feature/pricing',
      ownsWorktree: true,
      added: 12,
      deleted: 3,
      hasView: true,
    }),
    row({ id: 'c-thread', parentId: 'c-branch', label: 'Ask about tax' }),
  ],
  collapsed: [],
  actions: {},
  drop: null,
}

const push = (next: NativeSidebar) => act(() => (window as Receiver).__crowbarSidebar!(next))

beforeEach(() => {
  posted.mockClear()
  ;(window as Shell).webkit = { messageHandlers: { sidebar: { postMessage: posted } } }
})
afterEach(() => {
  delete (window as Shell).webkit
})

describe('MobileSidebarPage', () => {
  // One touch scale for every kind of row, so a chat, a branch and a thread read alike.
  it('draws every row at the same height and title size', () => {
    render(<MobileSidebarPage />)
    push(snapshot)
    const items = screen.getAllByRole('treeitem')
    expect(new Set(items.map((el) => el.style.height))).toEqual(new Set(['34px']))
    expect(new Set(items.map((el) => el.style.fontSize))).toEqual(new Set(['15px']))
  })

  it('asks the shell for the sidebar once it can draw it', () => {
    render(<MobileSidebarPage />)
    expect(posted).toHaveBeenCalledWith({ type: 'sidebarReady' })
  })

  it('draws the pushed project, its host and its tree with the desktop rows', () => {
    render(<MobileSidebarPage />)
    push(snapshot)
    expect(screen.getByTestId('active-project-name')).toHaveTextContent('Crowbar Seed')
    expect(screen.getByRole('button', { name: /host settings: dockerlab, online/i })).toBeVisible()
    const tree = screen.getByRole('navigation', { name: 'Chats' })
    for (const label of ['checkout', 'Fix pricing', 'Ask about tax'])
      expect(within(tree).getByText(label)).toBeInTheDocument()
    expect(within(tree).getByText('feature/pricing')).toBeInTheDocument()
  })

  it('leaves a folded row’s children out', () => {
    render(<MobileSidebarPage />)
    push({ ...snapshot, collapsed: ['c-branch'] })
    expect(screen.queryByText('Ask about tax')).not.toBeInTheDocument()
  })

  it('sends what the user picks back to the shell', () => {
    render(<MobileSidebarPage />)
    push(snapshot)
    fireEvent.click(screen.getByText('Ask about tax'))
    expect(posted).toHaveBeenCalledWith({ type: 'selectRow', id: 'c-thread' })
    fireEvent.click(screen.getByRole('button', { name: 'Collapse Fix pricing' }))
    expect(posted).toHaveBeenCalledWith({ type: 'toggleFold', id: 'c-branch' })
    fireEvent.click(screen.getByRole('button', { name: /host settings/i }))
    expect(posted).toHaveBeenCalledWith({ type: 'openSettings' })
  })

  it('folds a row only from its disclosure', () => {
    render(<MobileSidebarPage />)
    push(snapshot)
    fireEvent.click(screen.getByText('checkout'))
    expect(posted).toHaveBeenCalledWith({ type: 'selectRow', id: 'c-repo' })
    expect(posted).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'toggleFold' }))
    const disclosure = screen.getByRole('button', { name: 'Collapse checkout' })
    expect(disclosure).toHaveAttribute('aria-expanded', 'true')
    fireEvent.click(disclosure)
    expect(posted).toHaveBeenCalledWith({ type: 'toggleFold', id: 'c-repo' })
  })

  it('shows a folded row’s disclosure as closed', () => {
    render(<MobileSidebarPage />)
    push({ ...snapshot, collapsed: ['c-branch'] })
    expect(screen.getByRole('button', { name: 'Expand Fix pricing' })).toHaveAttribute(
      'aria-expanded',
      'false',
    )
  })

  // Swift's own long-press shows the menu; the page only says which row, where, and its verbs.
  it('tells the shell which row a touch lands on, with that row’s menu', () => {
    const rename = {
      id: 'rename',
      title: 'Rename',
      symbol: 'pencil',
      destructive: false,
      confirm: null,
    }
    render(<MobileSidebarPage />)
    push({ ...snapshot, actions: { 'c-thread': [rename] } })
    fireEvent.pointerDown(screen.getByText('Ask about tax'))
    expect(posted).toHaveBeenCalledWith({
      type: 'rowPress',
      rowId: 'c-thread',
      rect: { x: 0, y: 0, width: 0, height: 0 },
      actions: [rename],
    })
  })

  // Icons arrive as data: this page cannot reach the daemon their URLs point at.
  it('draws the project and repo icons it is handed', () => {
    const icon = 'data:image/png;base64,iVBORw=='
    render(<MobileSidebarPage />)
    push({
      ...snapshot,
      projects: [{ ...snapshot.projects[0], avatarUrl: icon }],
      rows: snapshot.rows.map((r) =>
        r.repoIcon ? { ...r, repoIcon: { ...r.repoIcon, avatarURL: icon } } : r,
      ),
    })
    const images = document.querySelectorAll(`img[src="${icon}"]`)
    expect(images.length).toBe(2)
  })

  describe('dragging a row', () => {
    const OPEN = { before: true, after: true, into: true }
    const SHUT = { before: false, after: false, into: false }
    // The tree's row, not the lifted copy drawn while it is dragged.
    const rowEl = (label: string) =>
      screen.getAllByText(label)[0].closest<HTMLElement>('[role="treeitem"]')!
    const touch = (y: number) => ({ touches: [{ clientX: 40, clientY: y }] })

    // Hold, then move: the main webview answers where the row may land, then carries out the drop.
    const dragOnto = (target: HTMLElement, y: number, targets: Record<string, typeof OPEN>) => {
      const held = rowEl('Ask about tax')
      fireEvent.touchStart(held, touch(10))
      act(() => vi.advanceTimersByTime(260))
      vi.spyOn(target, 'getBoundingClientRect').mockReturnValue(
        DOMRect.fromRect({ x: 0, y: 100, width: 300, height: 34 }),
      )
      document.elementFromPoint = () => target
      fireEvent.touchMove(held, touch(y))
      expect(posted).toHaveBeenCalledWith({ type: 'rowDrag', active: true })
      expect(posted).toHaveBeenCalledWith({ type: 'dragRow', rowId: 'c-thread' })
      push({
        ...snapshot,
        drop: {
          subjectId: 'c-thread',
          targets: Object.fromEntries(
            Object.entries(targets).map(([id, modes]) => [id, { modes, band: 0.3 }]),
          ),
        },
      })
      fireEvent.touchMove(held, touch(y + 1))
      fireEvent.touchEnd(held, { changedTouches: [{ clientX: 40, clientY: y + 1 }] })
      expect(posted).toHaveBeenCalledWith({ type: 'rowDrag', active: false })
    }

    beforeEach(() => vi.useFakeTimers())
    afterEach(() => vi.useRealTimers())

    it('moves a held row to where it is dropped', () => {
      render(<MobileSidebarPage />)
      push(snapshot)
      dragOnto(rowEl('checkout'), 131, { 'c-repo': OPEN })
      expect(posted).toHaveBeenCalledWith({
        type: 'moveRow',
        rowId: 'c-thread',
        targetId: 'c-repo',
        position: 'after',
      })
    })

    // A finger held still is the native long-press menu's, so the page never claims it.
    it('leaves a held, unmoved row to the shell’s menu', () => {
      render(<MobileSidebarPage />)
      push(snapshot)
      const held = rowEl('Ask about tax')
      fireEvent.touchStart(held, touch(10))
      act(() => vi.advanceTimersByTime(600))
      fireEvent.touchMove(held, touch(13))
      fireEvent.touchEnd(held, { changedTouches: [{ clientX: 40, clientY: 13 }] })
      expect(posted).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'rowDrag' }))
      expect(posted).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'dragRow' }))
    })

    it('moves nothing where the drop rules refuse it', () => {
      render(<MobileSidebarPage />)
      push(snapshot)
      dragOnto(rowEl('checkout'), 131, { 'c-repo': SHUT })
      expect(posted).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'moveRow' }))
      expect(posted).toHaveBeenCalledWith({ type: 'dragRow', rowId: null })
    })
  })
})
