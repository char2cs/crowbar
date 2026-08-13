/**
 * Dragging a row onto the editor pane.
 *
 * The guard is the thing worth pinning. The pane is a very large target
 * immediately beside the sidebar, so a reorder that travels the length of a
 * long list crosses it on the way — and a pane that removed on release would
 * turn that transit into a delete. Nothing may be armed, and nothing may be
 * drawn, until the pointer has stayed a beat.
 *
 * jsdom has no `elementsFromPoint` and no layout, so both are stubbed (the same
 * technique as workspace-tree-drag.test.tsx). Pointer events ride `MouseEvent`s
 * with pointer type names, which React and the window listeners duck-type.
 */
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, act } from '@testing-library/react'

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
  createFolder: vi.fn(() => Promise.resolve({ id: 'new-folder' })),
  deleteFolder: vi.fn(() => Promise.resolve()),
}))

vi.mock('@/lib/api/workspace', () => ({
  reparentWorkspace: vi.fn(() => Promise.resolve()),
}))

import { idle, success } from '@/lib/loadable'
import { useWorkspaceListStore } from '@/lib/store/workspace-list'
import { useProjectDataStore } from '@/lib/store/projects'
import { useHomeWorkspaceStore } from '@/lib/store/home-workspace'
import { useSidebarStore, type Repo } from '@/lib/store/sidebar'
import { getInitialRemovalState, useRemovalTrayStore } from '@/lib/store/sidebar-removal'
import { WorkspaceTree } from '@/components/layout/workspace-tree'
import { EditorRemovalOverlay, PANE_ARM_MS } from '@/components/layout/editor-removal-overlay'
import type { Project } from '@/lib/types'

const ROW_HEIGHT = 36
/** Anything at or beyond this x is the editor pane; the sidebar is to its left. */
const PANE_X = 400

const project: Project = {
  id: 'p1',
  name: 'crowbar-project',
  path: '/p1',
  lastActivity: new Date(0),
}

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
  ],
  ...over,
})

function Shell() {
  return (
    <>
      <WorkspaceTree />
      {/* Stands in for the content pane of the IDE shell, which is what carries
          both the drop attribute and the veil in the real app. */}
      <div data-pane-drop="">
        <EditorRemovalOverlay />
      </div>
    </>
  )
}

/**
 * Rows stacked in render order, and a hit test that answers the pane for
 * anything to the right of the sidebar.
 */
function stubHitTest(): Map<string, HTMLElement> {
  const rows = new Map<string, HTMLElement>()
  document.querySelectorAll<HTMLElement>('[data-ws-drop], [data-project-drop]').forEach((el, i) => {
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
    const id = el.getAttribute('data-ws-drop') ?? el.getAttribute('data-project-drop')!
    rows.set(id, el)
  })
  const pane = document.querySelector<HTMLElement>('[data-pane-drop]')!
  document.elementsFromPoint = ((x: number, y: number) => {
    if (x >= PANE_X) return [pane]
    for (const el of rows.values()) {
      const r = el.getBoundingClientRect()
      if (y >= r.top && y < r.bottom) return [el]
    }
    return []
  }) as typeof document.elementsFromPoint
  return rows
}

const overlay = () => document.querySelector<HTMLElement>('[data-pane-removal]')!
/** The zone is DRAWN — up for the whole drag, in either of its two states. */
const veilUp = () => !overlay().hidden
/** A release right now would remove — the state the dwell unlocks. */
const veilArmed = () => overlay().hasAttribute('data-armed')
const trayIds = () => useRemovalTrayStore.getState().entries.map((e) => e.id)

function pointerDown(el: HTMLElement, y: number): void {
  el.dispatchEvent(
    new MouseEvent('pointerdown', { button: 0, clientX: 0, clientY: y, bubbles: true }),
  )
}
function pointerMove(x: number, y: number): void {
  act(() => {
    window.dispatchEvent(new MouseEvent('pointermove', { clientX: x, clientY: y, bubbles: true }))
  })
}
function pointerUp(x: number, y: number): void {
  act(() => {
    window.dispatchEvent(new MouseEvent('pointerup', { clientX: x, clientY: y, bubbles: true }))
  })
}

/** Grab `id`'s row and take it over the pane, dwelling `ms` before letting go. */
function dragToPane(id: string, ms: number, selector = 'data-ws-drop'): void {
  const el = document.querySelector<HTMLElement>(`[${selector}="${id}"]`)!
  const start = el.getBoundingClientRect().top + ROW_HEIGHT / 2
  pointerDown(el, start)
  pointerMove(0, start + 20)
  pointerMove(PANE_X, 100)
  act(() => {
    vi.advanceTimersByTime(ms)
  })
  pointerUp(PANE_X, 100)
}

beforeEach(() => {
  vi.clearAllMocks()
  // NOT `shouldAdvanceTime`: the dwell is measured in milliseconds, and a fake
  // clock that also creeps with the real one turns "one tick short of the dwell"
  // into a race with however long the test itself took to get there.
  vi.useFakeTimers()
  HTMLElement.prototype.setPointerCapture = vi.fn()
  useWorkspaceListStore.setState({ data: idle() })
  useHomeWorkspaceStore.setState({ workspace: null })
  useProjectDataStore.setState({ data: success([project]) })
  useRemovalTrayStore.setState(getInitialRemovalState())
  useSidebarStore.setState({
    repos: [repo()],
    collapsedRepos: new Set<string>(),
    collapsedWorkspaces: new Set<string>(),
    collapsedProjects: new Set<string>(),
  })
})

afterEach(() => {
  vi.useRealTimers()
})

