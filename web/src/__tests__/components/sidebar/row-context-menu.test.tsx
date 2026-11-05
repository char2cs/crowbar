import { describe, expect, it, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { InlineRenameInput } from '@/components/sidebar/inline-rename-input'
import { SidebarRowContextMenu } from '@/components/sidebar/row-context-menu'
import { inputOf, repoRows } from '@/__tests__/__fixtures__/tree-rows'
import { deriveRows } from '@/features/workspace/tree/rows'
import type { Repo } from '@/core/entities/repo-types'
import { treeUiStore, useTreeUi } from '@/features/workspace/tree/tree-ui-store'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import * as api from '@/lib/api'
import * as sidebarPlacement from '@/lib/api/sidebar-placement'
import * as homeWorkspaceResolver from '@/features/workspace/lib/home-workspace-resolver'
import * as trashActions from '@/components/layout/trash-actions'
import * as createActions from '@/components/layout/create-actions'
import { toast } from '@/features/window/stores/toast-store'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { seedHomeTree, seedRepos } from '@/__tests__/__fixtures__/sidebar-entities'

// handleCreate's fallback needs a router `navigate`.
vi.mock('@tanstack/react-router', () => ({
  useNavigate: () => vi.fn(),
}))

// The menu's verbs are asserted at their API boundary; `resolveHomeRowScope` gets a resolved home
// workspace id rather than exercising its async fetch.
function stubMenuVerbs() {
  vi.spyOn(toast, 'error').mockImplementation(() => '')
  vi.spyOn(toast, 'success').mockImplementation(() => '')
  vi.spyOn(trashActions, 'handleTrashRepo').mockImplementation(() => false)
  vi.spyOn(createActions, 'handleCreate').mockResolvedValue(undefined)
  vi.spyOn(api, 'setWorkspaceLock').mockResolvedValue(undefined)
  vi.spyOn(sidebarPlacement, 'createFolder').mockResolvedValue({
    folder: {
      id: 'folder-new',
      repoId: 'repo-1',
      projectId: 'proj-1',
      name: 'New folder',
      order: 0,
    },
    shifted: [],
    shiftedRows: [],
  })
  vi.spyOn(sidebarPlacement, 'createHomeFolder').mockResolvedValue({
    folder: {
      id: 'home-folder-new',
      repoId: '',
      projectId: 'proj-1',
      name: 'New folder',
      order: 0,
    },
    shifted: [],
    shiftedRows: [],
  })
  vi.spyOn(homeWorkspaceResolver, 'getHomeWorkspaceId').mockReturnValue('home-ws-1')
}

// The fixture is the store and the rows are derived from it, id space included: hand-written
// row ids hid every id-space bug on this seam (a row id that was `w.id` matched a check on `w.id`).
const REPO: Repo = {
  id: 'repo-1',
  projectId: 'proj-1',
  name: 'repo',
  avatarLabel: 'R',
  avatarColor: 'bg-indigo-700',
  defaultWorkspaceId: 'ws-home',
  defaultBranch: 'main',
  workspaces: [
    { id: 'ws-1', branch: 'feature-x', age: '' },
    { id: 'ws-2', branch: 'develop', age: '', status: 'locked', owningChatId: 'ws-2-row' },
  ],
  folders: [{ id: 'folder-1', repoId: 'repo-1', name: 'Bugs', order: 0 }],
  chats: [
    {
      id: 'home-row',
      repoId: 'repo-1',
      ownsWorktree: true,
      workspaceId: 'ws-home',
      title: '',
      order: 0,
    },
    {
      id: 'ws-2-row',
      repoId: 'repo-1',
      ownsWorktree: true,
      workspaceId: 'ws-2',
      title: '',
      order: 1,
    },
    // A plain thread — no `ownsWorktree`, no parent, no workspace ground —
    // exactly what a chat row with no folder yet to right-click looks like.
    {
      id: 'thread-1',
      repoId: 'repo-1',
      ownsWorktree: false,
      title: 'A thread',
      order: 2,
    },
  ],
}
/** A plain (repo-scoped) thread's row id, addendum's own reported gap: no
 *  right-click path to create the first folder from a bare chat row. */
const THREAD_ROW_ID = 'thread-1'

/** The repo-home row's id: the owning chat, never `defaultWorkspaceId`. */
const HOME_ROW_ID = 'home-row'
/** A LOCKED branch row's id: likewise the owning chat, never `ws-2`. */
const LOCKED_ROW_ID = 'ws-2-row'
/** A regular (unlocked) fork keeps the workspace id — the one branch row whose
 *  two id spaces still coincide, and the reason a bug here stayed invisible. */
const FORK_ROW_ID = 'ws-1'

let rows: SidebarRow[] = []

/** The tree's inline editor for whichever row the store is renaming. */
function RenamingRow() {
  const stop = () => treeUiStore.getState().stopRenaming()
  const renaming = useTreeUi((s) => s.renaming)
  return renaming && <InlineRenameInput defaultValue="" onConfirm={stop} onCancel={stop} />
}

/** `tree` renders in the menu's own React tree, as the sidebar's rows do. */
function renderMenu(tree?: React.ReactNode) {
  const treeRef = { current: document.createElement('div') }
  document.body.appendChild(treeRef.current)
  const onImport = vi.fn()
  render(
    <>
      {tree}
      <SidebarRowContextMenu treeRef={treeRef} rows={rows} onImport={onImport} />
    </>,
  )
  return { treeRef, onImport }
}

function rightClick(tree: HTMLElement, rowId: string) {
  const target = document.createElement('div')
  target.setAttribute('role', 'treeitem')
  target.setAttribute('data-sidebar-row-id', rowId)
  target.tabIndex = 0
  tree.appendChild(target)
  // The row holds focus when the menu opens, so that is where the menu returns it.
  target.focus()
  fireEvent.contextMenu(target)
  return target
}

/** sidebar-row.tsx's own "..." button, nested inside the row exactly as it
 *  really renders — the row-level `[role="treeitem"]` ancestor carries the
 *  id, not the button itself. */
function clickRepoMenuButton(tree: HTMLElement, rowId: string) {
  const row = document.createElement('div')
  row.setAttribute('role', 'treeitem')
  row.setAttribute('data-sidebar-row-id', rowId)
  const button = document.createElement('button')
  button.setAttribute('data-control', 'repo-menu')
  row.appendChild(button)
  tree.appendChild(row)
  fireEvent.click(button)
  return button
}

/** A project-home folder, exactly as `deriveRows` would draw it — home
 *  rides no repo, so it is never part of `repoRows(REPO)`'s output. */
const HOME_FOLDER_ROW_ID = 'home-folder-1'
const homeFolderRow: SidebarRow = {
  id: HOME_FOLDER_ROW_ID,
  kind: 'folder',
  parentId: null,
  order: 0,
  label: 'Home Folder',
  ownsWorktree: false,
  workspaceId: null,
  working: false,
  hasView: false,
}

/** A plain (home-scoped) thread, same gap as `THREAD_ROW_ID` above but for a
 *  project's home rather than a repo — likewise synthetic, since home rows
 *  never come out of `deriveRows`. */
const HOME_THREAD_ROW_ID = 'home-thread-1'
const homeThreadRow: SidebarRow = {
  id: HOME_THREAD_ROW_ID,
  kind: 'chat',
  parentId: null,
  order: 1,
  label: 'A home thread',
  ownsWorktree: false,
  workspaceId: null,
  working: false,
  hasView: false,
}

beforeEach(() => {
  vi.clearAllMocks()
  stubMenuVerbs()
  seedRepos([REPO])
  treeUiStore.setState(treeUiStore.getInitialState(), true)
  seedHomeTree('proj-1', {
    chats: [
      {
        id: 'home-branch-chat',
        repoId: '',
        ownsWorktree: true,
        workspaceId: 'home-ws-1',
        title: '',
        order: 0,
      },
      {
        id: HOME_THREAD_ROW_ID,
        repoId: '',
        ownsWorktree: false,
        title: 'A home thread',
        order: 1,
      },
    ],
    folders: [{ id: HOME_FOLDER_ROW_ID, repoId: '', name: 'Home Folder', order: 0 }],
  })
  rows = [...repoRows(REPO), homeFolderRow, homeThreadRow]
})

describe('SidebarRowContextMenu', () => {
  // The rows this menu is driven by really do carry the ids this file asserts —
  // if `deriveRows` ever stops giving a locked branch its owning chat's id,
  // the fixture must follow it rather than the other way round.
  it('is driven by deriveRows’s real output, where a locked branch is id’d by its owning chat', () => {
    expect(rows.map((r) => r.id).sort()).toEqual(
      [
        HOME_ROW_ID,
        FORK_ROW_ID,
        LOCKED_ROW_ID,
        THREAD_ROW_ID,
        'folder-1',
        HOME_FOLDER_ROW_ID,
        HOME_THREAD_ROW_ID,
      ].sort(),
    )
    expect(rows.find((r) => r.id === LOCKED_ROW_ID)?.workspaceId).toBe('ws-2')
  })

  // The project-home row IS the repo's own checkout, the one branch that must stay put,
  // even though it carries `ownsWorktree` like any fork.
  it('right-clicking the project-home row offers Rename and Import, but not Lock/Unlock', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, HOME_ROW_ID)
    expect(screen.getByText('Rename')).toBeInTheDocument()
    expect(screen.getByText('Import branches')).toBeInTheDocument()
    expect(screen.queryByText('Lock')).not.toBeInTheDocument()
    expect(screen.queryByText('Unlock')).not.toBeInTheDocument()
  })

  it('right-clicking a real workspace branch row offers Rename and Lock, not Import', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, FORK_ROW_ID)
    expect(screen.getByText('Rename')).toBeInTheDocument()
    expect(screen.getByText('Lock')).toBeInTheDocument()
    expect(screen.queryByText('Import branches')).not.toBeInTheDocument()
  })

  it('clicking Lock on an unlocked fork fires setWorkspaceLock for its workspace', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, FORK_ROW_ID)
    fireEvent.click(screen.getByText('Lock'))
    expect(api.setWorkspaceLock).toHaveBeenCalledWith('ws-1', true)
  })

  // Amended in fix round 1: a folder row with `ownsWorktree: true` (matching
  // deriveRows's real output) must still not offer Lock/Unlock — the gate
  // has to be kind-based, not `ownsWorktree`-based.
  it('right-clicking a folder row (ownsWorktree: true, matching real deriveRows output) offers only Rename', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, 'folder-1')
    expect(screen.getByText('Rename')).toBeInTheDocument()
    expect(screen.queryByText('Lock')).not.toBeInTheDocument()
    expect(screen.queryByText('Unlock')).not.toBeInTheDocument()
    expect(screen.queryByText('Import branches')).not.toBeInTheDocument()
  })

  // A locked branch row's id is its owning chat's, which matches no workspace:
  // `locked` must be read in the workspace id space or the lock is a one-way door.
  describe('a LOCKED branch row, addressed by its owning chat id', () => {
    it('offers Unlock rather than Lock', () => {
      const { treeRef } = renderMenu()
      rightClick(treeRef.current, LOCKED_ROW_ID)
      expect(screen.getByText('Unlock')).toBeInTheDocument()
      expect(screen.queryByText('Lock')).not.toBeInTheDocument()
    })

    // performRenameWorkspaceBranch (row-actions.ts) already refuses a locked
    // workspace's rename write silently — the menu offering Rename here just
    // meant the dialog opened, took a new name, and did nothing with it.
    it('does not offer Rename', () => {
      const { treeRef } = renderMenu()
      rightClick(treeRef.current, LOCKED_ROW_ID)
      expect(screen.queryByText('Rename')).not.toBeInTheDocument()
    })

    it('clicking Unlock actually fires setWorkspaceLock for the WORKSPACE it owns', () => {
      const { treeRef } = renderMenu()
      rightClick(treeRef.current, LOCKED_ROW_ID)
      fireEvent.click(screen.getByText('Unlock'))
      expect(api.setWorkspaceLock).toHaveBeenCalledWith('ws-2', false)
    })

    // `POST .../chats/folders` resolves `parentId` as a chat/folder: the row's own
    // (chat) id is sent, never the workspace it translates to (that 404s).
    it('clicking New folder actually creates one under the CHAT it owns, not its workspace', () => {
      const { treeRef } = renderMenu()
      rightClick(treeRef.current, LOCKED_ROW_ID)
      fireEvent.click(screen.getByText('New folder'))
      expect(sidebarPlacement.createFolder).toHaveBeenCalledWith(
        'proj-1',
        'repo-1',
        'New folder',
        LOCKED_ROW_ID,
      )
    })
  })

  it('clicking New folder on the repo-home row roots it at the repo', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, HOME_ROW_ID)
    fireEvent.click(screen.getByText('New folder'))
    expect(sidebarPlacement.createFolder).toHaveBeenCalledWith('proj-1', 'repo-1', 'New folder', '')
  })

  // A fresh tree with no branch/folder yet still needs a row-level path to its first folder.
  it('a plain repo-scoped chat row offers New folder, root-normalised (a bubble has no folder-anchor of its own)', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, THREAD_ROW_ID)
    expect(screen.getByText('New folder')).toBeInTheDocument()
    fireEvent.click(screen.getByText('New folder'))
    expect(sidebarPlacement.createFolder).toHaveBeenCalledWith('proj-1', 'repo-1', 'New folder', '')
  })

  // Same gap, project-home side: a plain home chat rides no repo at all, so
  // it goes through `performCreateHomeFolder` instead, same root-normalised
  // parentId a home-scoped folder click already uses.
  it('a plain home-scoped chat row offers New folder, HOME-scoped and root-normalised', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, HOME_THREAD_ROW_ID)
    expect(screen.getByText('New folder')).toBeInTheDocument()
    fireEvent.click(screen.getByText('New folder'))
    expect(sidebarPlacement.createHomeFolder).toHaveBeenCalledWith('proj-1', 'New folder', '')
  })

  it('clicking Rename starts inline rename for the row id, not a modal', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, 'folder-1')
    fireEvent.click(screen.getByText('Rename'))
    expect(treeUiStore.getState().renaming).toBe('folder-1')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('a rename started from the menu keeps its focused input once the menu has closed', async () => {
    const { treeRef } = renderMenu(<RenamingRow />)
    rightClick(treeRef.current, 'folder-1')
    fireEvent.click(screen.getByText('Rename'))
    await waitFor(() => expect(screen.queryByRole('menu')).not.toBeInTheDocument())
    expect(screen.getByRole('textbox')).toHaveFocus()
  })

  it('clicking Import branches calls onImport with the project-home row id', () => {
    const { treeRef, onImport } = renderMenu()
    rightClick(treeRef.current, HOME_ROW_ID)
    fireEvent.click(screen.getByText('Import branches'))
    expect(onImport).toHaveBeenCalledWith(HOME_ROW_ID)
  })

  // The repo-home row's "..." button opens this same menu, never a separate one.
  it('right-clicking the repo-home row also offers Delete Repo', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, HOME_ROW_ID)
    expect(screen.getByText('Delete Repo')).toBeInTheDocument()
  })

  it('right-clicking an ordinary branch row does not offer Delete Repo', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, FORK_ROW_ID)
    expect(screen.queryByText('Delete Repo')).not.toBeInTheDocument()
  })

  // A repo's entry can be filed into a project-home folder (`repo.folderId`,
  // deriveRows's header push) — it is still the repo, so its menu is
  // the repo header's, not a plain branch row's. Identity was keyed on
  // `parentId === null`, so the filed header lost Import/Delete Repo and
  // gained a Lock that `performSetWorkspaceLock` silently no-ops for.
  it('a repo header filed into a home folder keeps the repo menu: Import and Delete Repo, never Lock', () => {
    const folder = { id: HOME_FOLDER_ROW_ID, repoId: '', name: 'Home Folder', order: 0 }
    const home = { workspaceId: 'home-ws-1', chats: [], folders: [folder] }
    rows = deriveRows(inputOf({ repos: [{ ...REPO, folderId: HOME_FOLDER_ROW_ID }], home }))
    expect(rows.find((r) => r.id === HOME_ROW_ID)?.parentId).toBe(HOME_FOLDER_ROW_ID)
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, HOME_ROW_ID)
    expect(screen.getByText('Import branches')).toBeInTheDocument()
    expect(screen.getByText('Delete Repo')).toBeInTheDocument()
    expect(screen.queryByText('Lock')).not.toBeInTheDocument()
    expect(screen.queryByText('Unlock')).not.toBeInTheDocument()
  })

  describe('the repo-home row\'s own "..." button (sidebar-row.tsx, data-control="repo-menu")', () => {
    it('opens the exact same menu a right-click on the row opens', () => {
      const { treeRef } = renderMenu()
      clickRepoMenuButton(treeRef.current, HOME_ROW_ID)
      expect(screen.getByText('Rename')).toBeInTheDocument()
      expect(screen.getByText('Import branches')).toBeInTheDocument()
      expect(screen.getByText('New folder')).toBeInTheDocument()
      expect(screen.getByText('Delete Repo')).toBeInTheDocument()
    })

    it('clicking Delete Repo calls handleTrashRepo with the REPO id, not the row id', () => {
      vi.mocked(trashActions.handleTrashRepo).mockReturnValue(true)
      const { treeRef } = renderMenu()
      clickRepoMenuButton(treeRef.current, HOME_ROW_ID)
      fireEvent.click(screen.getByText('Delete Repo'))
      expect(trashActions.handleTrashRepo).toHaveBeenCalledExactlyOnceWith('repo-1')
      expect(toast.error).not.toHaveBeenCalled()
    })

    it('a refusal (nothing held) surfaces a toast instead of pretending to succeed', () => {
      vi.mocked(trashActions.handleTrashRepo).mockReturnValue(false)
      const { treeRef } = renderMenu()
      clickRepoMenuButton(treeRef.current, HOME_ROW_ID)
      fireEvent.click(screen.getByText('Delete Repo'))
      expect(toast.error).toHaveBeenCalledExactlyOnceWith("Can't delete repo yet")
    })

    // The button sits inside the row's own click-to-open div, and every
    // other trailing-cluster button relies on stopping that propagation on
    // the way up. This listens in the capture phase specifically so it
    // cannot lose that race — proven here by asserting the row's own click
    // handler (onOpen) never fires alongside it.
    it("does not also trigger the row's own click-to-open", () => {
      const { treeRef } = renderMenu()
      const onOpen = vi.fn()
      const row = document.createElement('div')
      row.setAttribute('role', 'treeitem')
      row.setAttribute('data-sidebar-row-id', HOME_ROW_ID)
      row.addEventListener('click', onOpen)
      const button = document.createElement('button')
      button.setAttribute('data-control', 'repo-menu')
      row.appendChild(button)
      treeRef.current.appendChild(row)
      fireEvent.click(button)
      expect(onOpen).not.toHaveBeenCalled()
    })
  })

  // Caught live: a project-home folder is never in any repo's `folders`
  // (home rides no repo), so `performCreateFolder`'s repo lookup found
  // nothing and silently no-op'd — no request, no error, no new folder.
  it('right-clicking a project-home folder offers only Rename and New folder', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, HOME_FOLDER_ROW_ID)
    expect(screen.getByText('Rename')).toBeInTheDocument()
    expect(screen.getByText('New folder')).toBeInTheDocument()
    expect(screen.queryByText('Lock')).not.toBeInTheDocument()
    expect(screen.queryByText('Import branches')).not.toBeInTheDocument()
  })

  it('clicking New folder on a project-home folder creates it HOME-scoped, not through the repo endpoint', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, HOME_FOLDER_ROW_ID)
    fireEvent.click(screen.getByText('New folder'))
    expect(sidebarPlacement.createHomeFolder).toHaveBeenCalledWith(
      'proj-1',
      'New folder',
      HOME_FOLDER_ROW_ID,
    )
    expect(sidebarPlacement.createFolder).not.toHaveBeenCalled()
  })

  it('right-clicking an unknown row is a no-op', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, 'nonexistent')
    expect(screen.queryByText('Rename')).not.toBeInTheDocument()
  })
})

