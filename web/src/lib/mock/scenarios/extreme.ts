import { nanoid } from 'nanoid'
import type { ScenarioDataset } from './index'
import type { ProjectChat } from '@/lib/store/sidebar'
import type {
  ReviewThread,
  ReviewMessage,
} from '@/features/workspace/stores/slices/branch-review-slice'
import type { BranchReviewChat } from '@/lib/mock/branch-diff'
import type { Commit, GitStatus } from '@/lib/mock/git-data'
import { generateLargeFileDiff } from '@/lib/mock/branch-diff'
import { getMockFileTree, getMockFileContent } from '@/lib/mock/files'
import type { FileNode } from '@/lib/mock/files'
import type { GitDiffLine } from '@/features/git/types/git-types'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { MarkdownTurn } from '@/features/markdown-chat/types'

// ─── Helpers ──────────────────────────────────────────────────────────────────

function genHash(seed: number): string {
  let h = seed * 1_234_567 + 987_654_321
  h = ((h ^ (h >>> 16)) * 0x45d9f3b) & 0x7fffffff
  return (h + seed * 0xdeadbeef).toString(16).padStart(10, '0').repeat(4).slice(0, 40)
}

function msg(
  id: string,
  author: string | null,
  isAgent: boolean,
  body: string,
  ts: string,
): ReviewMessage {
  return { id, author, isAgent, body, createdAt: ts }
}

// ─── 8 realistic message-pool templates for thread generation ─────────────────

const MESSAGE_POOLS: Array<Array<[string | null, boolean, string]>> = [
  [
    [
      null,
      false,
      "This function is O(n²) in the worst case because of the nested loop. With the data sizes we're expecting this will be noticeably slow.",
    ],
    [
      'Aria',
      true,
      'Confirmed — if `items` is large this is a real perf issue. Build a `Map<id, item>` in a single pass first, then the inner lookup becomes O(1). Overall drops to O(n).',
    ],
    [null, false, 'Refactored to use a Map. Benchmarks show 40× speedup on 10k items.'],
    [
      'Aria',
      true,
      'The Map is rebuilt on every call. If this is hot path, memoize it or use `useMemo`.',
    ],
    [null, false, 'Added `useMemo` with items array as dependency.'],
  ],
  [
    [
      'Morgan',
      false,
      'Nit: the variable name `data` is too generic. Given the context it should be `userProfile`.',
    ],
    [null, false, 'Renamed throughout.'],
    [
      'Aria',
      true,
      "`profilePayload` implies it's being sent somewhere. If it's received, `profileRecord` reads more naturally.",
    ],
    [null, false, 'Changed to `profileRecord`.'],
  ],
  [
    [
      null,
      false,
      'Should this be `null` or `undefined`? The rest of the codebase uses `undefined` for optional values.',
    ],
    [
      'Aria',
      true,
      '`null` is correct here — the backend explicitly returns `null` when absent (not omitting the key). Worth a comment explaining the distinction.',
    ],
    [null, false, 'Added comment: `// null = backend explicitly absent, not unset`.'],
    [
      'Morgan',
      false,
      'Could we encode this at the type level? A `type ExplicitNull<T> = T | null` with JSDoc.',
    ],
    [
      'Aria',
      true,
      'Good idea but overkill for now. If this pattern appears in 3+ places, introduce the type alias.',
    ],
  ],
  [
    [
      'Aria',
      true,
      "This `useEffect` has `user` in the dep array but `user.id` is what's actually used inside. The effect re-runs on any identity change even if id didn't change.",
    ],
    [null, false, 'Fixed — changed `[user]` to `[user.id]`.'],
    [
      'Aria',
      true,
      'If `user` can be null here, `user.id` would throw. Add a null guard: `[user?.id]`.',
    ],
    [null, false, 'Added null guard and wrapped body in `if (!user) return`.'],
  ],
  [
    [null, false, 'Is this error being swallowed intentionally? The `catch` block is empty.'],
    [
      'Aria',
      true,
      'Silent failures make debugging very hard. At minimum log in dev. Better: propagate to UI via error state.',
    ],
    [
      'Morgan',
      false,
      'We have a `reportError()` util that sends to Sentry in prod and logs in dev.',
    ],
    [
      null,
      false,
      'Added `reportError(err)` in the catch. Also added error state that shows a toast.',
    ],
    [
      'Aria',
      true,
      "Make sure the toast message is user-friendly and doesn't expose internal details like stack traces.",
    ],
    [null, false, 'The toast just says "Something went wrong. Please try again."'],
  ],
  [
    [
      'Aria',
      true,
      'This component re-renders every time the parent re-renders because the callback is defined inline. Wrap it in `useCallback`.',
    ],
    [null, false, 'Wrapped in `useCallback`. Dependencies are `[dispatch, itemId]`.'],
    ['Aria', true, 'Check that `dispatch` is stable — from Zustand it should be, but verify.'],
    [
      'Morgan',
      false,
      '`dispatch` from Zustand is guaranteed stable — defined once in store creation.',
    ],
  ],
  [
    [
      null,
      false,
      'The regex will fail for email addresses with subdomains (e.g. `user@mail.company.co.uk`).',
    ],
    [
      'Aria',
      true,
      'Correct catch. Use `/^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$/` — it allows any non-whitespace/@ with at least one dot after @.',
    ],
    [null, false, 'Updated regex. Added test case for subdomain emails.'],
    ['Aria', true, 'Also add test cases for `user+tag@example.com` and `user@localhost`.'],
  ],
  [
    [
      'Morgan',
      false,
      "This API call doesn't handle timeouts. On a slow connection the user will see a spinner forever.",
    ],
    [
      'Aria',
      true,
      'Add an `AbortController` with a timeout. Store the timeout id and clear it in `finally`.',
    ],
    [null, false, 'Implemented with `AbortController`. Timeout set to 10s.'],
    ['Aria', true, 'Make the timeout configurable via a constant or env var.'],
    [
      null,
      false,
      'Added `API_TIMEOUT_MS` constant, defaults to 10000, overridable via `VITE_API_TIMEOUT`.',
    ],
  ],
]

