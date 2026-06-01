import { nanoid } from 'nanoid'
import type { ScenarioDataset } from './index'

export const emptyDataset: ScenarioDataset = {
  repos: () => [],
  projects: () => [],
  workspace: () => undefined,
  createWorkspace: (repoId, branch) => ({ id: nanoid(), repoId, branch }),
  fileTree: () => ({ name: '', path: '', isDir: true } as any),
  fileContent: () => '',
  branchDiff: () => ({
    commitHash: '0000000000000000000000000000000000000000',
    commitMessage: '',
    files: [],
    totalFiles: 0,
    totalAdditions: 0,
    totalDeletions: 0,
  }),
  branchThreads: () => [],
  branchDescription: () => '',
  branchChats: () => [],
  gitLog: () => [],
  gitStatus: () => ({ branch: 'main', ahead: 0, behind: 0, files: [] }),
  gitBranches: () => [],
  markdownTurns: () => [],
}
