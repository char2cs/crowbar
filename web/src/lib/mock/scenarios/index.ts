import type { Repo } from '@/lib/store/sidebar'
import type { Project, WorkspacePayload } from '@/lib/types'
import type { FileNode } from '@/lib/mock/files'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { ReviewThread } from '@/features/branch-review/types/review-types'
import type { BranchReviewChat } from '@/lib/mock/branch-diff'
import type { Commit, GitStatus, Branch } from '@/lib/mock/git-data'
import type { MarkdownTurn } from '@/features/markdown-chat/types'

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
  markdownTurns: (wsId: string, stepId: string) => MarkdownTurn[]
}

let _extreme: ScenarioDataset | null = null
let _normal: ScenarioDataset | null = null
let _empty: ScenarioDataset | null = null

export function getDataForScenario(scenario: string): ScenarioDataset {
  switch (scenario) {
    case 'extreme': {
      if (!_extreme) _extreme = require('./extreme').extremeDataset
      return _extreme!
    }
    case 'empty': {
      if (!_empty) _empty = require('./empty').emptyDataset
      return _empty!
    }
    default: {
      if (!_normal) _normal = require('./normal').normalDataset
      return _normal!
    }
  }
}
