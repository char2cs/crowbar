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
}
