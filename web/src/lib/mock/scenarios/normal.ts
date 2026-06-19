import { nanoid } from 'nanoid'
import type { ScenarioDataset } from './index'
import type { ProjectChat } from '@/lib/store/sidebar'
import type {
  ReviewThread,
  ReviewMessage,
} from '@/features/workspace/stores/slices/branch-review-slice'
import type { BranchReviewChat } from '@/lib/mock/branch-diff'
import type { GitDiffLine } from '@/features/git/types/git-types'
import type { MultiFileDiff } from '@/features/git/types/git-diff-types'
import type { MarkdownTurn } from '@/features/markdown-chat/types'
import { getMockFileTree, getMockFileContent } from '@/lib/mock/files'
import type { FileNode } from '@/lib/mock/files'
import { getMockMarkdownTurns } from '@/lib/mock/markdown-chat'

// Fallback conversation for chats/workspaces without a specific fixture, so an
// opened conversation is never empty in the normal scenario.
function genNormalTurns(wsId: string): MarkdownTurn[] {
  return [
    {
      id: `turn-${wsId}-0-u`,
      role: 'user',
      content: 'How should we validate the email field before submitting the signup form?',
      timestamp: '2026-06-01T09:00:00Z',
      authorName: 'Mateo',
      widgets: [],
    },
    {
      id: `turn-${wsId}-0-a`,
      role: 'agent',
      content:
        'Validate client-side for instant feedback, then trust the server as the source of truth:\n\n```ts\nif (!isValidEmail(data.email)) {\n  setError("email", { message: "Invalid email address" })\n  return\n}\n```\n\nKeep the regex permissive (`/^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$/`) — strict RFC validation is a rabbit hole and rejects valid addresses. The API still re-validates, so this is purely UX.',
      timestamp: '2026-06-01T09:00:30Z',
      authorName: 'Claude',
      model: 'Opus 4.8',
      widgets: [],
    },
    {
      id: `turn-${wsId}-1-u`,
      role: 'user',
      content: 'Should the form show a single error or distinguish network vs validation failures?',
      timestamp: '2026-06-01T09:01:00Z',
      authorName: 'Mateo',
      widgets: [],
    },
    {
      id: `turn-${wsId}-1-a`,
      role: 'agent',
      content:
        'Distinguish them: inline field errors for validation, a toast for network/server errors. They have different recovery actions — fix the input vs retry — so collapsing them into one message hurts UX.',
      timestamp: '2026-06-01T09:01:30Z',
      authorName: 'Claude',
      model: 'Opus 4.8',
      widgets: [],
    },
  ]
}

// ─── Repos ───────────────────────────────────────────────────────────────────

const REPOS = [
  {
    id: 'rabbyte',
    name: 'rabbyte',
    avatarLabel: 'R',
    avatarColor: 'bg-violet-700',
    workspaces: [
      { id: 'rb-develop', branch: 'develop', status: 'locked' as const, age: '—' },
      {
        id: 'rb-onboarding',
        branch: 'feature/onboarding',
        parentId: 'rb-develop',
        status: 'pr-open' as const,
        added: 340,
        deleted: 12,
        age: '2d ago',
      },
      {
        id: 'rb-fix',
        branch: 'fix/signup-form',
        parentId: 'rb-develop',
        status: 'new' as const,
        added: 23,
        age: 'just now',
      },
    ],
  },
]

// ─── Projects ────────────────────────────────────────────────────────────────

const PROJECTS = [
  {
    id: 'rabbyte',
    name: 'Rabbyte',
    path: '/Users/mateo/dev/rabbyte',
    lastActivity: new Date(Date.now() - 2 * 60 * 60 * 1000),
  },
]

// ─── Workspaces ──────────────────────────────────────────────────────────────

const WORKSPACES: Record<string, { id: string; projectId: string; repoId: string; branch: string }> = {
  'rb-develop': { id: 'rb-develop', projectId: '', repoId: 'rabbyte', branch: 'develop' },
  'rb-onboarding': { id: 'rb-onboarding', projectId: '', repoId: 'rabbyte', branch: 'feature/onboarding' },
  'rb-fix': { id: 'rb-fix', projectId: '', repoId: 'rabbyte', branch: 'fix/signup-form' },
}