const EXTREME_FILES = [
  'src/lib/query/query-layer.ts',
  'src/lib/query/cache-manager.ts',
  'src/api/auth/session.ts',
  'src/features/dashboard/components/ActivityFeed.tsx',
  'src/features/editor/stores/buffer-slice.ts',
  'src/lib/ws/channel.ts',
  'src/middleware/rate-limit.ts',
  'src/lib/crypto/token.ts',
  'src/features/git/components/diff-viewer.tsx',
  'src/api/v2/workspaces.ts',
  'src/features/billing/stripe-webhook.ts',
  'src/lib/db/connection-pool.ts',
]

function genThreads(wsId: string, count: number): ReviewThread[] {
  return Array.from({ length: count }, (_, i) => {
    const pool = MESSAGE_POOLS[i % MESSAGE_POOLS.length]
    const lineNum = ((i * 17 + 3) % 120) + 1
    const file = EXTREME_FILES[i % EXTREME_FILES.length]
    const messages = pool.map(([author, isAgent, body], j) =>
      msg(
        `msg-${wsId}-${i}-${j}`,
        author,
        isAgent,
        body,
        `2026-05-${String(15 + (i % 15)).padStart(2, '0')}T${String(8 + (j % 12)).padStart(2, '0')}:${String((i * 7 + j * 13) % 60).padStart(2, '0')}:00Z`,
      ),
    )
    return {
      id: `thread-${wsId}-${i}`,
      filePath: file,
      lineNumber: lineNum,
      side: i % 3 === 0 ? ('left' as const) : ('right' as const),
      messages,
      isResolved: i % 5 === 0,
    }
  })
}

