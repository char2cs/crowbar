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
  /**
   * The repo's own identity, present only on the project-home row (the one
   * `rows-from-repo.ts` ever gives a null parentId) — what its click-to-edit
   * icon (EditableRepoIcon, repo-icon-mark.tsx) needs to reach the repo's own
   * REST base and render the repo's actual mark, rather than the generic
   * GitBranch glyph every other branch row draws. Absent when the repo's
   * `projectId` hasn't seeded yet — that row falls back to the generic glyph
   * rather than guessing at a REST base it can't yet build.
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
