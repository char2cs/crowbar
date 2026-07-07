import type { Repo } from '@/lib/store/sidebar'
import type { Project, WorkspacePayload } from '@/lib/types'
import type { FileNode } from '@/lib/mock/files'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { ReviewThread } from '@/features/workspace/stores/slices/branch-review-slice'
import type { BranchReviewChat } from '@/lib/mock/branch-diff'
import type { Commit, GitStatus, Branch } from '@/lib/mock/git-data'

export interface ScenarioDataset {
  repos: () => Repo[]
  projects: () => Project[]
  workspace: (wsId: string) => WorkspacePayload | undefined
  createWorkspace: (repoId: string, branch: string) => WorkspacePayload
  fileTree: (repoPath: string) => FileNode
  fileContent: (path: string) => string
  branchDiff: (wsId: string) => MultiFileDiff
  branchThreads: (wsId: string) => ReviewThread[]
  branchDescription: (wsId: string) => string
  branchChats: (wsId: string) => BranchReviewChat[]
  gitLog: (repoPath: string) => Commit[]
  gitStatus: (repoPath: string) => GitStatus
  gitBranches: (repoPath: string) => Branch[]
}

import { normalDataset } from './normal'
import { extremeDataset } from './extreme'
import { emptyDataset } from './empty'

export function getDataForScenario(scenario: string): ScenarioDataset {
  switch (scenario) {
    case 'extreme':
      return extremeDataset
    case 'empty':
      return emptyDataset
    default:
      return normalDataset
  }
}