const wsStore = new Map(Object.entries(WORKSPACES))

// ─── Diffs ───────────────────────────────────────────────────────────────────

function makeLine(
  type: GitDiffLine['line_type'],
  content: string,
  old?: number,
  newL?: number,
): GitDiffLine {
  return { line_type: type, content, old_line_number: old, new_line_number: newL }
}

const DIFFS: Record<string, MultiFileDiff> = {
  'rb-onboarding': {
    commitHash: 'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2',
    commitMessage: 'feat(onboarding): add multi-step onboarding flow with progress tracking',
    totalFiles: 4,
    totalAdditions: 340,
    totalDeletions: 12,
    files: [
      {
        file_path: 'src/features/onboarding/OnboardingWizard.tsx',
        is_new: true,
        is_deleted: false,
        is_renamed: false,
        additions: 148,
        deletions: 0,
        lines: [
          makeLine('header', '@@ -0,0 +1,148 @@'),
          makeLine('added', "import { useState } from 'react'", undefined, 1),
          makeLine('added', "import { useNavigate } from 'react-router-dom'", undefined, 2),
          makeLine('added', '', undefined, 3),
          makeLine('added', 'const STEPS = [', undefined, 4),
          makeLine('added', "  { id: 'profile', title: 'Your Profile' },", undefined, 5),
          makeLine('added', "  { id: 'workspace', title: 'Create Workspace' },", undefined, 6),
          makeLine('added', "  { id: 'invite', title: 'Invite Team' },", undefined, 7),
          makeLine('added', "  { id: 'complete', title: 'All Done!' },", undefined, 8),
          makeLine('added', ']', undefined, 9),
          makeLine('added', '', undefined, 10),
          makeLine('added', 'export function OnboardingWizard() {', undefined, 11),
          makeLine('added', '  const [step, setStep] = useState(0)', undefined, 12),
          makeLine('added', '  const navigate = useNavigate()', undefined, 13),
          makeLine('added', '  function handleNext() {', undefined, 14),
          makeLine('added', '    if (step < STEPS.length - 1) setStep(s => s + 1)', undefined, 15),
          makeLine('added', "    else navigate('/dashboard')", undefined, 16),
          makeLine('added', '  }', undefined, 17),
          makeLine('added', '}', undefined, 18),
        ],
      },
      {
        file_path: 'src/api/onboarding.ts',
        is_new: true,
        is_deleted: false,
        is_renamed: false,
        additions: 45,
        deletions: 0,
        lines: [
          makeLine('header', '@@ -0,0 +1,45 @@'),
          makeLine('added', "import { apiFetch } from '@/lib/api'", undefined, 1),
          makeLine('added', '', undefined, 2),
          makeLine('added', 'export interface OnboardingData {', undefined, 3),
          makeLine('added', '  displayName: string', undefined, 4),
          makeLine('added', '  company?: string', undefined, 5),
          makeLine('added', '  workspaceName: string', undefined, 6),
          makeLine('added', '  inviteEmails: string[]', undefined, 7),
          makeLine('added', '}', undefined, 8),
          makeLine('added', '', undefined, 9),
          makeLine(
            'added',
            'export async function completeOnboarding(data: OnboardingData) {',
            undefined,
            10,
          ),
          makeLine('added', "  return apiFetch('/api/v1/onboarding/complete', {", undefined, 11),
          makeLine('added', "    method: 'POST',", undefined, 12),
          makeLine('added', "    headers: { 'Content-Type': 'application/json' },", undefined, 13),
          makeLine('added', '    body: JSON.stringify(data),', undefined, 14),
          makeLine('added', '  })', undefined, 15),
          makeLine('added', '}', undefined, 16),
        ],
      },
      {
        file_path: 'src/router.tsx',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 8,
        deletions: 2,
        lines: [
          makeLine('header', '@@ -12,8 +12,14 @@'),
          makeLine('removed', "import { LoginPage } from './pages/LoginPage'", 13),
          makeLine('added', "import { LoginPage } from './pages/LoginPage'", undefined, 13),
          makeLine(
            'added',
            "import { OnboardingWizard } from './features/onboarding/OnboardingWizard'",
            undefined,
            14,
          ),
          makeLine('removed', "  { path: '/dashboard', element: <DashboardPage /> },", 16),
          makeLine('added', "  { path: '/dashboard', element: <DashboardPage /> },", undefined, 17),
          makeLine(
            'added',
            "  { path: '/onboarding', element: <OnboardingWizard /> },",
            undefined,
            18,
          ),
        ],
      },
      {
        file_path: 'src/lib/validation.ts',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 5,
        deletions: 0,
        lines: [
          makeLine('header', '@@ -12,3 +12,8 @@'),
          makeLine('context', '}', 14, 14),
          makeLine('added', '', undefined, 15),
          makeLine(
            'added',
            'export function isValidEmail(email: string): boolean {',
            undefined,
            16,
          ),
          makeLine('added', '  return /^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$/.test(email)', undefined, 17),
          makeLine('added', '}', undefined, 18),
        ],
      },
    ],
  },
  'rb-fix': {
    commitHash: 'b2c3d4e5f6a7b2c3d4e5f6a7b2c3d4e5f6a7b2c3',
    commitMessage: 'fix(signup): validate email before submission to prevent 400 errors',
    totalFiles: 2,
    totalAdditions: 23,
    totalDeletions: 4,
    files: [
      {
        file_path: 'src/features/auth/SignupForm.tsx',
        is_new: false,
        is_deleted: false,
        is_renamed: false,
        additions: 18,
        deletions: 4,
        lines: [
          makeLine('header', '@@ -24,10 +24,24 @@'),
          makeLine('removed', '  const onSubmit = async (data: SignupFormData) => {', 25),
          makeLine('removed', '    await signup(data)', 26),
          makeLine('removed', '    navigate("/dashboard")', 27),
          makeLine('removed', '  }', 28),
          makeLine('added', '  const onSubmit = async (data: SignupFormData) => {', undefined, 25),
          makeLine('added', '    if (!isValidEmail(data.email)) {', undefined, 26),
          makeLine(
            'added',
            "      setError('email', { message: 'Invalid email address' })",
            undefined,
            27,
          ),
          makeLine('added', '      return', undefined, 28),
          makeLine('added', '    }', undefined, 29),
          makeLine('added', '    try {', undefined, 30),
          makeLine('added', '      await signup(data)', undefined, 31),
          makeLine('added', "      navigate('/dashboard')", undefined, 32),
          makeLine('added', '    } catch (err) {', undefined, 33),
          makeLine(
            'added',
            "      setError('root', { message: 'Signup failed. Please try again.' })",
            undefined,
            34,
          ),
          makeLine('added', '    }', undefined, 35),
          makeLine('added', '  }', undefined, 36),
        ],
      },
    ],
  },
}

