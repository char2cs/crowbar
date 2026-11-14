import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, fireEvent, render, screen, within } from '@testing-library/react'

vi.mock('@/lib/persistence/sidebar-ui', () => ({
  saveSidebarUI: vi.fn().mockResolvedValue(undefined),
  loadSidebarUI: vi.fn().mockResolvedValue(null),
}))

import { SidebarTree } from '@/components/sidebar/sidebar-tree'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import { getWorkspaceStore } from '@/features/workspace/stores/workspace-store-registry'
import { resetChatStore, setChatWorking } from '@/__tests__/__fixtures__/agent-chat'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { treeUiStore } from '@/features/workspace/tree/tree-ui-store'

// A null scrollRef and no-op commit callbacks suffice: no test below runs a live drag.
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
  treeUiStore.setState({ collapsed: new Set<string>() })
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
    // SidebarRow marks its fold button with `data-control="fold"` and an aria-label, not a
    // per-row testid, so find it within folder-1's treeitem.
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

  // A folder's own row carries Thread and Fork; nothing renders beneath an empty one.
  it('an empty folder renders nothing beneath it — its own row carries Thread and Fork directly', () => {
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
    // A folder applies the same logic as its parent: under a real repo it
    // gets BOTH Thread and Fork on its own row, same as a `branch` row does.
    screen.getByRole('button', { name: /^thread bugs$/i }).click()
    expect(onCreate).toHaveBeenCalledWith('folder-1', 'thread')
    screen.getByRole('button', { name: /^fork bugs$/i }).click()
    expect(onCreate).toHaveBeenCalledWith('folder-1', 'workspace')
  })

  // A project-home folder (deriveRows) owns no worktree at all — no
  // repo means no worktree to fork — so it gets no Fork button, and still
  // renders nothing beneath it when empty. It DOES still get Thread: project
  // home applies the same logic to a folder it applies to a chat, and a home
  // chat has always gotten Thread with no repo required.
  it('an empty folder with no worktree to fork gets Thread but no Fork', () => {
    const onCreate = vi.fn()
    render(
      <SidebarTree
        rows={[rows[0]]}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={onCreate}
        {...DRAG_PROPS}
      />,
    )
    expect(screen.queryByRole('button', { name: /^fork bugs$/i })).not.toBeInTheDocument()
    screen.getByRole('button', { name: /^thread bugs$/i }).click()
    expect(onCreate).toHaveBeenCalledWith('folder-1', 'thread')
  })

  // Explicit product request: the fold chevron must not render at all on a
  // row with no children — toggling it already had zero visible effect (there
  // was nothing to fold), so leaving the button there was an affordance for
  // nothing. `onToggleFold` is passed only when `hasChildren` is true.
  it('a childless row renders no fold/chevron control at all', () => {
    render(
      <SidebarTree
        rows={[rows[0]]}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )
    expect(screen.queryByRole('button', { name: /collapse bugs/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /expand bugs/i })).not.toBeInTheDocument()
    expect(document.querySelector('[data-control="fold"]')).not.toBeInTheDocument()
  })

  // Every branch/chat row carries its own Fork and Thread buttons, so a childless branch row
  // renders nothing beneath it.
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

  // An empty chat row grows no bare child row; its own Thread button adds one.
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

  // An open view is active by definition: the tree gives the row the idle ground at full-strength
  // label text, never the muted label Recents uses (only SidebarTreeRow sets `hasViewIdle`).
  it('gives a chat row whose chat is live open in a pane the idle ground, not a greyed label — even though the row prop itself always arrives hasView: false', () => {
    // Every row in `rows` is seeded with `hasView: false` (deriveRows
    // never seeds live state into the row object — see its own note). The
    // signal has to come from a LIVE subscription to pane membership, not
    // from the prop, so open chat-1 in a real view on the
    // window pane store rather than passing hasView: true into `rows`.
    windowPaneStore.getState().paneActions.openChat('chat-1')

    render(
      <SidebarTree
        rows={rows}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )

    const label = screen.getByText('Fix the thing')
    expect(label.className).not.toContain('text-muted-foreground')
    const row = label.closest('[role="treeitem"]') as HTMLElement
    // Exact token match, not a substring: ROW_INACTIVE also carries
    // `hover:bg-sidebar-element-idle`, and a bare `toContain` can't tell
    // that hover-scoped variant apart from the always-on ground this row
    // is actually asserting.
    expect(row).toHaveClass('bg-sidebar-element-idle')
    // The folder row's chat never opened anywhere — no false-positive.
    const folderLabel = screen.getByText('Bugs')
    expect(folderLabel.className).not.toContain('text-muted-foreground')
    const folderRow = folderLabel.closest('[role="treeitem"]') as HTMLElement
    expect(folderRow).not.toHaveClass('bg-sidebar-element-idle')
    expect(folderRow).toHaveClass('hover:bg-sidebar-element-idle')
  })

  it('does not grey or ground a chat row whose chat is not open in any pane', () => {
    render(
      <SidebarTree
        rows={rows}
        onOpen={vi.fn()}
        onTrash={vi.fn()}
        onCreate={vi.fn()}
        {...DRAG_PROPS}
      />,
    )

    const label = screen.getByText('Fix the thing')
    expect(label.className).not.toContain('text-muted-foreground')
    const row = label.closest('[role="treeitem"]') as HTMLElement
    // Same exact-token distinction as above: the row's hover-only ground
    // (`hover:bg-sidebar-element-idle`, ROW_INACTIVE) is fine here — only
    // the always-on ground (ROW_HAS_VIEW_IDLE) would be a false grounding.
    expect(row).not.toHaveClass('bg-sidebar-element-idle')
    expect(row).toHaveClass('hover:bg-sidebar-element-idle')
  })
})

// Rows carry an inert `working`; each row reads the live flag from the ChatStore.
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

  const setWorking = (value: boolean) => setChatWorking('chat-live', value)

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
    resetChatStore()
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

  /** A chat the store learns about after the row is drawn still spins it. */
  it('spins a row whose chat reaches the store after the row is drawn', () => {
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

    act(() => setChatWorking('chat-late', true))

    expect(isSpinning(container)).toBe(true)
  })

  /** A bubble carries a real chat identity too — §5.7's "what is up right now"
   *  is not a workspace-only question. */
  it('spins a chat bubble whose conversation is mid-turn', () => {
    setChatWorking('chat-bubble', true)

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
