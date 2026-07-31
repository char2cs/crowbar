/**
 * The drag itself, end to end through the real provider.
 *
 * What this covers that the pure modules cannot: the pointer threshold, the
 * ghost being a CLONE of the grabbed row, the optimistic write landing before
 * the request and snapping back as a unit when it is refused, and the re-render
 * split that keeps a moving ghost off the rows.
 *
 * jsdom has neither `setPointerCapture` nor `elementsFromPoint` nor layout, so
 * all three are stubbed. Pointer events are dispatched as `MouseEvent`s with
 * pointer type names — React and the window listeners duck-type them by name
 * (same technique as pane-sash.test.tsx).
 */
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, act } from '@testing-library/react'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useMatch: () => null,
}))

vi.mock('@/lib/api/sidebar-placement', () => ({
  placeWorkspace: vi.fn(() => Promise.resolve()),
  placeFolder: vi.fn(() => Promise.resolve()),
  placeRepo: vi.fn(() => Promise.resolve()),
  placeProject: vi.fn(() => Promise.resolve()),
  createFolder: vi.fn(() => Promise.resolve()),
  deleteFolder: vi.fn(() => Promise.resolve()),
}))

vi.mock('@/lib/api/workspace', () => ({
  reparentWorkspace: vi.fn(() => Promise.resolve()),
}))

import { placeWorkspace } from '@/lib/api/sidebar-placement'
import { reparentWorkspace } from '@/lib/api/workspace'
import { idle, success } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { WorkspaceTree } from '@/components/layout/workspace-tree'
import {
  WorkspaceTreeProvider,
  useWorkspaceTreeActions,
} from '@/components/layout/workspace-tree-context'
import type { Project } from '@/lib/types'

const ROW_HEIGHT = 36

const project = (id: string, name: string): Project => ({
  id,
  name,
  path: `/${id}`,
  lastActivity: new Date(0),
})

const repo = (over: Partial<Repo> = {}): Repo => ({
  id: 'r1',
  projectId: 'p1',
  name: 'crowbar',
  avatarLabel: 'C',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'w-default',
  workspaces: [
    { id: 'a', branch: 'alpha', status: 'new', age: '', order: 0 },
    { id: 'b', branch: 'beta', status: 'new', age: '', order: 1 },
    { id: 'c', branch: 'gamma', status: 'new', age: '', order: 2 },
  ],
  ...over,
})

/** Give every workspace row a rect, stacked in render order. */
function stubRowRects(): Map<string, HTMLElement> {
  const rows = new Map<string, HTMLElement>()
  document.querySelectorAll<HTMLElement>('[data-ws-drop]').forEach((el, i) => {
    const top = i * ROW_HEIGHT
    el.getBoundingClientRect = () =>
      ({
        top,
        bottom: top + ROW_HEIGHT,
        height: ROW_HEIGHT,
        left: 0,
        right: 200,
        width: 200,
      }) as DOMRect
    rows.set(el.getAttribute('data-ws-drop')!, el)
  })
  return rows
}

/** Point the stubbed hit test at whichever row covers `y`. */
function hitTestByY(rows: Map<string, HTMLElement>): void {
  document.elementsFromPoint = ((_x: number, y: number) => {
    for (const el of rows.values()) {
      const r = el.getBoundingClientRect()
      if (y >= r.top && y < r.bottom) return [el]
    }
    return []
  }) as typeof document.elementsFromPoint
}

const rowFor = (branch: string) => screen.getByText(branch).closest('[data-ws-drop]') as HTMLElement

/** The one hairline the whole drag shares, portalled to the body. */
const dropLine = () => document.querySelector('[data-drop-indicator]') as HTMLElement

function pointerDown(el: HTMLElement, y: number): void {
  el.dispatchEvent(
    new MouseEvent('pointerdown', { button: 0, clientX: 0, clientY: y, bubbles: true }),
  )
}
function pointerMove(y: number, x = 0): void {
  act(() => {
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: x, clientY: y, bubbles: true }))
  })
}
function pointerUp(y: number): void {
  act(() => {
    window.dispatchEvent(new MouseEvent('pointerup', { clientX: 0, clientY: y, bubbles: true }))
  })
}

/** Drag `branch`'s row to `y` and let go. */
function dragTo(branch: string, y: number): void {
  const el = rowFor(branch)
  const start = el.getBoundingClientRect().top + ROW_HEIGHT / 2
  pointerDown(el, start)
  pointerMove(start + 20) // past the 5px threshold
  pointerMove(y)
  pointerUp(y)
}

