import type { WorkspaceStatus } from '@/lib/store/sidebar'

export type SidebarRowKind = 'chat' | 'branch' | 'folder' | 'workflow'

export interface SidebarRow {
  id: string
  kind: SidebarRowKind
  parentId: string | null
  order: number
  label: string
  labelProvisional?: boolean
  ownsWorktree: boolean
  workspaceId: string | null
  /**
   * Whether this row's Fork control has anything to act on. Only meaningful
   * on a `chat`-kind row (a `branch`/`folder` row already answers this via
   * `ownsWorktree`, which a chat can never set): a project-home bubble rides
   * no repo at all — the same reason a project-home FOLDER's Fork is already
   * hidden (`foldersCanFork`, `walkTreeIntoRows`) — so it sets this `false`
   * rather than offering a button with no worktree to clone. Undefined (a
   * repo-scoped chat) means true — its ground workspace always resolves to
   * a real repo.
   */
  canFork?: boolean
  working: boolean
  hasView: boolean
  branchName?: string
  /** Lines added/deleted vs this workspace's fork parent (`Workspace.added`/
   *  `.deleted`), for the second line rule §3.6 draws under a branch row's
   *  label: `branchName -- added/deleted`. Present only on a `branch`-kind row
   *  that owns a real workspace — absent (not zero) means no diff is known yet,
   *  same as `Workspace.added`/`.deleted` themselves. */
  added?: number
  deleted?: number
  /** Whether the workspace this row owns is a protected/locked branch — the
   *  Lock glyph's own signal (RowGlyph, sidebar-row.tsx). A `branch` row's `id`
   *  no longer doubles as this signal now that every workspace-owning row,
   *  locked or not, is id'd from its owning chat (`rows-from-repo.ts`) rather
   *  than only a locked one. */
  locked?: boolean
  /**
   * The daemon's real status for the workspace this row owns — 'new',
   * 'locked', or the GitHub/GitLab PR states ('pr-open', 'pr-merged',
   * 'pr-closed', 'pr-conflicts') `use-workspace-provider-stream.ts`'s poll
   * keeps live. `RowGlyph` (sidebar-row.tsx) reads it to draw the same
   * icon `WorkspaceBranchIcon` already draws for the workspace switcher —
   * present only on a `branch` row that owns a real workspace, same
   * condition as `locked` (both come off the identical
   * `Workspace.status` read in `rows-from-repo.ts`'s `walk()`).
   */
  status?: WorkspaceStatus
  /**
   * Whether the workspace this row owns has no on-disk worktree at all —
   * `lib/workspace/placeholder.ts`'s `isPlaceholderWorkspace`, the same test
   * `placeholder-toast-watcher.tsx` uses to fire the "Couldn't set up ..."
   * toast. `RowGlyph` renders the warning glyph ahead of the `status` switch
   * for this — a placeholder is locked, but it needs the user's attention,
   * not the "protected, immutable" lock. Present under the same condition as
   * `status`; absent means either "known not to be one" or "not a real
   * workspace-owning row" — never treated as true.
   */
  isPlaceholder?: boolean
  /**
   * The repo's own identity, present only on the repo's own home row (the
   * repo's default-workspace row, `rows-from-repo.ts`'s one root push — its
   * `parentId` is that repo's own `folderId`, not always null: a repo's entry
   * may itself be filed into a project-home folder) — what its click-to-edit
   * icon (EditableRepoIcon, repo-icon-mark.tsx) needs to reach the repo's own
   * REST base and render the repo's actual mark, rather than the generic
   * GitBranch glyph every other branch row draws. Also the signal
   * `sidebar-drop-policy.ts`/`drop-actions.ts` use to tell this ONE branch row
   * apart from every other — its placement lives on `domain.Repository`, not
   * `Workspace`/`Chat`, so it needs a whole different plan. Absent when the
   * repo's `projectId` hasn't seeded yet — that row falls back to the generic
   * glyph rather than guessing at a REST base it can't yet build, and (until
   * it seeds) is not draggable as a repo either.
   */
  repoIcon?: {
    repoId: string
    projectId: string
    name: string
    avatarLabel: string
    avatarColor: string
    avatarURL?: string
  }
  /**
   * Present only on a synthetic row standing in for a create still in
   * flight (pending-creates.ts) — no real chat/workspace exists at `id` yet.
   * `sidebar-row.tsx` renders it non-interactive: a naming input, a spinner,
   * or an inline error, at the exact slot the real row lands in once
   * created.
   */
  pending?: {
    tempId: string
    status: 'naming' | 'creating' | 'error'
    error?: string
  }
  /**
   * Present on a REAL, already-existing row that is currently held in the
   * removal tray (`useRemovalTrayStore`) — attached by
   * `removal-plan.ts`'s `attachRemovalState`, cross-referencing the store's
   * `entries` against this row (a `branch` row matches by `workspaceId`,
   * since a 'workspace'-kind entry's own `id` is the raw workspace id, never
   * the owning-chat id the row is actually rendered/looked-up by — see that
   * function's own doc). `sidebar-row.tsx` renders this row transformed IN
   * PLACE — the countdown, the "goes with" count, the Keep/undo control —
   * rather than hiding it while a separate tray shows the same thing
   * elsewhere. Never present alongside `pending`: a pending row's id is a
   * synthetic tempId no removal entry could ever name.
   */
  removal?: {
    entryId: string
    /** Null only for a 'repo'/'project' kind entry, which `attachRemovalState`
     *  never attaches to a row (those removals have no single row of their
     *  own in this tree) — so this is always a real deadline in practice. */
    deadlineAt: number | null
    extra: number
  }
}