// ─── Threads ──────────────────────────────────────────────────────────────────

function msg(
  id: string,
  author: string | null,
  isAgent: boolean,
  body: string,
  ts: string,
): ReviewMessage {
  return { id, author, isAgent, body, createdAt: ts }
}

const THREADS: Record<string, ReviewThread[]> = {
  'rb-onboarding': [
    {
      id: 'rbt-1',
      filePath: 'src/features/onboarding/OnboardingWizard.tsx',
      lineNumber: 12,
      side: 'right',
      isResolved: false,
      messages: [
        msg(
          'rbm-1-1',
          null,
          false,
          'Should we persist `step` to localStorage so users can resume the wizard if they close the tab mid-onboarding?',
          '2026-06-01T10:00:00Z',
        ),
        msg(
          'rbm-1-2',
          'Aria',
          true,
          'Good UX consideration. Add a `useEffect` that writes `step` to localStorage on change, and reads it back as the `useState` initial value. Clear it on the complete step.',
          '2026-06-01T10:01:00Z',
        ),
        msg('rbm-1-3', null, false, 'Done — added in the next commit.', '2026-06-01T10:05:00Z'),
      ],
    },
    {
      id: 'rbt-2',
      filePath: 'src/api/onboarding.ts',
      lineNumber: 10,
      side: 'right',
      isResolved: false,
      messages: [
        msg(
          'rbm-2-1',
          'Aria',
          true,
          'The `inviteEmails: string[]` field should be validated server-side too. A malformed email slipping through would cause a confusing 422 from the API.',
          '2026-06-01T10:15:00Z',
        ),
        msg(
          'rbm-2-2',
          null,
          false,
          'Agreed — adding Zod validation to the API route handler.',
          '2026-06-01T10:20:00Z',
        ),
        msg(
          'rbm-2-3',
          'Aria',
          true,
          'Also consider rate-limiting this endpoint — someone could enumerate whether emails are registered.',
          '2026-06-01T10:21:00Z',
        ),
      ],
    },
    {
      id: 'rbt-3',
      filePath: 'src/router.tsx',
      lineNumber: 18,
      side: 'right',
      isResolved: true,
      messages: [
        msg(
          'rbm-3-1',
          null,
          false,
          'Should `/onboarding` be protected by an auth guard?',
          '2026-06-01T10:30:00Z',
        ),
        msg(
          'rbm-3-2',
          'Aria',
          true,
          "Protect it with auth but add an `isOnboardingComplete` flag to the user profile. Redirect to `/dashboard` if they're already onboarded.",
          '2026-06-01T10:31:00Z',
        ),
      ],
    },
  ],
  'rb-fix': [
    {
      id: 'rbt-4',
      filePath: 'src/features/auth/SignupForm.tsx',
      lineNumber: 34,
      side: 'right',
      isResolved: false,
      messages: [
        msg(
          'rbm-4-1',
          'Aria',
          true,
          "The error message 'Signup failed' is user-friendly but consider distinguishing network errors from server errors — a toast for network, inline for validation.",
          '2026-06-01T11:00:00Z',
        ),
        msg(
          'rbm-4-2',
          null,
          false,
          'Fair point, will handle in a follow-up — this PR is just the email validation fix.',
          '2026-06-01T11:02:00Z',
        ),
      ],
    },
  ],
}