const branchOrder = () =>
  Array.from(document.querySelectorAll('[data-ws-drop]')).map((el) =>
    el.getAttribute('data-ws-drop'),
  )

const workspaceById = (id: string) =>
  useSidebarStore.getState().repos[0].workspaces.find((w) => w.id === id)!

beforeEach(() => {
  vi.clearAllMocks()
  HTMLElement.prototype.setPointerCapture = vi.fn()
  useWorkspaceListStore.setState({ data: idle() })
  useHomeWorkspaceStore.setState({ workspace: null })
  useProjectDataStore.setState({ data: success([project('p1', 'workspaces')]) })
  useSidebarStore.setState({
    repos: [repo()],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
})

describe('the threshold', () => {
  it('does not start a drag from a press that never moves', () => {
    render(<WorkspaceTree />)
    stubRowRects()

    pointerDown(rowFor('alpha'), 10)
    pointerUp(10)

    expect(document.querySelector('[data-drag-ghost]')).toBeNull()
  })

  it('does not start one inside 5px, which is what keeps dblclick-to-rename alive', () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    pointerDown(rowFor('alpha'), 10)
    pointerMove(13)

    expect(document.querySelector('[data-drag-ghost]')).toBeNull()
    expect(HTMLElement.prototype.setPointerCapture).not.toHaveBeenCalled()
  })
})

describe('the ghost', () => {
  it('is a clone of the grabbed row, not a chip', () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    pointerDown(rowFor('alpha'), 10)
    pointerMove(30)

    const ghost = document.querySelector('[data-drag-ghost]')!
    expect(ghost.querySelector('[data-ws-drop="a"]')).not.toBeNull()
    expect(ghost.textContent).toContain('alpha')
  })

  it('carries no count badge for a single row', () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    pointerDown(rowFor('alpha'), 10)
    pointerMove(30)

    expect(document.querySelector('[data-drag-ghost-rows]')).toHaveAttribute(
      'data-drag-ghost-rows',
      '1',
    )
  })

  it('follows the pointer through the DOM, never through React', () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    pointerDown(rowFor('alpha'), 10)
    pointerMove(30)
    const ghost = document.querySelector<HTMLElement>('[data-drag-ghost]')!
    pointerMove(77, 40)

    expect(ghost.style.top).toBe('67px')
    expect(ghost.style.left).toBe('52px')
  })

  it('goes away when the drag ends', () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    pointerDown(rowFor('alpha'), 10)
    pointerMove(30)
    pointerUp(30)

    expect(document.querySelector('[data-drag-ghost]')).toBeNull()
  })
})

describe('the indicator says where the drop lands', () => {
  it("draws a hairline at the target row's own indent when the drop reorders", () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    pointerDown(rowFor('gamma'), ROW_HEIGHT * 2.5)
    pointerMove(ROW_HEIGHT * 2.5 + 20)
    pointerMove(ROW_HEIGHT * 0.1) // the top band of row `a`

    const line = dropLine()
    expect(line.style.display).toBe('block')
    expect(line.dataset.dropIndicator).toBe('before')
    // Inset into the row, and centred on the boundary it marks: row `a` starts
    // at 0 and spans 200px.
    expect(line.style.left).toBe('8px')
    expect(line.style.width).toBe('190px')
    expect(line.style.top).toBe('-1px')
    expect(rowFor('alpha').className).not.toContain('bg-sidebar-drop-nest')
  })

  // One element for the whole drag. Drawn inside each row it would mount and
  // unmount per crossing, which is a mark that blinks from slot to slot where
  // it should travel between them.
  it('MOVES between slots rather than being redrawn in each row', () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    pointerDown(rowFor('gamma'), ROW_HEIGHT * 2.5)
    pointerMove(ROW_HEIGHT * 2.5 + 20)
    pointerMove(ROW_HEIGHT * 0.1) // before `alpha`
    const first = dropLine()

    pointerMove(ROW_HEIGHT * 1.1) // before `beta`

    expect(dropLine()).toBe(first)
    expect(first.style.top).toBe(`${ROW_HEIGHT - 1}px`)
    expect(document.querySelectorAll('[data-drop-indicator]')).toHaveLength(1)
  })

  it('fills the target row when the drop nests, and draws no line', () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    pointerDown(rowFor('gamma'), ROW_HEIGHT * 2.5)
    pointerMove(ROW_HEIGHT * 2.5 + 20)
    pointerMove(ROW_HEIGHT * 0.5) // the middle of row `a`

    expect(rowFor('alpha').className).toContain('bg-sidebar-drop-nest')
    expect(dropLine().style.display).toBe('none')
  })

  // A protected branch reorders among its siblings and nothing else. The gap
  // under an EXPANDED sibling is that sibling's first-child slot, so it is a
  // nest wearing a line's clothes — and it is refused outright.
  it('draws nothing at all where the drop is refused', () => {
    useSidebarStore.setState({
      repos: [
        repo({
          workspaces: [
            { id: 'a', branch: 'alpha', status: 'new', age: '', order: 0 },
            { id: 'kid', branch: 'delta', parentId: 'a', status: 'new', age: '', order: 0 },
            { id: 'b', branch: 'beta', status: 'locked', age: '', order: 1 },
          ],
        }),
      ],
    })
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    pointerDown(rowFor('beta'), ROW_HEIGHT * 2.5)
    pointerMove(ROW_HEIGHT * 2.5 + 20)
    pointerMove(ROW_HEIGHT * 0.9) // the gap under the expanded `alpha`

    expect(dropLine().style.display).toBe('none')
    expect(rowFor('alpha').className).not.toContain('bg-sidebar-drop-nest')

    // Its own half of the row still reorders it — the refusal is specific.
    pointerMove(ROW_HEIGHT * 0.2)
    expect(dropLine().style.display).toBe('block')
    expect(dropLine().dataset.dropIndicator).toBe('before')
  })

  it('goes away with the drag', () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    pointerDown(rowFor('gamma'), ROW_HEIGHT * 2.5)
    pointerMove(ROW_HEIGHT * 2.5 + 20)
    pointerMove(ROW_HEIGHT * 0.1)
    pointerUp(ROW_HEIGHT * 0.1)

    expect(document.querySelector('[data-drop-indicator]')).toBeNull()
  })
})

