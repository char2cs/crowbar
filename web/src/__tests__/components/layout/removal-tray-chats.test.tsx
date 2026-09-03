/**
 * The removal tray, holding rows from BOTH trees.
 *
 * The Chats panel removes a row exactly as the workspace sidebar does — onto the
 * editor pane, into this tray, with eight seconds of undo — so the two share one
 * store, one clock and one commit path. What they do not share is a footer: the
 * two panels are pages of a carousel and only one is on screen at a time, so the
 * tray is drawn twice and each instance shows only the rows its own tree can
 * undo. A held row on a page you have to swipe to is a held row nobody sees.
 *
 * That split is what this file pins, along with the two things a chat row brings
 * with it: its own glyph and its own typeface (a chat title is prose, not a git
 * ref), and an unload flush that stays a single send however many trays are
 * mounted.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, render, screen } from '@testing-library/react'

vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => () => {},
  useRouterState: () => '',
  useRouter: () => ({ state: { location: { pathname: '/' } } }),
  useMatch: () => null,
}))

const { sendChatRemoval, deleteWorkspace } = vi.hoisted(() => ({
  sendChatRemoval: vi.fn(async () => {}),
  deleteWorkspace: vi.fn(async () => {}),
}))

vi.mock('@/features/agent/tree/lib/chat-removal', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/features/agent/tree/lib/chat-removal')>()),
  sendChatRemoval,
}))

vi.mock('@/lib/api', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/lib/api')>()),
  deleteWorkspace,
}))

import { RemovalTray } from '@/components/layout/removal-tray'
import { flushDrainingRemovals } from '@/components/layout/removal-commit'
import {
  getInitialRemovalState,
  useRemovalTrayStore,
  type RemovalDraft,
} from '@/lib/store/sidebar-removal'
import { getInitialState, useSidebarStore } from '@/lib/store/sidebar'

const draft = (over: Partial<RemovalDraft>): RemovalDraft => ({
  kind: 'chat',
  id: 'c1',
  label: 'A chat title',
  wsId: 'w1',
  providerIcon: '',
  projectId: '',
  repoId: '',
  hiddenIds: ['c1'],
  extra: 0,
  fallbackWsId: null,
  ...over,
})

const CLAUDE_SVG = '<svg data-p="claude"></svg>'
const CHAT = draft({ providerIcon: CLAUDE_SVG })
const CHAT_FOLDER = draft({ kind: 'chatFolder', id: 'f1', label: 'Spikes', hiddenIds: ['f1'] })
const WORKSPACE = draft({
  kind: 'workspace',
  id: 'ws1',
  label: 'feature/branch',
  wsId: '',
  projectId: 'p1',
  repoId: 'r1',
  hiddenIds: ['ws1'],
})

const hold = (...drafts: RemovalDraft[]) => act(() => useRemovalTrayStore.getState().hold(drafts))

/** The name each held row is drawn under — the label span, not the clock. */
const rowLabels = () =>
  [...document.querySelectorAll<HTMLElement>('[data-removal-entry] > span.flex-1')].map(
    (el) => el.textContent,
  )

beforeEach(() => {
  vi.clearAllMocks()
  useRemovalTrayStore.setState(getInitialRemovalState())
  useSidebarStore.setState(getInitialState())
})

afterEach(() => {
  cleanup()
})