describe('the arming guard', () => {
  it('arms nothing on a drag that merely crosses the pane', () => {
    render(<Shell />)
    stubHitTest()

    dragToPane('a', PANE_ARM_MS - 1)

    expect(trayIds()).toEqual([])
    expect(overlay().hidden).toBe(true)
  })

  it('leaves the veil UNARMED for as long as the dwell is still running', () => {
    render(<Shell />)
    stubHitTest()
    const el = document.querySelector<HTMLElement>('[data-ws-drop="a"]')!

    pointerDown(el, ROW_HEIGHT / 2)
    pointerMove(0, ROW_HEIGHT / 2 + 20)
    pointerMove(PANE_X, 100)
    act(() => {
      vi.advanceTimersByTime(PANE_ARM_MS - 1)
    })

    // Drawn, but not armed: the zone is discoverable from the first frame of the
    // drag, and only the dwell makes a release act on it.
    expect(veilUp()).toBe(true)
    expect(veilArmed()).toBe(false)
    expect(overlay().textContent).toContain('Drop here to remove alpha')

    act(() => {
      vi.advanceTimersByTime(1)
    })
    expect(veilArmed()).toBe(true)
    pointerUp(PANE_X, 100)
  })

  it('disarms — but stays drawn — the moment the pointer leaves the pane', () => {
    render(<Shell />)
    stubHitTest()
    const el = document.querySelector<HTMLElement>('[data-ws-drop="a"]')!

    pointerDown(el, ROW_HEIGHT / 2)
    pointerMove(0, ROW_HEIGHT / 2 + 20)
    pointerMove(PANE_X, 100)
    act(() => {
      vi.advanceTimersByTime(500)
    })
    expect(veilArmed()).toBe(true)

    pointerMove(0, ROW_HEIGHT * 1.5) // back over the sidebar
    // Back to available, NOT away: the row is still in the air and the pane is
    // still where it would go.
    expect(veilUp()).toBe(true)
    expect(veilArmed()).toBe(false)

    pointerUp(0, ROW_HEIGHT * 1.5)
    expect(trayIds()).toEqual([])
    // And once the drag is over the zone goes. This used to be a side effect of
    // leaving the pane, which stopped happening when leaving began painting the
    // available state instead — leaving the veil drawn over the editor after
    // every drag that ended anywhere but the pane.
    expect(veilUp()).toBe(false)
    expect(veilArmed()).toBe(false)
  })
})

describe('what the armed pane takes', () => {
  it('holds the row once the dwell has elapsed', () => {
    render(<Shell />)
    stubHitTest()

    dragToPane('a', 500)

    expect(trayIds()).toEqual(['a'])
    // Held, not deleted: the row is off screen and the store still has it.
    expect(document.querySelector('[data-ws-drop="a"]')).toBeNull()
    expect(useSidebarStore.getState().repos[0].workspaces.map((w) => w.id)).toEqual(['a', 'b'])
  })

  it('names what will go while it waits', () => {
    render(<Shell />)
    stubHitTest()
    const el = document.querySelector<HTMLElement>('[data-ws-drop="a"]')!

    pointerDown(el, ROW_HEIGHT / 2)
    pointerMove(0, ROW_HEIGHT / 2 + 20)
    pointerMove(PANE_X, 100)
    act(() => {
      vi.advanceTimersByTime(500)
    })

    expect(overlay().textContent).toContain('Release to remove alpha')
    expect(overlay().textContent).toContain('You will have 8 seconds to undo')
    pointerUp(PANE_X, 100)
  })

  // A project IS removable this way now — it goes to the tray like a repo, with
  // no clock and a confirmation. It used to be the one row the planner refused,
  // so the pane offered it nothing at all.
  it('holds a project too, and says so before it takes it', () => {
    render(<Shell />)
    stubHitTest()
    const el = document.querySelector<HTMLElement>('[data-project-drop="p1"]')!

    pointerDown(el, ROW_HEIGHT / 2)
    pointerMove(0, ROW_HEIGHT / 2 + 20)
    pointerMove(PANE_X, 100)
    act(() => {
      vi.advanceTimersByTime(500)
    })

    expect(overlay().textContent).toContain('Release to remove')
    // Never "8 seconds to undo": a project waits on a confirmation instead.
    expect(overlay().textContent).toContain('confirm')

    pointerUp(PANE_X, 100)
    expect(trayIds()).toEqual(['p1'])
  })

  it('draws no zone at all for a row that cannot be removed', () => {
    // A locked branch: the daemon refuses the delete, so the pane must not offer
    // it. The veil is a promise about a release — one that would be refused is
    // worse than no affordance.
    useSidebarStore.setState({
      repos: [
        repo({
          workspaces: [
            { id: 'a', branch: 'alpha', status: 'locked', age: '', order: 0 },
            { id: 'b', branch: 'beta', status: 'new', age: '', order: 1 },
          ],
        }),
      ],
    })
    render(<Shell />)
    stubHitTest()
    const el = document.querySelector<HTMLElement>('[data-ws-drop="a"]')!

    pointerDown(el, ROW_HEIGHT / 2)
    pointerMove(0, ROW_HEIGHT / 2 + 20)
    expect(veilUp()).toBe(false)

    pointerMove(PANE_X, 100)
    act(() => {
      vi.advanceTimersByTime(1000)
    })
    expect(veilUp()).toBe(false)

    pointerUp(PANE_X, 100)
    expect(trayIds()).toEqual([])
  })

  it('takes the whole multiselection when the grabbed row is part of it', () => {
    render(<Shell />)
    stubHitTest()
    for (const id of ['a', 'b']) {
      document.querySelector(`[data-ws-drop="${id}"]`)!.setAttribute('aria-selected', 'true')
    }

    dragToPane('b', 500)

    expect(trayIds().sort()).toEqual(['a', 'b'])
  })
})