function makeMassiveDiff(wsId: string, seed: number) {
  const isMassive = seed % 3 === 0
  if (isMassive) {
    // Genuinely large single-file diffs — the whole point of the "extreme"
    // scenario is to stress the diff viewer. (It used to be capped at 260
    // lines, so the renderer was never actually exercised.) The viewer
    // virtualises lines and highlights only the visible window, so even these
    // render in ~150ms regardless of file size.
    const big = generateLargeFileDiff('src/lib/query/query-layer.ts', 12000)
    const medium = generateLargeFileDiff('src/lib/query/cache-manager.ts', 6000)
    return {
      commitHash: genHash(seed),
      commitMessage: `refactor: major rewrite of ${wsId} data layer`,
      files: [
        {
          file_path: 'src/lib/query/query-layer.ts',
          is_new: false,
          is_deleted: false,
          is_renamed: false,
          additions: 65234,
          deletions: 44821,
          lines: big,
        },
        {
          file_path: 'src/lib/query/cache-manager.ts',
          is_new: false,
          is_deleted: false,
          is_renamed: false,
          additions: 18248,
          deletions: 20101,
          lines: medium,
        },
        {
          file_path: 'src/lib/query/index.ts',
          is_new: false,
          is_deleted: false,
          is_renamed: false,
          additions: 12,
          deletions: 28,
          lines: [
            { line_type: 'header', content: '@@ -1,30 +1,14 @@' } as GitDiffLine,
            {
              line_type: 'removed',
              content: "export { legacyFetch } from './legacy'",
              old_line_number: 1,
            } as GitDiffLine,
            {
              line_type: 'added',
              content: "export { QueryLayer } from './query-layer'",
              new_line_number: 1,
            } as GitDiffLine,
          ],
        },
      ],
      totalFiles: 3,
      totalAdditions: 83494,
      totalDeletions: 64949,
    }
  }
  const fileCount = 20 + (seed % 20)
  const files = Array.from({ length: fileCount }, (_, fi) => {
    const additions = 10 + ((fi * seed) % 80)
    const deletions = fi % 4 === 0 ? 0 : 5 + ((fi * seed) % 30)
    return {
      file_path: EXTREME_FILES[fi % EXTREME_FILES.length],
      is_new: fi < 3,
      is_deleted: false,
      is_renamed: false,
      additions,
      deletions,
      lines: [
        { line_type: 'header', content: `@@ -1,${deletions} +1,${additions} @@` } as GitDiffLine,
        ...Array.from({ length: Math.min(additions, 6) }, (_, li) => ({
          line_type: 'added' as const,
          content: `  // implementation line ${li + 1}`,
          new_line_number: li + 1,
        })),
      ],
    }
  })
  return {
    commitHash: genHash(seed),
    commitMessage: `feat(${wsId}): implement feature with ${fileCount} files changed`,
    files,
    totalFiles: fileCount,
    totalAdditions: files.reduce((s, f) => s + f.additions, 0),
    totalDeletions: files.reduce((s, f) => s + f.deletions, 0),
  }
}

const EXTREME_MSGS = [
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
  'refactor: split monolithic user service',
  'test: add unit tests for billing module',
  'feat: implement OAuth2 authentication flow',
  'chore: upgrade Go to v1.25',
  'security: patch XSS vulnerability in markdown renderer',
  'feat: implement multi-region failover',
  'fix: race condition in session token refresh',
  'perf: reduce bundle size by 34%',
  'feat: add audit log for compliance requirements',
  'refactor: replace redux with zustand',
  'fix: handle concurrent write race condition',
  'chore: configure Dependabot',
  'feat: add webhook delivery with retry logic',
  'test: add E2E tests for checkout flow',
  'fix: correct pagination offset in list endpoints',
  'refactor: consolidate error handling',
  'feat: add rate limiting per API key',
  'docs: add architecture decision records',
  'fix: resolve CORS preflight issue',
  'perf: lazy-load heavy chart components',
  'feat: implement GraphQL subscriptions',
  'chore: migrate from npm to pnpm',
]

const AUTHORS_EX = [
  'Mateo Urrutia',
  'Claude Agent',
  'Alice Chen',
  'Bob Rodriguez',
  'Dependabot[bot]',
  'Sofia Andersson',
]

function genCommits(count: number): Commit[] {
  return Array.from({ length: count }, (_, i) => {
    const hash = genHash(i + 100)
    const hoursAgo = i * 2
    const daysAgo = Math.floor(hoursAgo / 24)
    return {
      hash,
      shortHash: hash.slice(0, 7),
      message: EXTREME_MSGS[i % EXTREME_MSGS.length],
      author: AUTHORS_EX[i % AUTHORS_EX.length],
      date:
        daysAgo === 0
          ? `${hoursAgo % 24 || 1}h ago`
          : daysAgo === 1
            ? '1 day ago'
            : `${daysAgo} days ago`,
    }
  })
}

