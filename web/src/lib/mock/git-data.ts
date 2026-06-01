export type FileStatus = 'modified' | 'added' | 'deleted' | 'renamed' | 'untracked'

export interface GitFile {
  path: string
  status: FileStatus
  staged: boolean
}

export interface GitStatus {
  branch: string
  ahead: number
  behind: number
  files: GitFile[]
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

const REPO_GIT_STATUS: Record<string, GitStatus> = {
  crowbar: {
    branch: 'feature/app-design',
    ahead: 3, behind: 0,
    files: [
      { path: 'web/src/features/branch-review/components/git-tab.tsx', status: 'modified', staged: true },
      { path: 'web/src/lib/mock/git-data.ts', status: 'modified', staged: true },
      { path: 'api/internal/fixtures/git-log.json', status: 'added', staged: true },
      { path: 'web/src/features/git/api/git-commits-api.ts', status: 'modified', staged: false },
      { path: 'web/src/features/git/api/git-status-api.ts', status: 'modified', staged: false },
      { path: 'web/src/features/settings/components/tabs/developer-settings.tsx', status: 'added', staged: false },
      { path: 'web/src/mocks/handlers/git.ts', status: 'modified', staged: false },
    ],
  },
  'quiver-core': {
    branch: 'feature/oauth2',
    ahead: 5, behind: 1,
    files: [
      { path: 'src/auth/oauth2.ts', status: 'added', staged: true },
      { path: 'src/auth/jwt.ts', status: 'modified', staged: true },
      { path: 'src/db/migrations/002_add_oauth.sql', status: 'added', staged: true },
      { path: 'src/lib/redis.ts', status: 'added', staged: false },
      { path: 'tests/auth.test.ts', status: 'modified', staged: false },
      { path: 'src/api/routes.ts', status: 'modified', staged: false },
    ],
  },
  'quiver-desktop': {
    branch: 'feature/quiver-shell',
    ahead: 8, behind: 0,
    files: [
      { path: 'src-tauri/src/shell/mod.rs', status: 'added', staged: true },
      { path: 'src-tauri/src/shell/pty.rs', status: 'added', staged: true },
      { path: 'src/app/shell/ShellWindow.tsx', status: 'added', staged: true },
      { path: 'src/app/shell/TerminalPane.tsx', status: 'added', staged: true },
      { path: 'src/bridge/shell.ts', status: 'added', staged: false },
      { path: 'src-tauri/Cargo.toml', status: 'modified', staged: false },
      { path: 'src-tauri/src/fs/watcher.rs', status: 'modified', staged: false },
    ],
  },
  'quiver-cloud': {
    branch: 'feature/multi-tenant',
    ahead: 2, behind: 3,
    files: [
      { path: 'k8s/apps/multi-tenant.yaml', status: 'added', staged: true },
      { path: 'terraform/modules/eks/node_groups.tf', status: 'added', staged: true },
      { path: 'terraform/environments/staging/main.tf', status: 'modified', staged: false },
      { path: 'k8s/base/rbac.yaml', status: 'modified', staged: false },
      { path: '.github/workflows/deploy.yml', status: 'modified', staged: false },
    ],
  },
}

export function getMockGitStatus(repoPath: string): GitStatus {
  const repoId = repoPath.split('/').filter(Boolean).pop() ?? 'crowbar'
  return REPO_GIT_STATUS[repoId] ?? REPO_GIT_STATUS['crowbar']
}

const COMMIT_MESSAGES = [
  'feat: add payment processing module',
  'fix: resolve null pointer in auth middleware',
  'refactor: extract database connection pool',
  'chore: update dependencies to latest versions',
  'docs: add API endpoint documentation',
  'test: add integration tests for user service',
  'perf: optimize query execution with indexes',
  'feat: implement real-time notification system',
  'fix: handle edge case in date parsing',
  "Merge branch 'feature/payments' into develop",
  'feat: add CSV export functionality',
  'fix: correct timezone handling in scheduler',
  'chore: add CI/CD pipeline configuration',
  'style: apply linting rules across codebase',
  'feat: add dashboard analytics widgets',
  'fix: resolve memory leak in WebSocket handler',
  'refactor: split monolithic user service into microservices',
  'test: add unit tests for billing calculation module',
  'feat: implement OAuth2 authentication flow',
  'chore: upgrade Go to v1.25',
  'docs: update API reference with new endpoints',
  'fix: correct rounding error in invoice total calculation',
  'feat: add email notification system with templates',
  'refactor: migrate to TypeScript strict mode',
  'perf: add Redis caching for hot database queries',
  'feat: add file upload with S3 presigned URLs',
  'fix: handle concurrent write race condition in store',
  'chore: configure Dependabot for automatic security updates',
  'feat: add webhook delivery with retry logic',
  'test: add E2E tests for checkout flow',
  'fix: correct pagination offset in list endpoints',
  'refactor: consolidate error handling middleware',
  'feat: add rate limiting per API key',
  'docs: add architecture decision records',
  'fix: resolve CORS preflight issue for mobile clients',
  'perf: lazy-load heavy chart components',
  'feat: implement GraphQL subscriptions for live updates',
  'chore: migrate from npm to pnpm',
  'fix: handle empty state in workspace tree render',
  'feat: add keyboard shortcut for quick file search',
]

const AUTHORS = [
  'Mateo Urrutia',
  'Claude Agent',
  'Alice Chen',
  'Bob Rodriguez',
  'Dependabot[bot]',
  'Sofia Andersson',
]

function generateHash(seed: number): string {
  let h = seed * 1_234_567 + 987_654_321
  h = ((h ^ (h >>> 16)) * 0x45d9f3b) & 0x7fffffff
  return (h + seed * 0xdeadbeef).toString(16).padStart(10, '0').repeat(4).slice(0, 40)
}

export function getMockCommitHistory(_repoPath: string): Commit[] {
  return Array.from({ length: 200 }, (_, i) => {
    const hash = generateHash(i + 1)
    const hoursAgo = i * 4
    const daysAgo = Math.floor(hoursAgo / 24)
    const date = daysAgo === 0
      ? `${hoursAgo % 24 || 1} hours ago`
      : daysAgo === 1
        ? '1 day ago'
        : `${daysAgo} days ago`
    return {
      hash,
      shortHash: hash.slice(0, 7),
      message: COMMIT_MESSAGES[i % COMMIT_MESSAGES.length],
      author: AUTHORS[i % AUTHORS.length],
      date,
    }
  })
}

const BRANCH_PREFIXES = ['feature/', 'fix/', 'chore/', 'refactor/', 'hotfix/', 'release/']
const BRANCH_TOPICS = [
  'payment-flow', 'user-auth', 'data-migration', 'api-v2', 'dashboard-redesign',
  'ci-pipeline', 'test-coverage', 'performance', 'security-patch', 'mobile-app',
  'websocket-hub', 'file-explorer', 'git-integration', 'settings-panel', 'dark-mode',
]

const REPO_CURRENT_BRANCH: Record<string, string> = {
  crowbar: 'feature/app-design',
  'quiver-core': 'feature/oauth2',
  'quiver-desktop': 'feature/quiver-shell',
  'quiver-cloud': 'feature/multi-tenant',
}

export function getMockBranches(repoPath: string): Branch[] {
  const repoId = repoPath.split('/').filter(Boolean).pop() ?? 'crowbar'
  const currentBranch = REPO_CURRENT_BRANCH[repoId] ?? 'develop'

  const base: Branch[] = [
    { name: 'main', isCurrent: false, isRemote: false, lastCommit: COMMIT_MESSAGES[3] },
    { name: 'develop', isCurrent: false, isRemote: false, lastCommit: COMMIT_MESSAGES[9] },
    { name: currentBranch, isCurrent: true, isRemote: false, lastCommit: COMMIT_MESSAGES[0] },
    { name: 'origin/main', isCurrent: false, isRemote: true, lastCommit: COMMIT_MESSAGES[3] },
    { name: 'origin/develop', isCurrent: false, isRemote: true, lastCommit: COMMIT_MESSAGES[9] },
  ]

  for (let i = 0; i < 25; i++) {
    const prefix = BRANCH_PREFIXES[i % BRANCH_PREFIXES.length]
    const topic = BRANCH_TOPICS[i % BRANCH_TOPICS.length]
    base.push({
      name: `${prefix}${topic}-${i + 1}`,
      isCurrent: false,
      isRemote: i % 3 === 0,
      lastCommit: COMMIT_MESSAGES[(i * 3) % COMMIT_MESSAGES.length],
    })
  }

  return base
}
