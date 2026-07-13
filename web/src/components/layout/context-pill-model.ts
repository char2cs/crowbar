import type { Repo, WorkspaceStatus } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'
import type { RepoAvatarData } from './repo-avatar'

export type ContextPillModel =
  | {
      kind: 'workspace'
      status: WorkspaceStatus
      working?: boolean
      repoName: string
      branchName: string
      /** Only set for the default (imported folder) workspace. */
      repoAvatar?: RepoAvatarData
    }
  | { kind: 'project'; projectName: string }
  | { kind: 'home'; projectName: string; working?: boolean }
  | { kind: 'empty' }

interface DeriveArgs {
  activeWorkspaceId: string | undefined
  isHomeRoute: boolean
  repos: Repo[]
  projects: Project[]
  activeProjectId: string
  /** Live agent-working overlay for the project-home workspace, which rides no
   *  repo and so is tracked separately (see lib/store/home-workspace.ts). */
  homeWorking?: boolean
}

/**
 * Maps current route/store values to what the context pill should display.
 * Workspace context wins; otherwise fall back to the active project name.
 */
export function deriveContextPillModel({
  activeWorkspaceId,
  isHomeRoute,
  repos,
  projects,
  activeProjectId,
  homeWorking,
}: DeriveArgs): ContextPillModel {
  if (isHomeRoute) {
    const project = projects.find((p) => p.id === activeProjectId)
    if (project) return { kind: 'home', projectName: project.name, working: homeWorking }
  }

  if (activeWorkspaceId) {
    const repo = repos.find((r) => r.workspaces.some((ws) => ws.id === activeWorkspaceId))
    const workspace = repo?.workspaces.find((ws) => ws.id === activeWorkspaceId)
    if (repo && workspace) {
      return {
        kind: 'workspace',
        // A statusless workspace is freshly created — matches the 'new' default
        // applied on workspace creation in sidebar.ts.
        status: workspace.status ?? 'new',
        working: workspace.working,
        repoName: repo.name,
        branchName: workspace.branch,
      }
    }
    // The default workspace is the imported folder — excluded from the tree, so
    // it is not in repo.workspaces. Identify it by defaultWorkspaceId and label
    // it "default" (its incidental branch is irrelevant — Crowbar doesn't manage
    // that checkout).
    const defaultRepo = repos.find((r) => r.defaultWorkspaceId === activeWorkspaceId)
    if (defaultRepo) {
      return {
        kind: 'workspace',
        status: 'new',
        // The repo-home workspace is a workspace like any other: an agent can be
        // working in it, and then its icon must spin. Its `working` is dropped
        // from repo.workspaces (default workspaces are not tree rows), so it is
        // surfaced on the Repo itself as defaultWorking (see toSidebarRepo).
        working: defaultRepo.defaultWorking,
        repoName: defaultRepo.name,
        branchName: 'default',
        repoAvatar: {
          url: defaultRepo.avatarURL,
          label: defaultRepo.avatarLabel,
          color: defaultRepo.avatarColor,
        },
      }
    }
  }

  const project = projects.find((p) => p.id === activeProjectId)
  if (project) {
    return { kind: 'project', projectName: project.name }
  }

  return { kind: 'empty' }
}