// THE BUG: "can't start chats directly on a CLI, it always obligates me to
// use the native chat" — this menu had a Thread-shaped verb (New folder)
// but nothing that landed a new chat on Terminal, and neither does the
// row's own plain "+" button. This is the discoverable, visible affordance:
// same handleCreate the "+" button calls, with the optional 4th
// (presentation) argument the fix added.
describe('SidebarRowContextMenu — "New thread in Terminal"', () => {
  beforeEach(() => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [
        { id: 'claude', enabled: true, hasTerminal: true, terminalStartHere: true },
      ] as never,
    })
  })

  it('offers it alongside New folder on a plain repo-scoped thread row', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, THREAD_ROW_ID)
    expect(screen.getByText('New thread in Terminal')).toBeInTheDocument()
  })

  it('clicking it creates a thread on THIS row, presetting Terminal — the same handleCreate the "+" button uses', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, THREAD_ROW_ID)
    fireEvent.click(screen.getByText('New thread in Terminal'))
    expect(createActions.handleCreate).toHaveBeenCalledWith(
      THREAD_ROW_ID,
      'thread',
      expect.any(Function),
      'terminal',
    )
  })

  it('offers it on a project-home thread row too', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, HOME_THREAD_ROW_ID)
    expect(screen.getByText('New thread in Terminal')).toBeInTheDocument()
  })

  it('offers it on a locked branch row, same as the plain Thread button does', () => {
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, LOCKED_ROW_ID)
    expect(screen.getByText('New thread in Terminal')).toBeInTheDocument()
  })

  // House rule: absence, not a disabled control. A provider with no
  // terminal at all gets NO such item — never a greyed-out one.
  it('is ABSENT, not disabled, when the enabled provider declares no terminal', () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'claude', enabled: true, hasTerminal: false }] as never,
    })
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, THREAD_ROW_ID)
    expect(screen.queryByText('New thread in Terminal')).not.toBeInTheDocument()
    // The plain "New folder" verb is unaffected — this is about ONE item, not
    // the whole menu.
    expect(screen.getByText('New folder')).toBeInTheDocument()
  })

  // THE surfaces: fix (design spec 2.5): hasTerminal alone used to gate this.
  // codex HAS a terminal (attach) that is only reachable by switching to it
  // after a turn — never a launch target for a chat that does not exist yet.
  // terminalStartHere is the fact that distinguishes "has one" from "may
  // launch straight onto it".
  it('is ABSENT when the enabled provider has a terminal that is not a start_here surface', () => {
    useAgentProvidersStore.setState({
      status: 'ready',
      providers: [{ id: 'codex', enabled: true, hasTerminal: true }] as never,
    })
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, THREAD_ROW_ID)
    expect(screen.queryByText('New thread in Terminal')).not.toBeInTheDocument()
    expect(screen.getByText('New folder')).toBeInTheDocument()
  })

  // No enabled provider resolved yet is not evidence of "no terminal" — the
  // plain Thread button offers itself unconditionally too and lets the
  // click-time `enabledProvider()` toast the refusal; this item matches it.
  it('is still offered when no provider is enabled — the refusal happens at click time, same as the plain Thread button', () => {
    useAgentProvidersStore.setState({ status: 'ready', providers: [] })
    const { treeRef } = renderMenu()
    rightClick(treeRef.current, THREAD_ROW_ID)
    expect(screen.getByText('New thread in Terminal')).toBeInTheDocument()
  })
})