describe('RemovalTray scoping', () => {
  it('the chats tray shows chat rows and not the sidebar’s', () => {
    render(<RemovalTray scope="chats" />)
    hold(CHAT, CHAT_FOLDER, WORKSPACE)

    expect(rowLabels()).toEqual(['A chat title', 'Spikes'])
  })

  it('the sidebar tray shows the sidebar’s rows and not the chats’', () => {
    render(<RemovalTray />)
    hold(CHAT, CHAT_FOLDER, WORKSPACE)

    expect(rowLabels()).toEqual(['feature/branch'])
  })

  it('a tray with nothing of its own draws nothing at all', () => {
    const { container } = render(<RemovalTray scope="chats" />)
    hold(WORKSPACE)
    expect(container.firstChild).toBeNull()
  })

  it('a chat title reads as prose, and a branch still reads as a ref', () => {
    // A row that changed typeface on its way into the tray would read as a
    // different kind of thing at the one moment the user is deciding whether to
    // keep it.
    render(
      <>
        <RemovalTray scope="chats" />
        <RemovalTray />
      </>,
    )
    hold(CHAT, CHAT_FOLDER, WORKSPACE)

    const face = (label: string) => screen.getByText(label).className
    expect(face('A chat title')).toContain('font-sans')
    expect(face('A chat title')).not.toContain('font-semibold')
    // A folder name is prose too, and a container — the same weight the folder
    // row wears in the tree.
    expect(face('Spikes')).toContain('font-semibold')
    expect(face('feature/branch')).toContain('font-mono')
  })

  it('a held chat wears its PROVIDER’s mark, the one the row wore', () => {
    // It drew a generic message-square here once, which made a chat in the tray
    // the only row that did not look like the row it came from — at the one
    // moment the user is deciding whether to keep it. Every other kind already
    // showed its real mark (a repo its avatar, a workspace its branch icon, a
    // folder the duotone folder).
    render(<RemovalTray scope="chats" />)
    hold(CHAT)

    const row = document.querySelector('[data-removal-entry]')!
    expect(row.querySelector('[data-p="claude"]')).not.toBeNull()
    expect(row.querySelector('[data-provider-icon]')).not.toBeNull()
  })

  it('a held chat whose provider has gone falls back to the chat glyph', () => {
    // Same fallback the sidebar row takes, from the same component — never a
    // stand-in chosen only for the tray.
    render(<RemovalTray scope="chats" />)
    hold(draft({ providerIcon: '' }))

    const row = document.querySelector('[data-removal-entry]')!
    expect(row.querySelector('[data-chat-glyph]')).not.toBeNull()
  })

  it('a held chat is drawn STATIC — the tray never runs the turn spinner', () => {
    // A chat on its way out is not doing a turn, and `working` deliberately does
    // not travel on the draft.
    render(<RemovalTray scope="chats" />)
    hold(CHAT)

    const row = document.querySelector('[data-removal-entry]')!
    expect(row.querySelector('[data-flicker-spinner]')).toBeNull()
    expect(row.querySelector('[role="status"]')).toBeNull()
  })

  it('a chat FOLDER keeps the same duotone folder a workspace folder wears', () => {
    render(<RemovalTray scope="chats" />)
    hold(CHAT_FOLDER)

    const row = document.querySelector('[data-removal-entry]')!
    expect(row.querySelector('svg')).not.toBeNull()
    expect(row.querySelector('[data-provider-icon]')).toBeNull()
  })

  it('a held chat drains rather than waiting on an answer', () => {
    // A conversation is one row's worth of work, not a project's: eight seconds
    // of undo is a proportionate net, and there is no Remove button to press.
    render(<RemovalTray scope="chats" />)
    hold(CHAT)

    expect(useRemovalTrayStore.getState().entries[0].deadlineAt).not.toBeNull()
    expect(screen.getByRole('button', { name: /keep a chat title/i })).toBeTruthy()
    expect(screen.queryByRole('button', { name: 'Remove' })).toBeNull()
  })
})

describe('flushDrainingRemovals with two trays mounted', () => {
  it('sends each pending removal exactly once, however many trays call it', () => {
    hold(CHAT, WORKSPACE)

    // Both panels are mounted at once inside the sidebar carousel, so both
    // register the pagehide handler — the entry is settled before its send, so
    // the second call finds nothing left to post.
    flushDrainingRemovals()
    flushDrainingRemovals()

    expect(sendChatRemoval).toHaveBeenCalledTimes(1)
    expect(deleteWorkspace).toHaveBeenCalledTimes(1)
    expect(useRemovalTrayStore.getState().entries).toEqual([])
  })
})
