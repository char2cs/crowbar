import type { Repo, WorkspaceStatus } from '@/lib/store/sidebar'
import type { Project } from '@/lib/types'

export type ContextPillModel =
  | { kind: 'workspace'; status: WorkspaceStatus; repoName: string; branchName: string }
  | { kind: 'project'; projectName: string }
  | { kind: 'empty' }

interface DeriveArgs {
  activeWorkspaceId: string | undefined
  repos: Repo[]
  projects: Project[]
  activeProjectId: string
}

/**
 * Maps current route/store values to what the context pill should display.
 * Workspace context wins; otherwise fall back to the active project name.
 */
export function deriveContextPillModel({
  activeWorkspaceId,
  repos,
  projects,
  activeProjectId,
}: DeriveArgs): ContextPillModel {
  if (activeWorkspaceId) {
    const repo = repos.find((r) => r.workspaces?.some((ws) => ws.id === activeWorkspaceId))
    const workspace = repo?.workspaces.find((ws) => ws.id === activeWorkspaceId)
    if (repo && workspace) {
      return {
        kind: 'workspace',
        status: workspace.status ?? 'new',
        repoName: repo.name,
        branchName: workspace.branch,
      }
    }
  }

  const project = projects.find((p) => p.id === activeProjectId)
  if (project) {
    return { kind: 'project', projectName: project.name }
  }

  return { kind: 'empty' }
}