// ─── Descriptions ─────────────────────────────────────────────────────────────

const DESCRIPTIONS: Record<string, string> = {
  'rb-onboarding': `## Add Multi-Step Onboarding Flow

New users now go through a 4-step wizard after signup: profile → workspace → invite → complete.

### Changes
- **\`OnboardingWizard.tsx\`** — orchestrates wizard state and navigation
- **\`api/onboarding.ts\`** — \`completeOnboarding()\` persists everything server-side
- **\`router.tsx\`** — registers the \`/onboarding\` route`,

  'rb-fix': `## Fix Signup Form Email Validation

The signup form was submitting without validating email format, causing 400 errors from the API with no user feedback. Added \`isValidEmail()\` check before calling \`signup()\`.`,
}

// ─── Chats ────────────────────────────────────────────────────────────────────

const CHATS: Record<string, BranchReviewChat[]> = {
  'rb-onboarding': [
    { id: 'rbc-1', title: 'Onboarding wizard architecture', age: '1d', isActive: false },
    { id: 'rbc-2', title: 'Email validation approach', age: '3h', isActive: true },
  ],
  'rb-fix': [],
}

// ─── Project Chats ────────────────────────────────────────────────────────────

const PROJECT_CHATS: Record<string, ProjectChat[]> = {
  'rb-fix': [
    {
      id: 'chat-fix-1',
      wsId: 'rb-fix',
      title: 'Email validation approach',
      age: '5m',
      status: 'idle',
      type: 'chat',
    },
    {
      id: 'chat-fix-2',
      wsId: 'rb-fix',
      title: 'Form error handling',
      age: '2h',
      parentId: 'chat-fix-1',
      status: 'agent-running',
      type: 'chat',
    },
    {
      id: 'chat-fix-3',
      wsId: 'rb-fix',
      title: 'Unit test strategy',
      age: '1d',
      status: 'idle',
      type: 'chat',
    },
  ],
  'rb-onboarding': [
    {
      id: 'chat-ob-1',
      wsId: 'rb-onboarding',
      title: 'Onboarding flow design',
      age: '3d',
      status: 'idle',
      type: 'chat',
    },
    {
      id: 'chat-ob-2',
      wsId: 'rb-onboarding',
      title: 'Step validation logic',
      age: '4d',
      parentId: 'chat-ob-1',
      status: 'idle',
      type: 'chat',
    },
  ],
  'rb-develop': [
    {
      id: 'chat-dev-1',
      wsId: 'rb-develop',
      title: 'Architecture review',
      age: '1w',
      status: 'idle',
      type: 'chat',
    },
  ],
}

