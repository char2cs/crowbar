import { useEffect, type RefObject } from 'react'
import { useNavigate } from '@tanstack/react-router'
import {
  DownloadSimple,
  Folder,
  Lock,
  LockOpen,
  PencilSimpleLine,
  Terminal,
  Trash,
} from '@phosphor-icons/react'
import { ContextMenu, useContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'
import { useSidebarStore } from '@/lib/store/sidebar'
import { useSidebarInlineRenameStore } from '@/lib/store/sidebar-inline-rename'
import {
  performCreateFolder,
  performCreateFolderFromChat,
  performCreateHomeFolder,
  performSetWorkspaceLock,
} from '@/components/sidebar/lib/row-actions'
import { workspaceIdOfBranchRow } from '@/components/sidebar/lib/branch-row-id'
import { resolveHomeRowScope } from '@/lib/store/home-tree'
import type { SidebarRow } from '@/components/sidebar/types/sidebar-row'
import { handleCreate, handleTrashRepo } from '@/components/layout/space-content-actions'
import { useAgentProvidersStore } from '@/features/settings/stores/agent-providers-store'
import { providerCanStartOnTerminal } from '@/features/agent/api/agent-api'
import { toast } from '@/features/window/stores/toast-store'

interface SidebarRowContextMenuProps {
  treeRef: RefObject<HTMLElement | null>
  /** Every visible row, to look up kind/parentId by id. */
  rows: SidebarRow[]
  /** Opens the restored RepoImportDialog for the project-home row `repoRowId`. */
  onImport: (repoRowId: string) => void
}

interface MenuData {
  row: SidebarRow
  /** Read from `useSidebarStore` at open time — `SidebarRow` carries no
   *  `locked` field yet, so this can't come from the `rows` prop. */
  locked: boolean
}

/**
 * The sidebar's right-click menu — rename, lock/unlock, branch import,
 * "New folder", and "New thread in Terminal", the verbs Task 8's
 * unification (and this fix) left with no home on `SidebarRow`'s four-prop
 * surface.
 *
 * A SIBLING of the tree, listening for a native `contextmenu` event on
 * `treeRef.current` rather than a hook inside the tree: with the open/closed
 * state inside the tree component, opening this popup re-renders every row
 * to draw a menu that isn't part of the tree at all (the deleted
 * `row-context-menu.tsx` measured this before landing on the sibling
 * design). Out here it re-renders itself.
 *
 * No multiselect: this task doesn't touch the drag/selection system, so a
 * right-click always acts on exactly the one row under the pointer — found
 * via `data-sidebar-row-id`, not the drag system's `readDropRow`.
 */
export function SidebarRowContextMenu({ treeRef, rows, onImport }: SidebarRowContextMenuProps) {
  const menu = useContextMenu<MenuData>()
  const { openAt } = menu
  const navigate = useNavigate()
  // Whether the provider a new chat would actually start under offers a
  // START_HERE terminal (design spec 2.5) — a NARROW, reactive selector
  // (never `.getState()` in render): absence, not a disabled control, so
  // "New thread in Terminal" below is left OUT of `items` entirely rather
  // than pushed disabled. No enabled provider resolved yet is not evidence
  // of "no terminal" — the plain Thread button offers itself unconditionally
  // too and leaves that refusal to `enabledProvider()` at click time; this
  // matches it (providerCanStartOnTerminal(undefined) is permissive).
  const enabledProviderCanStartOnTerminal = useAgentProvidersStore((s) =>
    providerCanStartOnTerminal(s.providers.find((p) => p.enabled)),
  )

  useEffect(() => {
    const tree = treeRef.current
    if (!tree) return
    const resolveMenuData = (rowId: string): MenuData | null => {
      const row = rows.find((r) => r.id === rowId)
      if (!row) return null
      // Asked in the WORKSPACE id space, which a branch row's id is not in: a
      // locked branch is id'd by the chat that owns its workspace
      // (`rows-from-repo.ts`), so matching the raw row id against `w.id`
      // answered `false` for precisely the rows that ARE locked — the menu
      // offered "Lock" on an already-locked branch and never "Unlock".
      const repos = useSidebarStore.getState().repos
      const wsId = workspaceIdOfBranchRow(repos, rowId) ?? rowId
      const locked = repos.some((repo) =>
        repo.workspaces.some((w) => w.id === wsId && w.status === 'locked'),
      )
      return { row, locked }
    }
    const onContextMenu = (e: MouseEvent) => {
      if (!(e.target instanceof HTMLElement)) return
      const el = e.target.closest<HTMLElement>('[role="treeitem"]')
      const rowId = el?.getAttribute('data-sidebar-row-id')
      if (!rowId) return
      const data = resolveMenuData(rowId)
      if (!data) return
      e.preventDefault()
      openAt({ x: e.clientX, y: e.clientY }, data)
    }
    // The row's own "..." button (sidebar-row.tsx, `data-control="repo-menu"`)
    // opens this SAME menu, anchored under the button — explicit user
    // correction: a repo-home row used to carry a second, separate one-item
    // menu of its own (just "Delete Repo"), which drifted out of sync with
    // whatever this menu grew (Import branches, Rename, New folder). One row
    // gets one menu, reachable by right-click OR by the "..." button.
    //
    // Capture phase, not bubble: the button sits inside the row's own
    // `onClick`-to-open div, and every other trailing-cluster button already
    // stops that propagation on the way up (see e.g. the Thread button's own
    // `e.stopPropagation()`). A bubble-phase listener here would race that —
    // whichever runs first wins — where capture always runs first, well
    // before the row (or the button's own bubble handler, if it had one) ever
    // sees the click, so nothing extra is needed on the button itself.
    const onRepoMenuClick = (e: MouseEvent) => {
      if (!(e.target instanceof HTMLElement)) return
      const trigger = e.target.closest<HTMLElement>('[data-control="repo-menu"]')
      if (!trigger) return
      const el = trigger.closest<HTMLElement>('[role="treeitem"]')
      const rowId = el?.getAttribute('data-sidebar-row-id')
      if (!rowId) return
      const data = resolveMenuData(rowId)
      if (!data) return
      e.preventDefault()
      e.stopPropagation()
      const rect = trigger.getBoundingClientRect()
      openAt({ x: rect.left, y: rect.bottom + 4 }, data)
    }
    tree.addEventListener('contextmenu', onContextMenu)
    tree.addEventListener('click', onRepoMenuClick, true)
    return () => {
      tree.removeEventListener('contextmenu', onContextMenu)
      tree.removeEventListener('click', onRepoMenuClick, true)
    }
  }, [treeRef, rows, openAt])

  if (!menu.isOpen || !menu.data) return null
  const { row, locked } = menu.data
  // The repo header is identified by the row itself, never by its parent: a
  // repo's entry may be filed into a project-home folder and is still the repo.
  const repoIcon = row.repoIcon
  const isProjectHome = repoIcon !== undefined
  const isLockedBranch = row.kind === 'branch' && locked

  const items: ContextMenuItem[] = []

  // A locked branch keeps its checked-out branch name — same "must stay put"
  // reasoning the lock itself exists for (see Lock/Unlock below); starting a
  // rename here would arm an editor for a write `performRenameWorkspaceBranch`
  // silently refuses once the branch is locked, so the row *looked* renamable
  // and wasn't.
  //
  // Starts the SAME inline editor double-click already does
  // (sidebar-inline-rename.ts), not a modal — renaming is inline everywhere in
  // this app, the context menu is just a second entry point into it. Reaches
  // it directly rather than through a prop: this component fires for a row
  // wherever it renders (a tree row or its Recents mirror share one id), and
  // the store — not this menu — is what already knows which of the two
  // instances actually draws the input.
  if (!isLockedBranch) {
    items.push({
      id: 'rename',
      label: 'Rename',
      icon: <PencilSimpleLine />,
      onClick: () => useSidebarInlineRenameStore.getState().startRenaming(row.id),
    })
  }

  // Lock/Unlock only for a real workspace branch row — NOT `row.ownsWorktree`,
  // which is a "+"-button semantic (fork a workspace vs. start a thread) that
  // a folder row also carries true (its own "+" always forks a branch). Nor
  // the project-home row: it IS the repo's own checkout, the one branch that
  // must stay put — the deleted row-menu-model.ts's own words, "handing it
  // out for editing under the sidebar's rules is not what the lock is for."
  if (row.kind === 'branch' && !isProjectHome) {
    items.push(
      locked
        ? {
            id: 'unlock',
            label: 'Unlock',
            icon: <LockOpen />,
            onClick: () => void performSetWorkspaceLock(row.id, false),
          }
        : {
            id: 'lock',
            label: 'Lock',
            icon: <Lock />,
            onClick: () => void performSetWorkspaceLock(row.id, true),
          },
    )
  }

  if (isProjectHome) {
    items.push({
      id: 'import',
      label: 'Import branches',
      icon: <DownloadSimple />,
      onClick: () => onImport(row.id),
    })
  }

  if (row.kind === 'branch' || row.kind === 'folder' || row.kind === 'chat') {
    // A home row is never in `useSidebarStore`'s `repos` at all (home rides
    // no repo), so `performCreateFolder`'s repo lookup finds nothing for one
    // and silently no-ops — needs the home-scoped create instead.
    //
    // Reported live: right-clicking a plain CHAT row (repo-scoped or home)
    // offered no "New folder" at all — the ONE item that used to require a
    // branch/folder row to already exist, so a fresh tree with none yet had
    // no row-level path to create the first one. A bubble carries no
    // folder-anchor of its own, so its "New folder" always root-normalises
    // (`''`) rather than nesting under the bubble — same as clicking a
    // repo's own home row already does for `performCreateFolder`.
    const homeScope = resolveHomeRowScope(row.id)
    const isChat = row.kind === 'chat'
    items.push({
      id: 'new-folder',
      label: 'New folder',
      icon: <Folder />,
      onClick: () =>
        void (homeScope
          ? performCreateHomeFolder(homeScope.projectId, isChat ? '' : row.id)
          : isChat
            ? performCreateFolderFromChat(row.id)
            : performCreateFolder(row.id)),
    })

    // THE BUG this exists for: "can't start chats directly on a CLI, it
    // always obligates me to use the native chat" — no creation entry point
    // could land a single new chat on Terminal without flipping
    // chatIsDefaultPresentation (Settings → Chat) globally. Same
    // `handleCreate` the row's own "+" Thread button calls, with the
    // optional 4th arg that presets the landed chat's surface. Absence, not
    // disabled: left out of `items` entirely, never pushed with
    // `disabled: true`, whenever the provider that would run it has no
    // terminal at all.
    if (enabledProviderCanStartOnTerminal) {
      items.push({
        id: 'new-thread-terminal',
        label: 'New thread in Terminal',
        icon: <Terminal />,
        onClick: () => handleCreate(row.id, 'thread', navigate, 'terminal'),
      })
    }
  }

  // The repo's real delete entry point — `handleTrash` refuses this ONE row
  // (it resolves to just the repo's own default-branch workspace, not the
  // whole repo).
  if (repoIcon) {
    items.push(
      { id: 'delete-repo-separator', separator: true, label: '', onClick: () => {} },
      {
        id: 'delete-repo',
        label: 'Delete Repo',
        icon: <Trash />,
        className:
          'text-destructive data-highlighted:bg-destructive/10 data-highlighted:text-destructive dark:data-highlighted:bg-destructive/20',
        onClick: () => {
          if (!handleTrashRepo(repoIcon.repoId)) {
            toast.error(`Can't delete ${row.label} yet`)
          }
        },
      },
    )
  }

  return <ContextMenu isOpen items={items} position={menu.position} onClose={menu.close} />
}
