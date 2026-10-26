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
  active: [],
  collapsed: [],
  actions: {},
  drop: null,
  readout: null,
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
    expect(new Set(items.map((el) => el.style.height))).toEqual(new Set(['40px']))
    expect(new Set(items.map((el) => el.style.fontSize))).toEqual(new Set(['16px']))
  })

  // The desktop's repo header carries no branch line; only a workspace under it does.
  it('draws a repository header without its branch line', () => {
    render(<MobileSidebarPage />)
    push({
      ...snapshot,
      rows: snapshot.rows.map((r) => (r.id === 'c-repo' ? { ...r, branchName: 'develop' } : r)),
    })
    expect(screen.queryByText('develop')).not.toBeInTheDocument()
    expect(screen.getByText('feature/pricing')).toBeInTheDocument()
  })

  // One scale: a repo's logo, a branch's mark and a thread's are the same size.
  it('draws every row’s mark at one size', () => {
    render(<MobileSidebarPage />)
    push(snapshot)
    const marks = screen
      .getAllByRole('treeitem')
      .map((el) => el.querySelector<HTMLElement>('span[class*="inline-flex"]')!)
    expect(new Set(marks.map((el) => el.style.width)).size).toBe(1)
  })

  it('draws the project header larger than a row, with a touch-sized target', () => {
    render(<MobileSidebarPage />)
    push(snapshot)
    const header = screen.getByTestId('sidebar-project-switcher')
    expect(parseInt(header.style.height)).toBeGreaterThanOrEqual(44)
    expect(parseInt(header.style.height)).toBeGreaterThan(
      parseInt(screen.getAllByRole('treeitem')[0].style.height),
    )
  })

  it('appends the running chats below the tree, under a divider and no label', () => {
    render(<MobileSidebarPage />)
    push({ ...snapshot, active: [{ id: 'c-thread' }] })
    const section = screen.getByRole('region', { name: 'Active' })
    expect(within(section).getByText('Ask about tax')).toBeVisible()
    expect(within(section).queryByRole('heading')).not.toBeInTheDocument()
    expect(section.firstElementChild).toHaveAttribute('role', 'separator')
    const tree = screen.getByText('checkout')
    expect(tree.compareDocumentPosition(section) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
  })

  it('draws no Active section while nothing runs', () => {
    render(<MobileSidebarPage />)
    push(snapshot)
    expect(screen.queryByRole('region', { name: 'Active' })).not.toBeInTheDocument()
  })

  it('draws the shell’s build readout on the page', () => {
    render(<MobileSidebarPage />)
    push({
      ...snapshot,
      readout: {
        lines: {
          build: 'nightly 10-26 10:38',
          net: 'net ok',
          reconnects: '',
          stats: '120fps 10ms',
          netWarn: false,
        },
        net: 'ok',
        hosts: [{ name: 'homelab', net: 'ok' }],
      },
    })
    const readout = screen.getByText('120fps 10ms').parentElement!
    expect(readout.textContent).toBe('nightly 10-26 10:38  net ok  homelab ok120fps 10ms')
    push(snapshot)
    expect(screen.queryByText('120fps 10ms')).not.toBeInTheDocument()
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

  describe('a row on its way out', () => {
    const removing = {
      ...snapshot,
      rows: snapshot.rows.map((r) =>
        r.id === 'c-thread'
          ? { ...r, removal: { entryId: 'e1', deadlineAt: Date.now() + 8000, extra: 0 } }
          : r,
      ),
    }

    it('shows its countdown and a way to keep it, which goes back to the shell', () => {
      render(<MobileSidebarPage />)
      push(removing)
      expect(document.querySelector('[data-removal-secs="e1"]')).toBeInTheDocument()
      fireEvent.click(screen.getByRole('button', { name: 'Keep Ask about tax' }))
      expect(posted).toHaveBeenCalledWith({ type: 'keepRemoval', entryId: 'e1' })
    })

    it('counts down on the page itself, which the shell’s clock cannot reach', () => {
      vi.useFakeTimers()
      render(<MobileSidebarPage />)
      push({
        ...removing,
        rows: removing.rows.map((r) =>
          r.removal ? { ...r, removal: { ...r.removal, deadlineAt: Date.now() + 5000 } } : r,
        ),
      })
      const secs = document.querySelector('[data-removal-secs="e1"]')!
      expect(secs).toHaveTextContent('5')
      act(() => vi.advanceTimersByTime(2000))
      expect(secs).toHaveTextContent('3')
      vi.useRealTimers()
    })
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
      document.elementsFromPoint = () => [target]
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

    // A finger held still is the menu's: the page asks the shell to show it, once, and drags nothing.
    it('asks the shell for the row’s menu when the finger is held still', () => {
      const rename = {
        id: 'rename',
        title: 'Rename',
        symbol: 'pencil',
        destructive: false,
        confirm: null,
      }
      render(<MobileSidebarPage />)
      push({ ...snapshot, actions: { 'c-thread': [rename] } })
      const held = rowEl('Ask about tax')
      fireEvent.touchStart(held, touch(10))
      act(() => vi.advanceTimersByTime(260))
      expect(posted).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'rowMenu' }))
      act(() => vi.advanceTimersByTime(260))
      fireEvent.touchMove(held, touch(13))
      fireEvent.touchEnd(held, { changedTouches: [{ clientX: 40, clientY: 13 }] })
      const menus = posted.mock.calls.filter(([m]) => m.type === 'rowMenu')
      expect(menus).toEqual([
        [
          {
            type: 'rowMenu',
            rowId: 'c-thread',
            rect: { x: 0, y: 0, width: 0, height: 0 },
            actions: [rename],
          },
        ],
      ])
      expect(posted).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'rowDrag' }))
      expect(posted).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'dragRow' }))
    })

    describe('an Active row', () => {
      const kill = {
        id: 'kill',
        title: 'Kill agent',
        symbol: 'xmark',
        destructive: true,
        confirm: null,
      }
      const pushActive = () => {
        render(<MobileSidebarPage />)
        push({ ...snapshot, active: [{ id: 'c-thread' }], actions: { 'c-thread': [kill] } })
        return within(screen.getByRole('region', { name: 'Active' })).getByText('Ask about tax')
      }

      it('asks for the same menu as its tree row when held still', () => {
        const held = pushActive()
        fireEvent.touchStart(held, touch(10))
        act(() => vi.advanceTimersByTime(260))
        act(() => vi.advanceTimersByTime(260))
        fireEvent.touchEnd(held, { changedTouches: [{ clientX: 40, clientY: 10 }] })
        expect(posted).toHaveBeenCalledWith(
          expect.objectContaining({ type: 'rowMenu', rowId: 'c-thread', actions: [kill] }),
        )
      })

      it('never lifts: a moving hold opens no menu and drags nothing', () => {
        const held = pushActive()
        fireEvent.touchStart(held, touch(10))
        act(() => vi.advanceTimersByTime(260))
        fireEvent.touchMove(held, touch(40))
        act(() => vi.advanceTimersByTime(600))
        expect(posted).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'rowMenu' }))
        expect(posted).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'rowDrag' }))
      })
    })

    it('opens no menu for a hold that moved into a drag', () => {
      render(<MobileSidebarPage />)
      push(snapshot)
      const held = rowEl('Ask about tax')
      fireEvent.touchStart(held, touch(10))
      act(() => vi.advanceTimersByTime(260))
      fireEvent.touchMove(held, touch(40))
      act(() => vi.advanceTimersByTime(600))
      expect(posted).toHaveBeenCalledWith({ type: 'rowDrag', active: true })
      expect(posted).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'rowMenu' }))
    })

    it('opens no menu for a tap released before the hold', () => {
      render(<MobileSidebarPage />)
      push(snapshot)
      const held = rowEl('Ask about tax')
      fireEvent.touchStart(held, touch(10))
      act(() => vi.advanceTimersByTime(100))
      fireEvent.touchEnd(held, { changedTouches: [{ clientX: 40, clientY: 10 }] })
      act(() => vi.advanceTimersByTime(600))
      expect(posted).not.toHaveBeenCalledWith(expect.objectContaining({ type: 'rowMenu' }))
    })

    // The lifted copy sits under the finger, so a hit test finds it first; the row beneath is the target.
    it('drops onto the row under the lifted copy, not onto the copy', () => {
      render(<MobileSidebarPage />)
      push(snapshot)
      const held = rowEl('Ask about tax')
      fireEvent.touchStart(held, touch(10))
      act(() => vi.advanceTimersByTime(260))
      const target = rowEl('checkout')
      vi.spyOn(target, 'getBoundingClientRect').mockReturnValue(
        DOMRect.fromRect({ x: 0, y: 100, width: 300, height: 34 }),
      )
      fireEvent.touchMove(held, touch(131))
      push({
        ...snapshot,
        drop: { subjectId: 'c-thread', targets: { 'c-repo': { modes: OPEN, band: 0.3 } } },
      })
      const lifted = document
        .querySelector('[data-drag-overlay]')!
        .querySelector<HTMLElement>('[data-sidebar-row-id]')!
      // dnd-kit positions the copy inside a wrapper of its own, which is also over the finger.
      const wrapper = document.querySelector('[data-drag-overlay]')!.parentElement!
      document.elementsFromPoint = () => [wrapper, lifted, target]
      fireEvent.touchMove(held, touch(132))
      fireEvent.touchEnd(held, { changedTouches: [{ clientX: 40, clientY: 132 }] })
      expect(posted).toHaveBeenCalledWith(
        expect.objectContaining({ type: 'moveRow', rowId: 'c-thread', targetId: 'c-repo' }),
      )
    })

    // Two chats under one workspace: the held one lands above or below its sibling.
    it.each([
      ['above', 104, 'before'],
      ['below', 136, 'after'],
    ])('reorders a chat %s its sibling', (_, finger, position) => {
      const siblings = {
        ...snapshot,
        rows: [
          ...snapshot.rows,
          row({ id: 'c-other', parentId: 'c-branch', label: 'Looking around', order: 1 }),
        ],
      }
      render(<MobileSidebarPage />)
      push(siblings)
      const held = rowEl('Looking around')
      const target = rowEl('Ask about tax')
      vi.spyOn(target, 'getBoundingClientRect').mockReturnValue(
        DOMRect.fromRect({ x: 0, y: 100, width: 300, height: 40 }),
      )
      document.elementsFromPoint = () => [target]
      fireEvent.touchStart(held, touch(300))
      act(() => vi.advanceTimersByTime(260))
      fireEvent.touchMove(held, touch(280))
      // The finger reaches the sibling before the shell has answered, then rests there.
      fireEvent.touchMove(held, touch(finger))
      push({
        ...siblings,
        drop: {
          subjectId: 'c-other',
          targets: { 'c-thread': { modes: { before: true, after: true, into: true }, band: 0.3 } },
        },
      })
      fireEvent.touchEnd(held, { changedTouches: [{ clientX: 40, clientY: finger }] })
      expect(posted).toHaveBeenCalledWith({
        type: 'moveRow',
        rowId: 'c-other',
        targetId: 'c-thread',
        position,
      })
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