// ─── Git ──────────────────────────────────────────────────────────────────────

const COMMIT_MSGS = [
  'feat(onboarding): add multi-step wizard',
  'feat(auth): implement signup with email verification',
  'fix(signup): validate email before submission',
  'chore: add vitest configuration',
  'feat(dashboard): add activity feed component',
  'fix(api): handle rate limit errors gracefully',
  'refactor(auth): extract token refresh logic',
  'feat(profile): add avatar upload',
  'chore: upgrade React to v19',
  'docs: update API integration guide',
  'feat(billing): add Stripe webhook handler',
  'fix(router): handle 404 on unknown routes',
  'test: add auth flow integration tests',
  'feat(settings): add notification preferences',
  'chore: enable TypeScript strict mode',
]

function genHash(seed: number): string {
  let h = seed * 1_234_567 + 987_654_321
  h = ((h ^ (h >>> 16)) * 0x45d9f3b) & 0x7fffffff
  return (h + seed * 0xdeadbeef).toString(16).padStart(10, '0').repeat(4).slice(0, 40)
}

// ─── Dataset ──────────────────────────────────────────────────────────────────

export const normalDataset: ScenarioDataset = {
  repos: () => REPOS,
  projects: () => PROJECTS,
  workspace: (wsId) => wsStore.get(wsId),
  createWorkspace: (repoId, branch) => {
    const id = nanoid()
    const ws = { id, projectId: '', repoId, branch }
    wsStore.set(id, ws)
    return ws
  },
  fileTree: (repoPath) => getMockFileTree(repoPath) as unknown as FileNode,
  fileContent: (path) => getMockFileContent(path),
  branchDiff: (wsId) => DIFFS[wsId] ?? DIFFS['rb-fix'],
  branchThreads: (wsId) => THREADS[wsId] ?? [],
  branchDescription: (wsId) => DESCRIPTIONS[wsId] ?? '',
  branchChats: (wsId) => CHATS[wsId] ?? [],
  chats: (wsId) => PROJECT_CHATS[wsId] ?? [],
  gitLog: () =>
    Array.from({ length: 20 }, (_, i) => ({
      hash: genHash(i + 1),
      shortHash: genHash(i + 1).slice(0, 7),
      message: COMMIT_MSGS[i % COMMIT_MSGS.length],
      author: i % 3 === 0 ? 'Claude Agent' : 'Mateo Urrutia',
      date: i === 0 ? '1 hour ago' : i < 5 ? `${i * 8} hours ago` : `${Math.floor(i / 2)} days ago`,
    })),
  gitStatus: () => ({
    branch: 'feature/onboarding',
    ahead: 3,
    behind: 0,
    files: [
      {
        path: 'src/features/onboarding/OnboardingWizard.tsx',
        status: 'added' as const,
        staged: true,
      },
      { path: 'src/api/onboarding.ts', status: 'added' as const, staged: false },
    ],
  }),
  gitBranches: () => [
    { name: 'main', isCurrent: false, isRemote: false },
    { name: 'develop', isCurrent: false, isRemote: false },
    { name: 'feature/onboarding', isCurrent: true, isRemote: false },
    { name: 'fix/signup-form', isCurrent: false, isRemote: false },
  ],
  markdownTurns: (wsId, stepId) => {
    const fixture = getMockMarkdownTurns(wsId, stepId)
    return fixture.length > 0 ? fixture : genNormalTurns(wsId)
  },
}