describe('committing the drop', () => {
  it("writes the new order before the request, and sends the daemon's index", () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    dragTo('gamma', ROW_HEIGHT * 0.1) // before `alpha`

    expect(branchOrder()).toEqual(['c', 'a', 'b'])
    expect(placeWorkspace).toHaveBeenCalledWith('p1', 'r1', 'c', { folderId: '', order: 0 })
  })

  it('re-parents the fork when the drop nests, by the other endpoint', () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    dragTo('gamma', ROW_HEIGHT * 0.5) // the middle of `alpha`

    expect(reparentWorkspace).toHaveBeenCalledWith('p1', 'r1', 'c', 'a')
  })

  // The re-parent is 202 + an async rebase, so the slot cannot go out with it:
  // it would be an index into a level the row has not joined, and the daemon
  // would append the row instead — the right depth, the wrong place.
  it('holds the promised slot back until the re-parent has actually landed', async () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    dragTo('gamma', ROW_HEIGHT * 0.5) // the middle of `alpha`

    await Promise.resolve()
    expect(placeWorkspace).not.toHaveBeenCalled()

    // The daemon reports the move: a frame about `c` carrying its new parent.
    act(() => {
      useSidebarStore.setState((s) => ({
        repos: s.repos.map((r) => ({
          ...r,
          workspaces: r.workspaces.map((w) => (w.id === 'c' ? { ...w, parentId: 'a' } : w)),
        })),
      }))
    })

    await vi.waitFor(() =>
      expect(placeWorkspace).toHaveBeenCalledWith('p1', 'r1', 'c', { order: 0 }),
    )
  })

  it('snaps the move back when the re-parent reports a failure', async () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    dragTo('gamma', ROW_HEIGHT * 0.5)
    expect(workspaceById('c').parentId).toBe('a')
    expect(workspaceById('c').order).toBe(0)

    await Promise.resolve()
    // A refused rebase leaves the workspace where it was and reports why.
    act(() => {
      useSidebarStore.setState((s) => ({
        repos: s.repos.map((r) => ({
          ...r,
          workspaces: r.workspaces.map((w) =>
            w.id === 'c' ? { ...w, parentId: undefined, lastError: 'rebase: could not apply' } : w,
          ),
        })),
      }))
    })

    // The whole optimistic placement comes back, index included.
    await vi.waitFor(() => expect(workspaceById('c').order).toBe(2))
    expect(workspaceById('c').parentId ?? '').toBe('')
    expect(placeWorkspace).not.toHaveBeenCalled()
  })

  it('is a no-op where the drop was refused', () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    dragTo('alpha', ROW_HEIGHT * 0.5) // onto itself

    expect(placeWorkspace).not.toHaveBeenCalled()
    expect(reparentWorkspace).not.toHaveBeenCalled()
    expect(branchOrder()).toEqual(['a', 'b', 'c'])
  })

  it('snaps the whole level back when the request is refused', async () => {
    vi.mocked(placeWorkspace).mockRejectedValueOnce(new Error('409 conflict'))
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)

    dragTo('gamma', ROW_HEIGHT * 0.1)
    expect(branchOrder()).toEqual(['c', 'a', 'b'])

    // Every displaced sibling comes back too, not just the row that moved.
    await vi.waitFor(() => expect(branchOrder()).toEqual(['a', 'b', 'c']))
    expect(workspaceById('a').order).toBe(0)
    expect(workspaceById('b').order).toBe(1)
    expect(workspaceById('c').order).toBe(2)
  })
})

