// web/src/lib/mock/git-data.ts

export type FileStatus = 'modified' | 'added' | 'deleted' | 'renamed'

export interface GitFile {
  path: string
  status: FileStatus
}

export interface GitStatus {
  staged: GitFile[]
  unstaged: GitFile[]
  branch: string
}

export interface Commit {
  hash: string
  shortHash: string
  message: string
  author: string
  date: string
}

export interface Branch {
  name: string
  isCurrent: boolean
  isRemote: boolean
  lastCommit?: string
}

export function getMockGitStatus(_repoPath: string): GitStatus {
  return {
    branch: 'feat/payment-flow',
    staged: [
      { path: 'src/payment/PaymentError.ts', status: 'added' },
    ],
    unstaged: [
      { path: 'src/payment/PaymentService.ts', status: 'modified' },
      { path: 'src/payment/payment.test.ts', status: 'modified' },
    ],
  }
}

export function getMockCommitHistory(_repoPath: string): Commit[] {
  return [
    {
      hash: 'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2',
      shortHash: 'a1b2c3d',
      message: 'feat: add PaymentError with typed codes',
      author: 'Mateo Urrutia',
      date: '2 hours ago',
    },
    {
      hash: 'b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3',
      shortHash: 'b2c3d4e',
      message: 'feat: scaffold PaymentService class',
      author: 'Mateo Urrutia',
      date: '4 hours ago',
    },
    {
      hash: 'c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4',
      shortHash: 'c3d4e5f',
      message: 'chore: add payment module structure',
      author: 'Mateo Urrutia',
      date: '1 day ago',
    },
    {
      hash: 'd4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5',
      shortHash: 'd4e5f6a',
      message: 'feat: initial project scaffold',
      author: 'Mateo Urrutia',
      date: '3 days ago',
    },
    {
      hash: 'e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6',
      shortHash: 'e5f6a1b',
      message: 'chore: add README and license',
      author: 'Mateo Urrutia',
      date: '5 days ago',
    },
  ]
}

export function getMockBranches(_repoPath: string): Branch[] {
  return [
    { name: 'feat/payment-flow', isCurrent: true, isRemote: false, lastCommit: 'feat: add PaymentError with typed codes' },
    { name: 'main', isCurrent: false, isRemote: false, lastCommit: 'chore: add README and license' },
    { name: 'fix/auth-bug', isCurrent: false, isRemote: false, lastCommit: 'fix: resolve token expiry edge case' },
    { name: 'origin/main', isCurrent: false, isRemote: true, lastCommit: 'chore: add README and license' },
  ]
}