function genChats(wsId: string, count: number): BranchReviewChat[] {
  const titles = [
    'Architecture review',
    'Performance bottlenecks',
    'Security audit findings',
    'Database migration plan',
    'API versioning strategy',
    'Test coverage gaps',
    'Cache invalidation design',
    'Error handling patterns',
    'TypeScript migration',
    'WebSocket reconnect logic',
  ]
  return Array.from({ length: count }, (_, i) => ({
    id: `chat-${wsId}-${i}`,
    title: titles[i % titles.length],
    age: i === 0 ? '5m' : i < 3 ? `${i * 2}h` : `${i} days`,
    isActive: i < 2,
  }))
}

// ─── Repos ────────────────────────────────────────────────────────────────────

const EXTREME_REPOS = [
  {
    id: 'crowbar',
    name: 'crowbar',
    avatarLabel: 'C',
    avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ex-cr-dev', branch: 'develop', status: 'locked' as const, age: '—' },
      {
        id: 'ex-cr-1',
        branch: 'feature/app-design',
        parentId: 'ex-cr-dev',
        status: 'pr-open' as const,
        added: 5672,
        age: '16h ago',
      },
      {
        id: 'ex-cr-2',
        branch: 'enhancement/scaffold',
        parentId: 'ex-cr-1',
        status: 'new' as const, working: true,
        added: 22892,
        age: '3d ago',
      },
      {
        id: 'ex-cr-3',
        branch: 'fix/toolbar-crash',
        parentId: 'ex-cr-1',
        status: 'new' as const,
        age: 'just now',
      },
      {
        id: 'ex-cr-4',
        branch: 'feature/api-backend',
        parentId: 'ex-cr-dev',
        status: 'pr-merged' as const,
        added: 27347,
        deleted: 455,
        age: '1d ago',
      },
      {
        id: 'ex-cr-5',
        branch: 'feature/ws-channels',
        parentId: 'ex-cr-dev',
        status: 'pr-open' as const,
        added: 8841,
        deleted: 203,
        age: '2d ago',
      },
      {
        id: 'ex-cr-6',
        branch: 'fix/ws-reconnect',
        parentId: 'ex-cr-5',
        status: 'new' as const,
        added: 47,
        age: '2h ago',
      },
      {
        id: 'ex-cr-7',
        branch: 'refactor/query-layer',
        parentId: 'ex-cr-dev',
        status: 'new' as const, working: true,
        added: 103482,
        deleted: 88910,
        age: '5d ago',
      },
      {
        id: 'ex-cr-8',
        branch: 'refactor/query-layer-tests',
        parentId: 'ex-cr-7',
        status: 'new' as const,
        added: 4320,
        age: '4d ago',
      },
      {
        id: 'ex-cr-9',
        branch: 'refactor/cache-manager',
        parentId: 'ex-cr-7',
        status: 'pr-open' as const,
        added: 18248,
        deleted: 20101,
        age: '4d ago',
      },
      {
        id: 'ex-cr-10',
        branch: 'fix/cache-stampede',
        parentId: 'ex-cr-9',
        status: 'new' as const, working: true,
        added: 312,
        age: '3d ago',
      },
      {
        id: 'ex-cr-11',
        branch: 'feat/ai-review',
        parentId: 'ex-cr-dev',
        status: 'pr-open' as const,
        added: 9120,
        deleted: 340,
        age: '6d ago',
      },
      {
        id: 'ex-cr-12',
        branch: 'feat/terminal-v2',
        parentId: 'ex-cr-dev',
        status: 'new' as const, working: true,
        added: 15600,
        deleted: 2100,
        age: '7d ago',
      },
      {
        id: 'ex-cr-13',
        branch: 'fix/pty-resize',
        parentId: 'ex-cr-12',
        status: 'new' as const,
        added: 88,
        age: '1d ago',
      },
      {
        id: 'ex-cr-14',
        branch: 'chore/bump-deps',
        parentId: 'ex-cr-dev',
        status: 'pr-closed' as const,
        added: 312,
        deleted: 298,
        age: '6d ago',
      },
      {
        id: 'ex-cr-15',
        branch: 'hotfix/security-xss',
        parentId: 'ex-cr-dev',
        status: 'pr-open' as const,
        added: 23,
        deleted: 7,
        age: '2h ago',
      },
    ],
  },
  {
    id: 'quiver-core',
    name: 'quiver.core',
    avatarLabel: 'Q',
    avatarColor: 'bg-emerald-700',
    workspaces: [
      { id: 'ex-qc-dev', branch: 'develop', status: 'locked' as const, age: '—' },
      {
        id: 'ex-qc-1',
        branch: 'feature/oauth2',
        parentId: 'ex-qc-dev',
        status: 'pr-open' as const,
        added: 4521,
        deleted: 89,
        age: '1d ago',
      },
      {
        id: 'ex-qc-2',
        branch: 'fix/token-expiry',
        parentId: 'ex-qc-1',
        status: 'new' as const,
        added: 47,
        age: 'just now',
      },
      {
        id: 'ex-qc-3',
        branch: 'perf/redis-cache',
        parentId: 'ex-qc-dev',
        status: 'new' as const, working: true,
        added: 1823,
        deleted: 402,
        age: '12h ago',
      },
      {
        id: 'ex-qc-4',
        branch: 'feat/graphql-subscriptions',
        parentId: 'ex-qc-dev',
        status: 'pr-open' as const,
        added: 12400,
        deleted: 800,
        age: '3d ago',
      },
      {
        id: 'ex-qc-5',
        branch: 'fix/graphql-n-plus-one',
        parentId: 'ex-qc-4',
        status: 'new' as const, working: true,
        added: 340,
        deleted: 120,
        age: '2d ago',
      },
      {
        id: 'ex-qc-6',
        branch: 'feat/audit-log',
        parentId: 'ex-qc-dev',
        status: 'pr-open' as const,
        added: 6700,
        deleted: 200,
        age: '5d ago',
      },
      {
        id: 'ex-qc-7',
        branch: 'refactor/db-pool',
        parentId: 'ex-qc-dev',
        status: 'pr-merged' as const,
        added: 890,
        deleted: 1200,
        age: '8d ago',
      },
      {
        id: 'ex-qc-8',
        branch: 'feat/multi-region',
        parentId: 'ex-qc-dev',
        status: 'new' as const, working: true,
        added: 45000,
        deleted: 8200,
        age: '10d ago',
      },
      {
        id: 'ex-qc-9',
        branch: 'fix/region-failover',
        parentId: 'ex-qc-8',
        status: 'new' as const,
        added: 230,
        age: '9d ago',
      },
      {
        id: 'ex-qc-10',
        branch: 'chore/upgrade-postgres',
        parentId: 'ex-qc-dev',
        status: 'pr-closed' as const,
        added: 45,
        deleted: 30,
        age: '12d ago',
      },
      {
        id: 'ex-qc-11',
        branch: 'security/patch-xss',
        parentId: 'ex-qc-dev',
        status: 'pr-open' as const,
        added: 12,
        deleted: 8,
        age: '4h ago',
      },
    ],
  },
  {
    id: 'quiver-desktop',
    name: 'quiver.desktop',
    avatarLabel: 'Q',
    avatarColor: 'bg-orange-700',
    workspaces: [
      { id: 'ex-qd-dev', branch: 'develop', status: 'locked' as const, age: '—' },
      {
        id: 'ex-qd-1',
        branch: 'feature/quiver-shell',
        parentId: 'ex-qd-dev',
        status: 'pr-open' as const,
        added: 13485,
        deleted: 69,
        age: '3d ago',
      },
      {
        id: 'ex-qd-2',
        branch: 'fix/pty-encoding',
        parentId: 'ex-qd-1',
        status: 'new' as const,
        added: 34,
        age: '1d ago',
      },
      {
        id: 'ex-qd-3',
        branch: 'feat/native-file-picker',
        parentId: 'ex-qd-dev',
        status: 'new' as const, working: true,
        added: 2341,
        age: '8h ago',
      },
      {
        id: 'ex-qd-4',
        branch: 'fix/startup-crash',
        parentId: 'ex-qd-dev',
        status: 'pr-merged' as const,
        added: 23,
        deleted: 7,
        age: '4d ago',
      },
      {
        id: 'ex-qd-5',
        branch: 'feat/auto-update',
        parentId: 'ex-qd-dev',
        status: 'pr-open' as const,
        added: 3400,
        deleted: 890,
        age: '6d ago',
      },
      {
        id: 'ex-qd-6',
        branch: 'fix/update-rollback',
        parentId: 'ex-qd-5',
        status: 'new' as const, working: true,
        added: 120,
        deleted: 45,
        age: '5d ago',
      },
      {
        id: 'ex-qd-7',
        branch: 'feat/plugin-system',
        parentId: 'ex-qd-dev',
        status: 'pr-open' as const,
        added: 8900,
        deleted: 450,
        age: '7d ago',
      },
      {
        id: 'ex-qd-8',
        branch: 'chore/tauri-v2-migration',
        parentId: 'ex-qd-dev',
        status: 'new' as const, working: true,
        added: 34000,
        deleted: 29000,
        age: '9d ago',
      },
      {
        id: 'ex-qd-9',
        branch: 'fix/macos-permissions',
        parentId: 'ex-qd-dev',
        status: 'new' as const,
        added: 67,
        age: '6h ago',
      },
    ],
  },
  {
    id: 'quiver-cloud',
    name: 'quiver.cloud',
    avatarLabel: 'Q',
    avatarColor: 'bg-sky-700',
    workspaces: [
      { id: 'ex-qcl-dev', branch: 'develop', status: 'locked' as const, age: '—' },
      {
        id: 'ex-qcl-1',
        branch: 'feature/multi-tenant',
        parentId: 'ex-qcl-dev',
        status: 'pr-open' as const,
        added: 31204,
        deleted: 1823,
        age: '2d ago',
      },
      {
        id: 'ex-qcl-2',
        branch: 'feat/s3-presign',
        parentId: 'ex-qcl-dev',
        status: 'new' as const,
        added: 892,
        age: '4h ago',
      },
      {
        id: 'ex-qcl-3',
        branch: 'chore/infra-terraform',
        parentId: 'ex-qcl-dev',
        status: 'pr-merged' as const,
        added: 4812,
        deleted: 3201,
        age: '7d ago',
      },
      {
        id: 'ex-qcl-4',
        branch: 'feat/cdn-integration',
        parentId: 'ex-qcl-dev',
        status: 'new' as const, working: true,
        added: 2300,
        deleted: 400,
        age: '5d ago',
      },
      {
        id: 'ex-qcl-5',
        branch: 'fix/cdn-cache-invalidation',
        parentId: 'ex-qcl-4',
        status: 'new' as const,
        added: 45,
        age: '4d ago',
      },
      {
        id: 'ex-qcl-6',
        branch: 'feat/usage-billing',
        parentId: 'ex-qcl-dev',
        status: 'pr-open' as const,
        added: 7800,
        deleted: 1200,
        age: '8d ago',
      },
      {
        id: 'ex-qcl-7',
        branch: 'feat/sso-saml',
        parentId: 'ex-qcl-dev',
        status: 'new' as const, working: true,
        added: 12000,
        deleted: 2300,
        age: '10d ago',
      },
      {
        id: 'ex-qcl-8',
        branch: 'fix/saml-assertion',
        parentId: 'ex-qcl-7',
        status: 'new' as const,
        added: 89,
        age: '9d ago',
      },
      {
        id: 'ex-qcl-9',
        branch: 'chore/k8s-upgrade',
        parentId: 'ex-qcl-dev',
        status: 'pr-closed' as const,
        added: 340,
        deleted: 280,
        age: '14d ago',
      },
    ],
  },
]

