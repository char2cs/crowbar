import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import { RecentsBand, type RecentsBandEntry } from '@/components/sidebar/recents-band'
import { UNTITLED_CHAT_LABEL } from '@/features/agent/lib/chat-label'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'

// Task 21's drag wiring — a null scrollRef and no-op commit callbacks are
// enough for every test below, none of which exercises a live drag.
// `onCloseChat` (the per-chat close, spec below) is bundled here too as a
// no-op default; tests that care about it override the spot after the
// spread — see the "per-chat close" describe block.
const DRAG_PROPS = {
  scrollRef: { current: null } as React.RefObject<HTMLElement | null>,
  onDrop: vi.fn(),
  onPaneDrop: vi.fn(),
  onCloseChat: vi.fn(),
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

// `RecentsBand` resolves each chat via the REGISTERED workspace store for its
// workspaceId (`useRecentsChat` — no ambient `WorkspaceStoreContext.Provider`
// needed), since a project's Recents can span more than one workspace's store
// (spec §4). The mock is keyed by workspaceId, so a test can prove an entry is
// resolved against ITS OWN workspace's data rather than one shared fixture;
// a workspace absent from it has no store, exactly like an unopened one.
vi.mock('@/features/workspace/stores/workspace-store-registry', async (importOriginal) => ({
  ...(await importOriginal<
    typeof import('@/features/workspace/stores/workspace-store-registry')
  >()),
  getWorkspaceStore: (wsId: string) => {
    const agentChats = stores.current.get(wsId)
    if (!agentChats) return undefined
    return { subscribe: () => () => {}, getState: () => ({ agentChats }) }
  },
}))

beforeEach(() => {
  stores.current = new Map([['ws-1', { chats: DEFAULT_CHATS, working: {} }]])
  useSidebarStore.setState(getInitialState())
})

// REGRESSION: after a reload a persisted dormant entry names a chat whose
// workspace store nobody has mounted yet; the row used to resolve only
// through that store and drew nothing. The sidebar's own chat record is
// enough to draw it, and no store is minted for it.
describe('RecentsBand after a reload', () => {
  it('draws a dormant member from the sidebar chat list when its workspace has no store', () => {
    stores.current = new Map()
    useSidebarStore.setState({
      repos: [
        {
          id: 'r1',
          projectId: 'p1',
          name: 'crowbar',
          avatarLabel: 'C',
          avatarColor: 'bg-indigo-700',
          workspaces: [],
          chats: [
            { id: 'chat-9', repoId: 'r1', title: 'Remembered', order: 0, workspaceId: 'ws-9' },
          ],
        },
      ],
    })
    const entries: RecentsBandEntry[] = [
      { id: 'e9', localId: 'e9', chatIds: ['chat-9'], state: 'dormant', workspaceId: 'ws-9' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    expect(screen.getByTestId('recents-row-chat-9')).toBeInTheDocument()
    expect(screen.getByText('Remembered')).toBeInTheDocument()
  })
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
    // A solo entry's one row carries its close via SidebarRow's own trailing
    // cluster now (sidebar-row.tsx's `onClose` prop) — no dedicated
    // `recents-close-{id}` testid of its own any more (that button is a SET's
    // own "close everything" control — see the SET tests below).
    screen.getByRole('button', { name: 'Close Chat One' }).click()
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
    const close = screen.getByRole('button', { name: 'Close Chat One' })
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
        // Lit means SHOWING, not merely open: several views are live at once
        // now and only the one on screen wears the active surface.
        showing: true,
        workspaceId: 'ws-1',
      },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const shell = screen.getByTestId('recents-set-e1')
    expect(shell.className).toMatch(/bg-background/)
  })

  // Live-reported: a SET's shell (`p-0.5` around each member, on top of an
  // UNCHANGED `h-9`) measured taller than a plain row. Each member gives up
  // exactly what that padding adds back (`h-8`, via `compactHeight`) so the
  // shell's own height lands on the same 36px as `ROW_BASE` alone — a solo
  // row (never `compactHeight`) keeps the full `h-9`, since it has no shell
  // padding to compensate for.
  it("a set member gives up ROW_BASE's own h-9 for h-8, compensating for the shell's padding", () => {
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    for (const rowId of ['recents-row-chat-1', 'recents-row-chat-2']) {
      const treeitem = within(screen.getByTestId(rowId)).getByRole('treeitem')
      expect(classesOf(treeitem)).toContain('h-8')
      expect(classesOf(treeitem)).not.toContain('h-9')
    }
  })

  it('a live set that is NOT the view on screen takes no ground of its own', () => {
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
    // Live-reported: a parked/off-screen SET still showed a filled
    // background at rest, with nothing to justify it. It now matches a solo
    // off-screen row exactly (see that test below) — bare until it's the one
    // showing, `hasView`'s greyed label carrying the "still open" signal.
    expect(shell.className).not.toMatch(/bg-background/)
    expect(shell.className).not.toMatch(/bg-sidebar-element-idle/)
    expect(shell.getAttribute('data-view-showing')).toBeNull()
  })

  // Measured, not assumed: `bg-sidebar-element-idle` is a light overlay and
  // `ROW_ACTIVE` a dark inset-lit surface, so grounding every open-but-parked
  // solo row renders it MORE prominent than the one on screen and inverts the
  // hierarchy. What already separates parked from remembered is the row's own
  // `hasView` grey (§3.2).
  it('a live solo row that is not showing takes no ground of its own', () => {
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1'], state: 'live', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const wrapper = screen.getByTestId('recents-row-chat-1').parentElement!
    expect(wrapper.className).not.toMatch(/bg-background/)
    expect(wrapper.className).not.toMatch(/bg-sidebar-element-idle/)
  })

  it('a dormant (at-rest) set does not light the shell', () => {
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const shell = screen.getByTestId('recents-set-e1')
    expect(shell.className).not.toMatch(/bg-background/)
    expect(shell.className).not.toMatch(/bg-sidebar-element-idle/)
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

  it('a lone SHOWING entry does not double SidebarRow’s own margin on its shell wrapper', () => {
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'live',
      // The active surface belongs to the view on screen — that is the one
      // whose wrapper takes over the row's margin.
      showing: true,
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const rowWrapper = screen.getByTestId('recents-row-chat-1')
    const shellWrapper = rowWrapper.parentElement!
    const treeitem = rowWrapper.querySelector('[role="treeitem"]')!

    // The shell takes over SidebarRow's own outer margin exactly once, at a
    // fixed height matching a tree row's own so ROW_ACTIVE's border sits
    // inside it rather than growing an auto height...
    expect(classesOf(shellWrapper)).toEqual(expect.arrayContaining(['mx-1.5', 'my-0.5', 'h-9']))
    // ...and does NOT also reach for a set's padded/larger-radius shell — a
    // lone entry has nothing to group, so it should be pixel-for-pixel a
    // tree row's own footprint (spec §5.2: "exactly as in the tree"), not a
    // shell inflated by an extra 2px of padding on every edge.
    expect(classesOf(shellWrapper)).not.toContain('p-0.5')
    expect(classesOf(shellWrapper)).not.toContain('rounded-xl')

    // ...and SidebarRow's own inner row drops its redundant instance of that
    // same margin at the source (`suppressOwnMargin`) rather than a wrapper
    // cancelling it with an equal-and-opposite margin — that dueling
    // negative margin used to COLLAPSE against the shell's own to zero
    // instead of summing to it (CSS collapses adjoining margins to
    // `max(positives) + min(negatives)`, not a running total), deleting the
    // row's real gutter the moment it became the showing view.
    expect(classesOf(treeitem)).toEqual(expect.arrayContaining(['mx-0', 'my-0']))
  })

  // Live-reported regression: a lone SHOWING entry's title and branch-name
  // subtitle rendered as barely-legible ambient text (ROW_INACTIVE's own
  // `text-foreground`, ROW_SUBLABEL's own `text-muted-foreground`) on top of
  // the shell's inverted `bg-background-inverse` fill — "still using the
  // foreground values instead of the background." Both need the inverted
  // pair (`text-foreground-inverse`) instead, since they sit directly on
  // that inverted ground, not the ambient sidebar background.
  it('a lone SHOWING entry uses inverted text on its title and branch subtitle, not ambient foreground', () => {
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'live',
      showing: true,
      workspaceId: 'ws-1',
      chatIcons: {
        'chat-1': {
          kind: 'branch',
          ownsWorktree: true,
          branchName: 'feature/x',
          added: 3,
          deleted: 1,
        },
      },
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const treeitem = within(screen.getByTestId('recents-row-chat-1')).getByRole('treeitem')
    expect(treeitem).toHaveClass('text-foreground-inverse')
    expect(treeitem).not.toHaveClass('text-foreground')
    const label = treeitem.querySelector('[data-sidebar-row-label]')!
    expect(label.className).not.toContain('text-muted-foreground')
    const subtitle = label.querySelector('span:nth-child(2)')!
    expect(subtitle).toHaveClass('text-foreground-inverse')
    expect(subtitle).not.toHaveClass('text-muted-foreground')
  })

  // Same bug, the SET case: a showing set's shell is ROW_ACTIVE for every
  // member alike (there is no notion of "which member is on screen" within a
  // set), so every member's own text needs the same inversion, not just a
  // lone showing row.
  it('every member of a showing SET uses inverted text, not just a lone showing row', () => {
    const entries: RecentsBandEntry[] = [
      {
        id: 'e1',
        localId: 'e1',
        chatIds: ['chat-1', 'chat-2'],
        state: 'live',
        showing: true,
        workspaceId: 'ws-1',
      },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    for (const rowId of ['recents-row-chat-1', 'recents-row-chat-2']) {
      const treeitem = within(screen.getByTestId(rowId)).getByRole('treeitem')
      expect(treeitem).toHaveClass('text-foreground-inverse')
      expect(treeitem).not.toHaveClass('text-foreground')
    }
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

  // Explicit product correction, asked repeatedly: a SET never gets a
  // separate "close everything" control of its own any more — only each
  // member's own close (per-chat close describe block, below). Live-reported
  // against the PREVIOUS design (first an absolutely-positioned overlay with
  // a permanent `pr-10` reserve on both the shell and every member, later an
  // in-flow flex sibling): either way, a third close affordance alongside two
  // already-visible per-chat ones read as redundant and confusing.
  it('a SET renders no group-close control at all, live or working', () => {
    for (const state of ['live', 'working'] as const) {
      const entries: RecentsBandEntry[] = [
        { id: 'e1', localId: 'e1', chatIds: ['chat-1', 'chat-2'], state, workspaceId: 'ws-1' },
      ]
      const { unmount } = render(
        <RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />,
      )
      expect(screen.queryByTestId('recents-close-e1')).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'Close view' })).not.toBeInTheDocument()
      unmount()
    }
  })

  it("a set shell keeps its own external gutter and matches an ordinary row's radius", () => {
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

    // Unlike a solo-active row (whose outer wrapper is unstyled — the
    // painted box is the row itself, one level in), a SET's shell div IS
    // the painted box: there is no other element to carry its left/right
    // gutter. Vertical margins between adjoining siblings collapse either
    // way, so `my-0.5` here is belt-and-suspenders — but horizontal margins
    // NEVER collapse, so `mx-1.5` is the shell's ONLY source of external
    // gutter. Dropping it (an earlier, wrong pass at this fix) rendered the
    // shell flush against the sidebar's edges, visibly misaligned against
    // every other row in the list.
    expect(classesOf(shell)).toEqual(expect.arrayContaining(['mx-1.5', 'my-0.5']))
    expect(classesOf(shell)).toContain('p-0.5')
    // `rounded-lg`, matching ROW_BASE's own radius — `rounded-xl` here read as
    // a visibly different corner treatment between a plain row and a grouped
    // one (live-reported).
    expect(classesOf(shell)).toContain('rounded-lg')
    expect(classesOf(shell)).not.toContain('rounded-xl')
  })

  // Live-reported: the gap between two members (their two touching, un-
  // cancelled `mx-1.5`s: 12px) read as visibly larger than the gap from the
  // shell's own edge to a member (its `p-0.5`: 2px), with a THIRD value (0)
  // on the vertical edge (the member's OWN `my-0.5` was already cancelled).
  // Fixed by cancelling BOTH margins on every member and letting the shell's
  // own `gap-0.5`/`p-0.5` be the only source of spacing, so all three are the
  // same 2px — the same small gap the tab strip already uses between its own
  // pills (tabs.tsx's `gap-x-0.5`).
  it("every gap around and between a set's members is the same small value, on both axes", () => {
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
    expect(classesOf(shell)).toContain('p-0.5')
    expect(classesOf(shell)).toContain('gap-0.5')

    for (const rowId of ['recents-row-chat-1', 'recents-row-chat-2']) {
      const member = screen.getByTestId(rowId)
      expect(classesOf(member)).toContain('-mx-1.5')
      expect(classesOf(member)).toContain('-my-0.5')
    }
  })

  // Regression pin for the "row is taller than any other row" bug: a set's
  // members are flex items on ONE line now, where margins never collapse —
  // each member's own uncancelled `my-0.5` (2px top + 2px bottom) stacked
  // directly on top of the shell's own `p-0.5` padding, so the shell
  // measured 44px tall instead of matching a single ordinary row's 40px.
  it('a set shell is exactly one row tall, not inflated by its members’ own vertical margin', () => {
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    for (const rowId of ['recents-row-chat-1', 'recents-row-chat-2']) {
      expect(classesOf(screen.getByTestId(rowId))).toContain('-my-0.5')
    }
  })

  // Feedback: "On a single row interaction, we should only display the row
  // with no hover animation apart from showing the close button" — a lone
  // SHOWING entry already sits on its own bold ROW_ACTIVE ground, so the
  // ordinary tree-row hover accent (ROW_INACTIVE's `hover:bg-accent`) is
  // neutralized on it, leaving the close button as the only thing hover
  // reveals.
  it('a lone SHOWING entry neutralizes the ordinary tree-row hover accent on its own ground', () => {
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'live',
      showing: true,
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    expect(screen.getByTestId('recents-row-chat-1').className).toContain(
      '[&_[role="treeitem"]]:hover:bg-transparent',
    )
  })

  // User correction, live, with Zen browser's own grouped-tab capsule as the
  // explicit reference: "when hovering a group, the whole group should
  // receive the hover signal, not only the item being hovered." A SET
  // member's own local hover is silenced entirely now — the shell's own
  // `group-hover` (see its test below) is what answers hover, as ONE shared
  // surface across every member.
  it('a SET member silences its own local hover — the shell answers as one unit instead', () => {
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    for (const rowId of ['recents-row-chat-1', 'recents-row-chat-2']) {
      expect(screen.getByTestId(rowId).className).toContain(
        '[&_[role="treeitem"]]:hover:bg-transparent',
      )
    }
  })

  it('a NOT-showing SET shell lights as one unit on hover, with the CossUI top-highlight', () => {
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const shell = classesOf(screen.getByTestId('recents-set-e1'))
    expect(shell).toContain('group-hover:bg-sidebar-element-hover')
    expect(shell).toContain('group-hover:shadow-xs')
    expect(shell).toContain('group-hover:inset-shadow-[0_1px_var(--elevated-highlight)]')
  })

  it('a SHOWING SET shell does not layer the group-hover treatment on top of ROW_ACTIVE', () => {
    const entries: RecentsBandEntry[] = [
      {
        id: 'e1',
        localId: 'e1',
        chatIds: ['chat-1', 'chat-2'],
        state: 'live',
        showing: true,
        workspaceId: 'ws-1',
      },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const shell = classesOf(screen.getByTestId('recents-set-e1'))
    expect(shell).not.toContain('group-hover:bg-sidebar-element-hover')
  })

  it('a long title still truncates instead of forcing the row wider than the close button', () => {
    // The close button now lives INSIDE SidebarRow's own trailing cluster (a
    // plain flex sibling of the truncating label, same as Thread/Fork/Remove/
    // Fold) rather than an external overlay recents-band.tsx had to reserve
    // padding for — so this is SidebarRow's own truncation behavior, just
    // confirmed end to end through RecentsBand's render path.
    setWs1([{ id: 'chat-1', workspaceId: 'ws-1', title: 'A'.repeat(120) }])
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const row = screen.getByTestId('recents-row-chat-1')
    const label = row.querySelector('[data-sidebar-row-label]')
    expect(label).not.toBeNull()
    expect(label!.className).toMatch(/\btruncate\b/)
    expect(screen.getByRole('button', { name: /^Close /i })).toBeInTheDocument()
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

  // Part A regression (Task 12): the tree's equivalent row builder
  // (rows-from-repo.ts) falls back to UNTITLED_CHAT_LABEL and marks the row
  // `labelProvisional` (→ italic, sidebar-row.tsx) whenever `chat.title` is
  // falsy. RecentsMemberRow built its row straight from `chat.title` with no
  // fallback, so an untitled chat rendered as a bare pill with no visible
  // label at all — not even a placeholder.
  it('an untitled chat renders the UNTITLED_CHAT_LABEL fallback, italic, matching the tree row', () => {
    setWs1([{ id: 'chat-1', workspaceId: 'ws-1', title: '' }])
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const row = screen.getByTestId('recents-row-chat-1')
    expect(row).toHaveTextContent(UNTITLED_CHAT_LABEL)
    const label = row.querySelector('[data-sidebar-row-label]')
    expect(label).not.toBeNull()
    expect(label!.className).toMatch(/\bitalic\b/)
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

  // Feedback #1: "the workspace icon is not the same" — RecentsMemberRow used
  // to hand-build its row with no ownership data at all, so a chat that owns
  // a workspace always fell back to the generic ChatsCircle bubble instead of
  // the tree's own branch/lock/PR-status glyph. `chatIcons` (populated by the
  // real producer, recents-for-project.ts, from the SAME repo data the tree
  // reads) is what fixes that — asserted here via the placeholder warning
  // glyph's `role="img"` (WorkspaceBranchIcon), the one branch icon with a
  // queryable accessible name.
  it("draws a workspace-owning chat's REAL icon, not the generic bubble", () => {
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
      chatIcons: {
        'chat-1': {
          kind: 'branch',
          ownsWorktree: true,
          status: 'new',
          isPlaceholder: true,
          needsProvisioning: true,
        },
      },
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const row = screen.getByTestId('recents-row-chat-1')
    expect(within(row).getByRole('img', { name: /branch needs provisioning/i })).toBeInTheDocument()
  })

  it('a chat with no chatIcons entry keeps the plain bubble (no branch glyph)', () => {
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const row = screen.getByTestId('recents-row-chat-1')
    expect(within(row).queryByRole('img')).not.toBeInTheDocument()
  })

  // Feedback #2: a SET renders its members on ONE horizontal line.
  it('a set shell lays its members out as a horizontal flex row', () => {
    const entries: RecentsBandEntry[] = [
      { id: 'e1', localId: 'e1', chatIds: ['chat-1', 'chat-2'], state: 'set', workspaceId: 'ws-1' },
    ]
    render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const shell = screen.getByTestId('recents-set-e1')
    expect(classesOf(shell)).toContain('flex')
    // Each member shares the row's width and truncates on its own, rather
    // than each rendering at its full natural width.
    for (const rowId of ['recents-row-chat-1', 'recents-row-chat-2']) {
      const member = classesOf(screen.getByTestId(rowId))
      expect(member).toContain('flex-1')
      expect(member).toContain('min-w-0')
    }
  })

  // `flex-1`/`min-w-0` on a solo entry's wrapper is a no-op today (its
  // resting shell is a plain, non-flex `group relative` div — these
  // properties do nothing outside a flex parent), but load-bearing the
  // moment it becomes the showing view: that wrapper turns `flex` for the
  // margin-collapse fix (RecentsEntryRow's own `soloActive` doc), which
  // makes this div a flex ITEM whose default `flex: 0 1 auto` would
  // otherwise shrink it to its own content width instead of the row's real
  // width — pushing the trailing close button up against the label instead
  // of the row's far edge (live-reported). Applying it unconditionally
  // (never gated on `isSet`) is what keeps that working without the wrapper
  // needing to know which of its two flex states it's currently in.
  it('a solo entry still carries flex-1/min-w-0 for when its wrapper turns flex', () => {
    const entry: RecentsBandEntry = {
      id: 'e1',
      localId: 'e1',
      chatIds: ['chat-1'],
      state: 'dormant',
      workspaceId: 'ws-1',
    }
    render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
    const member = classesOf(screen.getByTestId('recents-row-chat-1'))
    expect(member).toContain('flex-1')
    expect(member).toContain('min-w-0')
  })

  // Feedback #3: each chat in a group gets its own hover-close, which removes
  // just that chat without dissolving the group.
  describe('per-chat close (a SET member’s own X)', () => {
    it('renders one per-chat close button per member of a set', () => {
      const entries: RecentsBandEntry[] = [
        {
          id: 'e1',
          localId: 'e1',
          chatIds: ['chat-1', 'chat-2'],
          state: 'set',
          workspaceId: 'ws-1',
        },
      ]
      render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
      // Each member's own close now lives inside its OWN SidebarRow (the
      // trailing-cluster `onClose` prop, sidebar-row.tsx) rather than a
      // separate `recents-close-chat-{id}` overlay button of this file's own.
      expect(
        within(screen.getByTestId('recents-row-chat-1')).getByRole('button', {
          name: 'Close Chat One',
        }),
      ).toBeInTheDocument()
      expect(
        within(screen.getByTestId('recents-row-chat-2')).getByRole('button', {
          name: 'Close Chat Two',
        }),
      ).toBeInTheDocument()
      // No separate group-wide close any more — each member's own is the
      // whole of it (see the SET tests earlier in this file).
      expect(screen.queryByTestId('recents-close-e1')).not.toBeInTheDocument()
    })

    it('clicking a member’s own close calls onCloseChat with the entry AND that chat id, never onClose', () => {
      const onClose = vi.fn()
      const onCloseChat = vi.fn()
      const entry: RecentsBandEntry = {
        id: 'e1',
        localId: 'e1',
        chatIds: ['chat-1', 'chat-2'],
        state: 'set',
        workspaceId: 'ws-1',
      }
      render(
        <RecentsBand
          entries={[entry]}
          onFocus={vi.fn()}
          onClose={onClose}
          {...DRAG_PROPS}
          onCloseChat={onCloseChat}
        />,
      )
      within(screen.getByTestId('recents-row-chat-2'))
        .getByRole('button', { name: 'Close Chat Two' })
        .click()
      expect(onCloseChat).toHaveBeenCalledWith(entry, 'chat-2')
      expect(onClose).not.toHaveBeenCalled()
    })

    it('a SOLO (non-set) entry renders exactly its own row close — no separate group-wide control', () => {
      const entry: RecentsBandEntry = {
        id: 'e1',
        localId: 'e1',
        chatIds: ['chat-1'],
        state: 'dormant',
        workspaceId: 'ws-1',
      }
      render(<RecentsBand entries={[entry]} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
      // A solo entry's one chat IS the whole view — its own SidebarRow close
      // covers it, so there is no second, separate shell-level control (that
      // control only exists for a SET — see RecentsEntryRow's own doc).
      expect(screen.queryByTestId('recents-close-e1')).not.toBeInTheDocument()
      expect(
        within(screen.getByTestId('recents-row-chat-1')).getByRole('button', {
          name: 'Close Chat One',
        }),
      ).toBeInTheDocument()
    })

    it('a working set has no per-chat close either — nothing has a close control while working', () => {
      setWs1(DEFAULT_CHATS, { 'chat-1': true })
      const entries: RecentsBandEntry[] = [
        {
          id: 'e1',
          localId: 'e1',
          chatIds: ['chat-1', 'chat-2'],
          state: 'working',
          workspaceId: 'ws-1',
        },
      ]
      render(<RecentsBand entries={entries} onFocus={vi.fn()} onClose={vi.fn()} {...DRAG_PROPS} />)
      expect(
        within(screen.getByTestId('recents-row-chat-1')).queryByRole('button', {
          name: /^Close /,
        }),
      ).not.toBeInTheDocument()
      expect(
        within(screen.getByTestId('recents-row-chat-2')).queryByRole('button', {
          name: /^Close /,
        }),
      ).not.toBeInTheDocument()
      expect(screen.queryByTestId('recents-close-e1')).not.toBeInTheDocument()
    })
  })
})
