import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, within } from '@testing-library/react'

vi.mock('@/lib/persistence/sidebar-ui', () => ({
  saveSidebarUI: vi.fn().mockResolvedValue(undefined),
  loadSidebarUI: vi.fn().mockResolvedValue(null),
}))

import { useSidebarStore } from '@/lib/store/sidebar'
import { SidebarTree } from '@/components/sidebar/sidebar-tree'
import { ROOT_PANE_ID } from '@/features/panes/constants/pane'
import {
  windowPaneStore,
  resetWindowPaneStoreForTests,
} from '@/features/panes/stores/window-pane-store'
import {
  destroyWorkspaceStore,
  getOrCreateWorkspaceStore,
  getWorkspaceStore,
} from '@/features/workspace/stores/workspace-store-registry'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'

// Task 21's drag wiring — a null scrollRef and no-op commit callbacks are
// enough for every test below, none of which exercises a live drag.
const DRAG_PROPS = {
  scrollRef: { current: null } as React.RefObject<HTMLElement | null>,
  onDrop: vi.fn(),
  onPaneDrop: vi.fn(),
}

const rows: SidebarRow[] = [
  {
    id: 'folder-1',
    kind: 'folder',
    parentId: null,
    order: 0,
    label: 'Bugs',
    ownsWorktree: false,
    workspaceId: null,
    working: false,
    hasView: false,
  },
  {
    id: 'chat-1',
    kind: 'chat',
    parentId: 'folder-1',
    order: 0,
    label: 'Fix the thing',
    ownsWorktree: false,
    workspaceId: null,
    working: false,
    hasView: false,
  },
]

beforeEach(() => {
  useSidebarStore.setState({ collapsedChatRows: new Set<string>() })
  resetWindowPaneStoreForTests()
})