// ─── Workspace store ──────────────────────────────────────────────────────────

const wsStore = new Map<string, { id: string; projectId: string; repoId: string; branch: string }>()
for (const repo of EXTREME_REPOS) {
  for (const ws of repo.workspaces) {
    wsStore.set(ws.id, { id: ws.id, projectId: '', repoId: repo.id, branch: ws.branch })
  }
}

// ─── Git statuses ─────────────────────────────────────────────────────────────

const GIT_STATUSES: Record<string, GitStatus> = {
  crowbar: {
    branch: 'refactor/query-layer',
    ahead: 47,
    behind: 3,
    files: [
      { path: 'src/lib/query/query-layer.ts', status: 'modified' as const, staged: true },
      { path: 'src/lib/query/cache-manager.ts', status: 'modified' as const, staged: true },
      { path: 'src/lib/query/legacy-fetch.ts', status: 'deleted' as const, staged: true },
      { path: 'src/lib/query/index.ts', status: 'modified' as const, staged: false },
      { path: 'src/features/dashboard/page.tsx', status: 'modified' as const, staged: false },
    ],
  },
  'quiver-core': {
    branch: 'feat/multi-region',
    ahead: 23,
    behind: 1,
    files: [
      { path: 'k8s/apps/multi-tenant.yaml', status: 'added' as const, staged: true },
      { path: 'src/api/region-router.ts', status: 'added' as const, staged: false },
    ],
  },
  'quiver-desktop': {
    branch: 'chore/tauri-v2-migration',
    ahead: 82,
    behind: 0,
    files: [
      { path: 'src-tauri/src/main.rs', status: 'modified' as const, staged: true },
      { path: 'src-tauri/Cargo.toml', status: 'modified' as const, staged: false },
    ],
  },
  'quiver-cloud': {
    branch: 'feature/multi-tenant',
    ahead: 34,
    behind: 5,
    files: [
      { path: 'k8s/base/rbac.yaml', status: 'modified' as const, staged: true },
      { path: 'src/middleware/tenant.ts', status: 'added' as const, staged: false },
    ],
  },
}

