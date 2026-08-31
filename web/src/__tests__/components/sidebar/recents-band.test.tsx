import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { RecentsBand, type RecentsBandEntry } from '@/components/sidebar/recents-band'

// Task 21's drag wiring — a null scrollRef and no-op commit callbacks are
// enough for every test below, none of which exercises a live drag.
const DRAG_PROPS = {
  scrollRef: { current: null } as React.RefObject<HTMLElement | null>,
  onDrop: vi.fn(),
  onPaneDrop: vi.fn(),
}

interface FakeChat {
  id: string
  workspaceId: string
  title: string
}

const DEFAULT_CHATS: FakeChat[] = [
  { id: 'chat-1', workspaceId: 'ws-1', title: 'Chat One' },
  { id: 'chat-2', workspaceId: 'ws-1', title: 'Chat Two' },
]

const { stores } = vi.hoisted(() => ({
  stores: {
    current: new Map<string, { chats: FakeChat[]; working: Record<string, boolean> }>(),
  },
}))

// `RecentsBand` now resolves each chat via `useWorkspaceStoreById(workspaceId,
// ...)` — no ambient `WorkspaceStoreContext.Provider` needed — since a
// project's Recents can span more than one workspace's store (spec §4). The
// mock is keyed by workspaceId, so a test can prove an entry is resolved
// against ITS OWN workspace's data rather than one shared fixture.
vi.mock('@/features/workspace/stores/hooks/use-workspace-store-by-id', () => ({
  useWorkspaceStoreById: (wsId: string, sel: (s: unknown) => unknown) =>
    sel({ agentChats: stores.current.get(wsId) ?? { chats: [], working: {} } }),
}))

beforeEach(() => {
  stores.current = new Map([['ws-1', { chats: DEFAULT_CHATS, working: {} }]])
})

/** Convenience: point 'ws-1' at a fresh chat/working fixture pair. */
function setWs1(chats: FakeChat[], working: Record<string, boolean> = {}) {
  stores.current.set('ws-1', { chats, working })
}