describe('SidebarTree', () => {
  it('renders a row per entry, nested under its parent', () => {
    render(
      <SidebarTree
        rows={rows}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    expect(screen.getByText('Bugs')).toBeInTheDocument()
    expect(screen.getByText('Fix the thing')).toBeInTheDocument()
  })

  it('folding a container hides its descendants', () => {
    render(
      <SidebarTree
        rows={rows}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    // The brief's own step-1 test reaches for `getByTestId('fold-folder-1')`,
    // which does not exist: SidebarRow (Task 5, already committed) marks its
    // fold button with `data-control="fold"` and an aria-label, never a
    // per-row testid. Same intent, found for real: the fold button scoped to
    // folder-1's own treeitem.
    const folderRow = screen.getByText('Bugs').closest('[role="treeitem"]') as HTMLElement
    fireEvent.click(within(folderRow).getByRole('button', { name: /collapse bugs/i }))
    expect(screen.queryByText('Fix the thing')).not.toBeInTheDocument()
  })

  it('indents each level by ROW_INDENT_STEP (14px, from workspace-row-base.ts)', () => {
    render(
      <SidebarTree
        rows={rows}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    const root = screen.getByText('Bugs').closest('[role="treeitem"]')?.parentElement
    const child = screen.getByText('Fix the thing').closest('[role="treeitem"]')?.parentElement
    expect(root?.getAttribute('style')).toContain('margin-inline-start: 0px')
    expect(child?.getAttribute('style')).toContain('margin-inline-start: 14px')
  })

  // User correction, live: the ghost/bootstrap row a childless folder used to
  // render underneath itself (removed here) was itself the defect, not a
  // missing worktree check on it — "it shouldn't exist" full stop. A folder's
  // own row now carries its own Fork button directly (sidebar-row.tsx), the
  // same place every other kind's already lived; there is nothing left to
  // render beneath an empty folder at all.
  it('an empty folder renders nothing beneath it — its own row carries Fork directly', () => {
    const onCreate = vi.fn()
    const repoFolder: SidebarRow = { ...rows[0], ownsWorktree: true }
    render(
      <SidebarTree
        rows={[repoFolder]}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={onCreate}
        {...DRAG_PROPS}
      />,
    )
    expect(screen.queryByTestId('affordance-dropdown')).not.toBeInTheDocument()
    expect(screen.queryByTestId('affordance-thread')).not.toBeInTheDocument()
    expect(screen.queryByTestId('affordance-workspace')).not.toBeInTheDocument()
    // Never Thread — a folder has no owning chat for one to run in, whether
    // or not it owns a worktree (`handleCreate` refuses this outright).
    expect(screen.queryByRole('button', { name: /^thread bugs$/i })).not.toBeInTheDocument()
    screen.getByRole('button', { name: /^fork bugs$/i }).click()
    expect(onCreate).toHaveBeenCalledWith('folder-1', 'workspace')
  })

  // A project-home folder (rows-from-home.ts) owns no worktree at all — no
  // repo means no worktree to fork — so it gets neither button, and still
  // renders nothing beneath it when empty.
  it('an empty folder with no worktree to fork gets no create affordance at all', () => {
    render(
      <SidebarTree
        rows={[rows[0]]}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    expect(screen.queryByRole('button', { name: /^fork bugs$/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /^thread bugs$/i })).not.toBeInTheDocument()
  })

  // Addendum §5: revises the old expectation above (a nested split-control
  // affordance row) now that every branch/chat row carries its own,
  // always-present Fork+Thread buttons directly (sidebar-row.tsx). Rendering
  // the nested AffordanceRow underneath an empty branch row too was pure
  // duplication — a second, unlabeled icon-only row that read as broken —
  // which is the exact "mystery blank row under every workspace" bug
  // reported live. A childless branch row now renders nothing beneath it;
  // its own Fork/Thread buttons are the only way in.
  it('an empty branch row does not render a nested affordance row — its own Fork/Thread buttons are the only affordance', () => {
    const branch: SidebarRow = {
      id: 'branch-1',
      kind: 'branch',
      parentId: null,
      order: 0,
      label: 'main',
      ownsWorktree: true,
      workspaceId: 'branch-1',
      working: false,
      hasView: false,
    }
    render(
      <SidebarTree
        rows={[branch]}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    expect(screen.queryByTestId('affordance-thread')).not.toBeInTheDocument()
    expect(screen.queryByTestId('affordance-workspace')).not.toBeInTheDocument()
    // Its own row buttons stand in for it instead.
    expect(screen.getByRole('button', { name: /^fork main$/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^thread main$/i })).toBeInTheDocument()
  })

  // The literal user-visible bug: a real chat row with no thread yet used to
  // grow a second, bare, unlabeled row underneath it — indistinguishable at a
  // glance from a corrupt/empty entity. A chat row's own Thread button
  // (sidebar-row.tsx) is already the way to add one.
  it('an empty chat row does not render a nested affordance row either', () => {
    const chat: SidebarRow = {
      id: 'chat-only',
      kind: 'chat',
      parentId: null,
      order: 0,
      label: 'Fix the thing',
      ownsWorktree: false,
      workspaceId: null,
      working: false,
      hasView: false,
    }
    render(
      <SidebarTree
        rows={[chat]}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    expect(screen.queryByTestId('affordance-thread')).not.toBeInTheDocument()
    expect(screen.queryByTestId('affordance-workspace')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /create new thread/i })).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: /^thread fix the thing$/i })).toBeInTheDocument()
  })

  it('renders siblings with no rule between them', () => {
    const siblings: SidebarRow[] = [
      { ...rows[0], id: 'folder-a', label: 'Folder A' },
      { ...rows[0], id: 'folder-b', label: 'Folder B' },
    ]
    const { container } = render(
      <SidebarTree
        rows={siblings}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    expect(container.querySelectorAll('hr')).toHaveLength(0)
    // Token-boundary check: `border-transparent` (ROW_INACTIVE's resting
    // border) must not false-positive as a `border-t*`/`border-b*` divider
    // utility.
    const hasDividerUtility = Array.from(container.querySelectorAll('[class]')).some((el) =>
      (el.getAttribute('class') ?? '').split(/\s+/).some((cls) => /^border-[tb](-|$)/.test(cls)),
    )
    expect(hasDividerUtility).toBe(false)
  })

  it('greys a chat row whose chat is live open in a pane, even though the row prop itself always arrives hasView: false', () => {
    // Every row in `rows` is seeded with `hasView: false` (rows-from-repo.ts
    // never seeds live state into the row object — see its own note). The
    // grey has to come from a LIVE subscription to pane membership, not from
    // the prop, so seed a pane holding chat-1's id directly on the window
    // pane store rather than passing hasView: true into `rows`.
    windowPaneStore.setState((s) => {
      s.panes[ROOT_PANE_ID] = { ...s.panes[ROOT_PANE_ID], chatId: 'chat-1' }
      return s
    })

    render(
      <SidebarTree
        rows={rows}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )

    expect(screen.getByText('Fix the thing').className).toContain('text-muted-foreground')
    // The folder row's chat never opened anywhere — no false-positive grey.
    expect(screen.getByText('Bugs').className).not.toContain('text-muted-foreground')
  })

  it('does not grey a chat row whose chat is not open in any pane', () => {
    render(
      <SidebarTree
        rows={rows}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )

    expect(screen.getByText('Fix the thing').className).not.toContain('text-muted-foreground')
  })
})

/**
 * THE LIVE SPINNER — spec §3.2's "`working` swaps the glyph for the flip-dot
 * spinner IN PLACE".
 *
 * `rows-from-repo.ts` deliberately never seeds real turn state onto a row (a
 * value seeded once latches the spinner on a chat whose turn ended minutes
 * ago), and for a long time nothing supplied it either — so no tree row ever
 * spun, on a conversation or a workspace. `SidebarTreeRow` now subscribes each
 * row the same way it already did for `hasView`.
 *
 * Driven through the REAL workspace store, not a mock of the subscription:
 * what broke was the wiring between the store and the row, and a stubbed
 * `readChatWorking` would assert nothing about it.
 */
describe('SidebarTree — live turn state', () => {
  const wsId = 'ws-live'
  const workingRows: SidebarRow[] = [
    {
      id: 'chat-live',
      kind: 'branch',
      parentId: null,
      order: 0,
      label: 'Working on it',
      ownsWorktree: true,
      workspaceId: wsId,
      // Inert, exactly as the bridge produces it — the subscription is what
      // must turn the spinner on, not this.
      working: false,
      hasView: false,
      branchName: 'feature/live',
    },
  ]

  function setWorking(value: boolean) {
    const store = getOrCreateWorkspaceStore(wsId)
    store.setState({
      agentChats: { ...store.getState().agentChats, working: { 'chat-live': value } },
    } as never)
  }

  function renderTree() {
    return render(
      <SidebarTree
        rows={workingRows}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
  }

  /** The spinner replaces the glyph, so "is it spinning" is asked of the SVG
   *  the row actually draws — `FlickerSpinner` is the only one that animates. */
  const isSpinning = (container: HTMLElement) =>
    container.querySelector('[data-flicker-spinner]') !== null

  afterEach(() => {
    destroyWorkspaceStore(wsId)
  })

  it('spins a workspace row while its chat is mid-turn', () => {
    setWorking(true)
    const { container } = renderTree()

    expect(isSpinning(container)).toBe(true)
  })

  it('does not spin when the chat is idle', () => {
    setWorking(false)
    const { container } = renderTree()

    expect(isSpinning(container)).toBe(false)
  })

  it('starts and stops spinning as the turn does, with no re-render from above', () => {
    setWorking(false)
    const { container } = renderTree()
    expect(isSpinning(container)).toBe(false)

    act(() => setWorking(true))
    expect(isSpinning(container)).toBe(true)

    act(() => setWorking(false))
    expect(isSpinning(container)).toBe(false)
  })

  /** The whole reason this could not go through `useWorkspaceStoreById`: the
   *  tree draws a row for every workspace in the repo, and minting a store per
   *  row is a documented per-session leak (`getWorkspaceStore`'s own doc). */
  it('does not create a workspace store for a row nobody has opened', () => {
    render(
      <SidebarTree
        rows={[{ ...workingRows[0], id: 'chat-cold', workspaceId: 'ws-never-opened' }]}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )

    expect(getWorkspaceStore('ws-never-opened')).toBeUndefined()
  })

  /** A row whose workspace mounts LATER must pick the spinner up — otherwise it
   *  is stuck on the `false` it read at mount for the life of the session. */
  it('binds to the workspace store when it appears after the row is drawn', () => {
    const { container } = render(
      <SidebarTree
        rows={[{ ...workingRows[0], id: 'chat-late', workspaceId: 'ws-late' }]}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    expect(isSpinning(container)).toBe(false)

    act(() => {
      const store = getOrCreateWorkspaceStore('ws-late')
      store.setState({
        agentChats: { ...store.getState().agentChats, working: { 'chat-late': true } },
      } as never)
    })

    expect(isSpinning(container)).toBe(true)
    destroyWorkspaceStore('ws-late')
  })

  /** A bubble carries a real chat identity too — §5.7's "what is up right now"
   *  is not a workspace-only question. */
  it('spins a chat bubble whose conversation is mid-turn', () => {
    const store = getOrCreateWorkspaceStore(wsId)
    store.setState({
      agentChats: { ...store.getState().agentChats, working: { 'chat-bubble': true } },
    } as never)

    const { container } = render(
      <SidebarTree
        rows={[
          {
            id: 'chat-bubble',
            kind: 'chat',
            parentId: null,
            order: 0,
            label: 'A thread',
            ownsWorktree: false,
            workspaceId: wsId,
            working: false,
            hasView: false,
          },
        ]}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )

    expect(isSpinning(container)).toBe(true)
  })
})