// ─── Dataset ──────────────────────────────────────────────────────────────────

const commitCache = genCommits(500)

// ─── Markdown chat turns ────────────────────────────────────────────────────
// A rich, multi-turn technical conversation generated for any chat/workspace id
// so opened conversations are never empty in the extreme scenario.

const CHAT_EXCHANGES: Array<[string, string]> = [
  [
    'The `QueryLayer.fetch` timeout leaks a timer when the fetcher resolves first. Under load we accumulate thousands of pending timers. How should I fix it?',
    'Store the timeout id and clear it in a `finally`:\n\n```ts\nconst id = setTimeout(() => controller.abort(), this.config.timeout)\ntry {\n  return await fetcher()\n} finally {\n  clearTimeout(id)\n}\n```\n\nThis guarantees the timer is cleared on both the success and error paths. The previous `Promise.race` left the loser dangling.',
  ],
  [
    'Should `CacheManager` handle cache stampede? Multiple concurrent requests for the same key before the first resolves.',
    'Yes — this is the **thundering herd** problem. Store a pending promise per key and return the same promise to all concurrent callers:\n\n```ts\nif (this.inflight.has(key)) return this.inflight.get(key)!\nconst p = fetcher().finally(() => this.inflight.delete(key))\nthis.inflight.set(key, p)\nreturn p\n```\n\nThis is **in-flight deduplication**. Given this refactor touches every data-fetching path, it should ship before production.',
  ],
  [
    'For React 18 concurrent mode, `setData` and `setStatus` in the `.then` chain trigger two renders. Worth optimizing?',
    'Merge them into a single state object or use `useReducer` — React 18 batches updates inside event handlers automatically, but not inside promise callbacks. A single `setState({ data, status })` collapses the two renders into one and removes the flicker.',
  ],
  [
    'The `destroy()` method clears the client but nothing stops a caller from using the instance afterward.',
    'Add an `isDestroyed` guard and **throw** (not return early) so callers don\'t silently get `undefined`:\n\n```ts\nif (this.isDestroyed) throw new Error("QueryLayer used after destroy()")\n```\n\nFailing loud here surfaces the lifecycle bug at the call site instead of much later.',
  ],
  [
    'There are 3 remaining callers of `legacyFetch` in `src/features/dashboard/`. Block the merge on migrating them?',
    "Yes — block it. Leaving `legacyFetch` callers alive after deleting the module breaks the build. Migrate all three to `QueryLayer.fetch`, run the full type-check, then merge. I'd also add a CI grep guard so no new `legacyFetch` imports sneak back in.",
  ],
]