describe('RecentsBand', () => {
  it('renders nothing when there are no entries', () => {
    const { container } = render(
      <RecentsBand entries={[]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('a working entry has no close control', () => {
    setWs1(DEFAULT_CHATS, { 'chat-1': true })
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1'], state: 'working', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    expect(screen.queryByTestId('recents-close-e1')).not.toBeInTheDocument()
    // §5.6: the spinner still rides the member — its absence isn't what hid the close control.
    expect(document.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
  })

  it('a set draws as one shell around its member rows', () => {
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const shell = screen.getByTestId('recents-set-e1')
    expect(within(shell).getAllByTestId(/^recents-row-/)).toHaveLength(2)
  })

  it('entries render flat, no indent', () => {
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1'], state: 'dormant', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    expect(screen.getByTestId('recents-row-chat-1')).not.toHaveAttribute('data-depth')
  })

  it('clicking a row calls onFocus with its entry', () => {
    const onFocus = vi.fn()
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={onFocus} onClose={vi.fn()} {...DRAG_PROPS} />)
    screen.getByRole('treeitem').click()
    expect(onFocus).toHaveBeenCalledWith(entry)
  })

  it('clicking close calls onClose with the entry, not the chat', () => {
    const onClose = vi.fn()
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'live',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={onClose} {...DRAG_PROPS} />)
    screen.getByTestId('recents-close-e1').click()
    expect(onClose).toHaveBeenCalledWith(entry)
  })

  it('the close control is never labelled as a delete', () => {
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const close = screen.getByTestId('recents-close-e1')
    expect(close.getAttribute('aria-label')).not.toMatch(/delete/i)
    // The tree's trash control is what this must NOT render for a Recents row.
    expect(screen.queryByRole('button', { name: /delete/i })).not.toBeInTheDocument()
  })

  it('a live set lights the shell, not the members', () => {
    const entries: RecentsBandEntry[] = [
      {
        id: 'e1',
        localId: 'e1',
        chatIds: ['chat-1', 'chat-2'],
        state: 'live',
        workspaceId: 'ws-1',
      },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const shell = screen.getByTestId('recents-set-e1')
    expect(shell.className).toMatch(/bg-background/)
  })

  it('a dormant (at-rest) set does not light the shell', () => {
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const shell = screen.getByTestId('recents-set-e1')
    expect(shell.className).not.toMatch(/bg-background/)
    expect(shell.className).toMatch(/bg-sidebar-element-idle/)
  })

  // Regression coverage for the doubled-margin bug: `SidebarRow` (via
  // `ROW_BASE`) already carries `mx-1.5 my-0.5 h-9` on every row it renders,
  // tree rows and Recents rows alike. RecentsEntryRow's own outer wrapper
  // used to ALSO apply `mx-1.5 my-0.5` (plus a `p-0.5` shell) around a lone
  // live entry, stacking a second copy of that margin/padding on top of the
  // row's own — a live entry rendered ~8px taller and ~8px narrower than a
  // real tree row despite the row's own `data-testid="recents-row-…"`
  // element still measuring exactly `h-9` on its own (the mismatch the
  // previous investigator missed by checking only that one element).
  // `className` assertions here check the actual rendered box-model classes
  // on BOTH the outer wrapper and the inner row — not just that a row is
  // present — so a doubled margin/padding regresses this test even though
  // every element still renders and every earlier test above still passes.
  function classesOf(el: Element): string[] {
    return el.className.split(/\s+/).filter(Boolean)
  }

  it('a lone live entry does not double SidebarRow’s own margin on its shell wrapper', () => {
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'live',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const rowWrapper = screen.getByTestId('recents-row-chat-1')
    const shellWrapper = rowWrapper.parentElement!

    // The shell takes over SidebarRow's own outer margin exactly once...
    expect(classesOf(shellWrapper)).toEqual(expect.arrayContaining(['mx-1.5', 'my-0.5']))
    // ...and does NOT also reach for a set's padded/larger-radius shell — a
    // lone entry has nothing to group, so it should be pixel-for-pixel a
    // tree row's own footprint (spec §5.2: "exactly as in the tree"), not a
    // shell inflated by an extra 2px of padding on every edge.
    expect(classesOf(shellWrapper)).not.toContain('p-0.5')
    expect(classesOf(shellWrapper)).not.toContain('rounded-xl')

    // ...and the row's own wrapper cancels SidebarRow's redundant instance
    // of that same margin, so the net applied margin is the shell's alone.
    expect(classesOf(rowWrapper)).toEqual(expect.arrayContaining(['-mx-1.5', '-my-0.5']))
  })

  it('a dormant entry keeps SidebarRow’s own margin as the ONLY margin (no shell, nothing to cancel)', () => {
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const rowWrapper = screen.getByTestId('recents-row-chat-1')
    const outerWrapper = rowWrapper.parentElement!

    expect(classesOf(outerWrapper)).not.toContain('mx-1.5')
    expect(classesOf(outerWrapper)).not.toContain('my-0.5')
    expect(classesOf(rowWrapper)).not.toContain('-mx-1.5')
    expect(classesOf(rowWrapper)).not.toContain('-my-0.5')
  })

  it('a set shell does not re-add the margin its members already carry, but each member keeps its own (the pill separation §5.3 asks for)', () => {
    const entries: RecentsBandEntry[] = [
      {
        id: 'e1',
        localId: 'e1',
        chatIds: ['chat-1', 'chat-2'],
        state: 'live',
        workspaceId: 'ws-1',
      },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const shell = screen.getByTestId('recents-set-e1')

    // The shell's own `mx-1.5 my-0.5` was pure dead weight (every member
    // already carries it via ROW_BASE) — gone now, only the real 2px padding
    // shell (§5.3) remains.
    expect(classesOf(shell)).not.toContain('mx-1.5')
    expect(classesOf(shell)).not.toContain('my-0.5')
    expect(classesOf(shell)).toContain('p-0.5')
    expect(classesOf(shell)).toContain('rounded-xl')

    // Members are NOT solo-active — each keeps its own uncancelled margin,
    // which is what visually separates one member's pill from the next.
    for (const rowId of ['recents-row-chat-1', 'recents-row-chat-2']) {
      const member = screen.getByTestId(rowId)
      expect(classesOf(member)).not.toContain('-mx-1.5')
      expect(classesOf(member)).not.toContain('-my-0.5')
    }
  })

  it('reserves room for the close control so a long title truncates before it, not under it', () => {
    // A title long enough to truncate right at the row's edge is the realistic
    // case where an unreserved close button would draw over the last
    // characters (tab-bar-item.tsx's own `pr-8` exists for the identical
    // reason). We can't measure real layout in jsdom, so assert the reserved
    // class directly, on the same element SidebarRow renders into.
    setWs1([{ id: 'chat-1', workspaceId: 'ws-1', title: 'A'.repeat(120) }])
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const rowWrapper = screen.getByTestId('recents-row-chat-1')
    expect(rowWrapper.className).toMatch(/\bpr-10\b/)
  })

  it('does not reserve close-button room on a working row, which has no close control', () => {
    setWs1(DEFAULT_CHATS, { 'chat-1': true })
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'working',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const rowWrapper = screen.getByTestId('recents-row-chat-1')
    expect(rowWrapper.className).not.toMatch(/\bpr-10\b/)
  })

  it('resolves each entry through its OWN workspace store, not one shared assumption', () => {
    // The whole reason for the workspaceId tag: a project's Recents can mix
    // chats from more than one workspace's store (spec §4). 'chat-2' exists
    // ONLY in 'ws-other's fixture, not in 'ws-1's (DEFAULT_CHATS) — if the
    // component ever fell back to a single ambient store, this entry would
    // silently render nothing (RecentsMemberRow returns null when `chat` is
    // undefined).
    stores.current.set('ws-other', {
      chats: [{ id: 'chat-2', workspaceId: 'ws-other', title: 'Other space chat' }],
      working: {},
    })
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1'], state: 'dormant', workspaceId: 'ws-1' },
      { id: 'e2', localId: 'e2', chatIds: ['chat-2'], state: 'dormant', workspaceId: 'ws-other' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    expect(screen.getByTestId('recents-row-chat-1')).toHaveTextContent('Chat One')
    expect(screen.getByTestId('recents-row-chat-2')).toHaveTextContent('Other space chat')
  })
})