describe('several rows at once', () => {
  /**
   * The multiselection is the next wave's; the drag reads it off the tree,
   * from the same `aria-selected` assistive tech reads, so marking the rows is
   * all it takes to exercise the whole path today.
   */
  const select = (...branches: string[]) => {
    for (const branch of branches) rowFor(branch).setAttribute('aria-selected', 'true')
  }

  it('carries the selection when the grabbed row is part of it', () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)
    select('beta', 'gamma')

    pointerDown(rowFor('gamma'), ROW_HEIGHT * 2.5)
    pointerMove(ROW_HEIGHT * 2.5 + 20)

    expect(document.querySelector('[data-drag-ghost-rows]')).toHaveAttribute(
      'data-drag-ghost-rows',
      '2',
    )
    pointerUp(ROW_HEIGHT * 2.5 + 20)
  })

  // One request after the other, never together: each `order` is an index into
  // the list as it stands once the previous has landed.
  it('moves both, in order, on one drop', async () => {
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)
    select('beta', 'gamma')

    dragTo('gamma', ROW_HEIGHT * 0.1) // before `alpha`

    expect(branchOrder()).toEqual(['b', 'c', 'a'])
    expect(placeWorkspace).toHaveBeenNthCalledWith(1, 'p1', 'r1', 'b', { folderId: '', order: 0 })
    await vi.waitFor(() =>
      expect(placeWorkspace).toHaveBeenNthCalledWith(2, 'p1', 'r1', 'c', {
        folderId: '',
        order: 1,
      }),
    )
  })

  it('snaps the whole move back as a unit when one request is refused', async () => {
    vi.mocked(placeWorkspace).mockRejectedValueOnce(new Error('409 conflict'))
    render(<WorkspaceTree />)
    const rows = stubRowRects()
    hitTestByY(rows)
    select('beta', 'gamma')

    dragTo('gamma', ROW_HEIGHT * 0.1)
    expect(branchOrder()).toEqual(['b', 'c', 'a'])

    await vi.waitFor(() => expect(branchOrder()).toEqual(['a', 'b', 'c']))
    expect(workspaceById('a').order).toBe(0)
    expect(workspaceById('b').order).toBe(1)
    expect(workspaceById('c').order).toBe(2)
  })
})

describe('the re-render split', () => {
  // Counting renders means counting something render itself does. A module
  // counter rather than a prop callback: React may replay or discard render
  // work, and a callback passed IN would be the thing being replayed.
  const renders = { count: 0 }

  /** A subscriber that reads ONLY the actions half of the tree context. */
  function ActionsOnly() {
    useWorkspaceTreeActions()
    renders.count += 1
    return null
  }

  it('does not re-render an actions-only subscriber as the ghost moves', () => {
    renders.count = 0
    render(
      <WorkspaceTreeProvider>
        <ActionsOnly />
        <div
          data-ws-drop="a"
          data-drop-repo="r1"
          data-drop-parent=""
          ref={(el) => {
            if (el)
              el.getBoundingClientRect = () =>
                ({ top: 0, bottom: 36, height: 36, left: 0, right: 200, width: 200 }) as DOMRect
          }}
        >
          alpha
        </div>
      </WorkspaceTreeProvider>,
    )
    document.elementsFromPoint = (() => []) as typeof document.elementsFromPoint

    const row = document.querySelector<HTMLElement>('[data-ws-drop="a"]')!
    pointerDown(row, 10)
    pointerMove(40) // starts the drag
    const afterStart = renders.count

    // Twenty pixels of pure ghost movement, no boundary crossed.
    for (let y = 41; y < 61; y++) pointerMove(y)

    expect(renders.count).toBe(afterStart)
  })
})