function genTurns(wsId: string): MarkdownTurn[] {
  const turns: MarkdownTurn[] = []
  CHAT_EXCHANGES.forEach(([q, a], i) => {
    const baseMin = i * 7
    turns.push({
      id: `turn-${wsId}-${i}-u`,
      role: 'user',
      content: q,
      timestamp: `2026-05-28T${String(9 + Math.floor(baseMin / 60)).padStart(2, '0')}:${String(baseMin % 60).padStart(2, '0')}:00Z`,
      authorName: 'Mateo',
      widgets: [],
    })
    turns.push({
      id: `turn-${wsId}-${i}-a`,
      role: 'agent',
      content: a,
      timestamp: `2026-05-28T${String(9 + Math.floor((baseMin + 1) / 60)).padStart(2, '0')}:${String((baseMin + 1) % 60).padStart(2, '0')}:00Z`,
      authorName: 'Claude',
      model: 'Opus 4.8',
      widgets: [],
    })
  })
  return turns
}

export const extremeDataset: ScenarioDataset = {
  repos: () => EXTREME_REPOS,
  projects: () => [
    {
      id: 'crowbar',
      name: 'Crowbar',
      path: '/Users/mateo/dev/crowbar',
      lastActivity: new Date(Date.now() - 30 * 60 * 1000),
    },
    {
      id: 'quiver',
      name: 'Quiver',
      path: '/Users/mateo/dev/quiver',
      lastActivity: new Date(Date.now() - 2 * 60 * 60 * 1000),
    },
  ],
  workspace: (wsId) => wsStore.get(wsId),
  createWorkspace: (repoId, branch) => {
    const id = nanoid()
    const ws = { id, projectId: '', repoId, branch }
    wsStore.set(id, ws)
    return ws
  },
  fileTree: (repoPath) => getMockFileTree(repoPath) as unknown as FileNode,
  fileContent: (path) => getMockFileContent(path),
  branchDiff: (wsId) => {
    const seed = Array.from(wsId).reduce((s, c) => s + c.charCodeAt(0), 0)
    return makeMassiveDiff(wsId, seed) as unknown as MultiFileDiff
  },
  branchThreads: (wsId) => genThreads(wsId, 28),
  branchDescription: (wsId) =>
    `## ${wsId}\n\nThis branch contains significant changes across multiple subsystems. Review carefully before merging.\n\n- [ ] Tests passing\n- [ ] Performance benchmarks acceptable\n- [ ] Security review complete`,
  branchChats: (wsId) => genChats(wsId, 9),
  chats: (wsId) =>
    Array.from({ length: 15 }, (_, i) => ({
      id: `chat-ext-${wsId}-${i}`,
      wsId,
      title: `Chat ${i + 1}: ${['Refactor approach', 'API design', 'Testing strategy', 'Deployment plan', 'Performance fix'][i % 5]}`,
      age: `${i + 1}d`,
      parentId: i > 0 && i % 4 === 0 ? `chat-ext-${wsId}-${i - 1}` : undefined,
      status: i % 6 === 0 ? ('agent-running' as const) : ('idle' as const),
      type: 'chat' as const,
    })) satisfies ProjectChat[],
  gitLog: () => commitCache,
  gitStatus: (repoPath) => {
    const repoId = repoPath.split('/').filter(Boolean).pop() ?? 'crowbar'
    return GIT_STATUSES[repoId] ?? GIT_STATUSES['crowbar']
  },
  gitBranches: () =>
    Array.from({ length: 40 }, (_, i) => ({
      name: i === 0 ? 'main' : i === 1 ? 'develop' : `feature/branch-${i}`,
      isCurrent: i === 2,
      isRemote: i % 3 === 0,
    })),
  markdownTurns: (wsId) => genTurns(wsId),
}
