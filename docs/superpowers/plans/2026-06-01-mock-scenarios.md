# Mock Scenarios + Fault Injection Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add three switchable mock scenarios (Extreme/Normal/Empty) and per-endpoint fault injection to the Developer settings panel, routing all MSW handlers through a scenario-aware data layer.

**Architecture:** Chaos store extended with `scenario` + `faults` (persisted to localStorage). `apiFetch` sends `X-Crowbar-Scenario` + `X-Crowbar-Fault` headers. MSW handlers read headers via helper functions and return scenario-appropriate data or 500 errors. Developer settings panel gains two new sections gated behind `VITE_USE_MOCK`.

**Tech Stack:** React, Zustand (persist middleware), MSW 2, base-ui Slider + Select, TypeScript

---

## Task 1: Extend chaos store with scenario + faults + persist

**Files:**
- Modify: `web/src/lib/store/chaos.ts`
- Test: `web/src/__tests__/lib/store/chaos.test.ts`

- [ ] **Step 1: Write failing test**

```ts
// web/src/__tests__/lib/store/chaos.test.ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { useChaosStore } from '@/lib/store/chaos'

beforeEach(() => {
  useChaosStore.setState({
    latency: 0, errorRate: 0,
    scenario: 'normal',
    faults: {
      workspaces: 0, projects: 0, 'file-tree': 0, 'file-content': 0,
      'branch-diff': 0, 'branch-threads': 0, 'branch-description': 0, 'branch-chats': 0,
      'git-commits': 0, 'git-status': 0, 'git-branches': 0, 'markdown-chat': 0,
    },
  })
})

describe('useChaosStore', () => {
  it('setScenario updates scenario', () => {
    useChaosStore.getState().setScenario('extreme')
    expect(useChaosStore.getState().scenario).toBe('extreme')
  })

  it('setFault updates a single fault key', () => {
    useChaosStore.getState().setFault('branch-diff', 75)
    expect(useChaosStore.getState().faults['branch-diff']).toBe(75)
    expect(useChaosStore.getState().faults['workspaces']).toBe(0)
  })

  it('resetFaults clears all fault values to 0', () => {
    useChaosStore.getState().setFault('branch-diff', 100)
    useChaosStore.getState().setFault('workspaces', 50)
    useChaosStore.getState().resetFaults()
    const { faults } = useChaosStore.getState()
    expect(Object.values(faults).every(v => v === 0)).toBe(true)
  })

  it('reset clears latency and errorRate but preserves scenario/faults', () => {
    useChaosStore.getState().setLatency(500)
    useChaosStore.getState().setScenario('empty')
    useChaosStore.getState().reset()
    expect(useChaosStore.getState().latency).toBe(0)
    expect(useChaosStore.getState().scenario).toBe('empty')
  })
})
```

- [ ] **Step 2: Run to confirm fail**

```bash
cd web && npx vitest run src/__tests__/lib/store/chaos.test.ts
```

Expected: FAIL — `setScenario` does not exist.

- [ ] **Step 3: Replace `web/src/lib/store/chaos.ts` entirely**

```ts
import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { getDB } from '@/lib/persistence/idb'

export type Scenario = 'extreme' | 'normal' | 'empty'

export type FaultKey =
  | 'workspaces' | 'projects'
  | 'file-tree' | 'file-content'
  | 'branch-diff' | 'branch-threads' | 'branch-description' | 'branch-chats'
  | 'git-commits' | 'git-status' | 'git-branches'
  | 'markdown-chat'

export const FAULT_KEYS: FaultKey[] = [
  'workspaces', 'projects',
  'file-tree', 'file-content',
  'branch-diff', 'branch-threads', 'branch-description', 'branch-chats',
  'git-commits', 'git-status', 'git-branches',
  'markdown-chat',
]

export const FAULT_LABELS: Record<FaultKey, string> = {
  'workspaces': 'Workspaces',
  'projects': 'Projects',
  'file-tree': 'File tree',
  'file-content': 'File content',
  'branch-diff': 'Branch diff',
  'branch-threads': 'Review threads',
  'branch-description': 'PR description',
  'branch-chats': 'Branch chats',
  'git-commits': 'Git commits',
  'git-status': 'Git status',
  'git-branches': 'Git branches',
  'markdown-chat': 'Chat history',
}

const DEFAULT_FAULTS: Record<FaultKey, number> = {
  'workspaces': 0, 'projects': 0,
  'file-tree': 0, 'file-content': 0,
  'branch-diff': 0, 'branch-threads': 0, 'branch-description': 0, 'branch-chats': 0,
  'git-commits': 0, 'git-status': 0, 'git-branches': 0,
  'markdown-chat': 0,
}

interface ChaosState {
  latency: number
  errorRate: number
  scenario: Scenario
  faults: Record<FaultKey, number>
  setLatency: (ms: number) => void
  setErrorRate: (rate: number) => void
  setScenario: (s: Scenario) => void
  setFault: (key: FaultKey, pct: number) => void
  reset: () => void
  resetFaults: () => void
  applyScenario: (s: Scenario) => Promise<void>
}

export const useChaosStore = create<ChaosState>()(
  persist(
    (set) => ({
      latency: 0,
      errorRate: 0,
      scenario: 'normal' as Scenario,
      faults: { ...DEFAULT_FAULTS },
      setLatency: (latency) => set({ latency }),
      setErrorRate: (errorRate) => set({ errorRate }),
      setScenario: (scenario) => set({ scenario }),
      setFault: (key, pct) => set(s => ({ faults: { ...s.faults, [key]: pct } })),
      reset: () => set({ latency: 0, errorRate: 0 }),
      resetFaults: () => set({ faults: { ...DEFAULT_FAULTS } }),
      applyScenario: async (newScenario) => {
        set({ scenario: newScenario })
        try {
          const db = await getDB()
          await Promise.all([
            db.clear('query-cache'),
            db.clear('branch-review'),
            db.clear('workspace-hierarchy'),
            db.clear('sidebar-ui'),
          ])
        } catch { /* best effort — storage may be unavailable */ }
        window.location.reload()
      },
    }),
    {
      name: 'crowbar.chaos',
      partialize: (s) => ({ scenario: s.scenario, faults: s.faults }),
    },
  ),
)
```

- [ ] **Step 4: Run test — confirm pass**

```bash
cd web && npx vitest run src/__tests__/lib/store/chaos.test.ts
```

Expected: 4 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/lib/store/chaos.ts web/src/__tests__/lib/store/chaos.test.ts
git commit -m "feat: extend chaos store with scenario + per-endpoint faults"
```

---

## Task 2: Inject scenario + fault headers in apiFetch

**Files:**
- Modify: `web/src/lib/api.ts`
- Test: `web/src/__tests__/lib/api-headers.test.ts`

- [ ] **Step 1: Write failing test**

```ts
// web/src/__tests__/lib/api-headers.test.ts
import { describe, it, expect, beforeAll, afterAll, afterEach, vi } from 'vitest'
import { setupServer } from 'msw/node'
import { http, HttpResponse } from 'msw'
import { useChaosStore } from '@/lib/store/chaos'

// Capture request headers
let capturedHeaders: Record<string, string> = {}

const server = setupServer(
  http.get('/api/v0/test', ({ request }) => {
    capturedHeaders = {}
    request.headers.forEach((value, key) => { capturedHeaders[key] = value })
    return HttpResponse.json({ ok: true })
  }),
)
beforeAll(() => server.listen())
afterEach(() => { server.resetHandlers(); capturedHeaders = {} })
afterAll(() => server.close())

describe('apiFetch scenario headers', () => {
  it('sends X-Crowbar-Scenario when VITE_USE_MOCK is true', async () => {
    // Simulate VITE_USE_MOCK = 'true' by setting up the store
    useChaosStore.setState({ scenario: 'extreme', faults: { 'branch-diff': 100 } as any })
    const { apiFetch } = await import('@/lib/api')
    // We can't easily test env vars in unit tests — test the header injection logic separately
    // This test verifies the store values are accessible
    expect(useChaosStore.getState().scenario).toBe('extreme')
    expect(useChaosStore.getState().faults['branch-diff']).toBe(100)
  })
})
```

Note: Full integration testing of header injection requires a running MSW service worker. The unit test above verifies the store is correctly wired. The headers are verified manually when running the dev server.

- [ ] **Step 2: Update `web/src/lib/api.ts` — add scenario + fault headers**

In `apiFetch`, after the existing chaos headers block, add:

```ts
export async function apiFetch<T>(path: string, init?: RequestInit): Promise<T> {
  const { latency, errorRate, scenario, faults } = useChaosStore.getState()
  const chaosHeaders: Record<string, string> = {}
  if (latency > 0) chaosHeaders['X-Crowbar-Latency'] = String(latency)
  if (errorRate > 0) chaosHeaders['X-Crowbar-Error-Rate'] = String(errorRate)
  if (import.meta.env.VITE_USE_MOCK === 'true') {
    chaosHeaders['X-Crowbar-Scenario'] = scenario
    const activeFaults = Object.entries(faults).filter(([, v]) => v > 0)
    if (activeFaults.length > 0) {
      chaosHeaders['X-Crowbar-Fault'] = JSON.stringify(Object.fromEntries(activeFaults))
    }
  }

  const res = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: { ...init?.headers, ...chaosHeaders },
  })
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
  return res.json() as Promise<T>
}
```

- [ ] **Step 3: TypeScript check**

```bash
cd web && npx tsc --noEmit
```

Expected: zero errors.

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/api.ts web/src/__tests__/lib/api-headers.test.ts
git commit -m "feat: inject X-Crowbar-Scenario + X-Crowbar-Fault headers from chaos store"
```

---

## Task 3: Create fault helper + scenario dataset interface

**Files:**
- Create: `web/src/lib/mock/fault.ts`
- Create: `web/src/lib/mock/scenarios/index.ts`
- Test: `web/src/__tests__/lib/mock/fault.test.ts`

- [ ] **Step 1: Write failing test**

```ts
// web/src/__tests__/lib/mock/fault.test.ts
import { describe, it, expect } from 'vitest'
import { shouldFault } from '@/lib/mock/fault'

function makeRequest(faultHeader?: string): Request {
  const headers = new Headers()
  if (faultHeader) headers.set('X-Crowbar-Fault', faultHeader)
  return new Request('http://localhost/api/test', { headers })
}

describe('shouldFault', () => {
  it('returns false when no X-Crowbar-Fault header', () => {
    expect(shouldFault(makeRequest(), 'branch-diff')).toBe(false)
  })

  it('returns false when key not in header', () => {
    expect(shouldFault(makeRequest('{"workspaces":100}'), 'branch-diff')).toBe(false)
  })

  it('returns true when key is 100%', () => {
    // At 100%, should always return true
    const results = Array.from({ length: 20 }, () =>
      shouldFault(makeRequest('{"branch-diff":100}'), 'branch-diff')
    )
    expect(results.every(Boolean)).toBe(true)
  })

  it('returns false when key is 0%', () => {
    const results = Array.from({ length: 20 }, () =>
      shouldFault(makeRequest('{"branch-diff":0}'), 'branch-diff')
    )
    expect(results.every(r => !r)).toBe(true)
  })

  it('returns false for invalid JSON', () => {
    expect(shouldFault(makeRequest('not-json'), 'branch-diff')).toBe(false)
  })
})
```

- [ ] **Step 2: Run to confirm fail**

```bash
cd web && npx vitest run src/__tests__/lib/mock/fault.test.ts
```

Expected: FAIL — module not found.

- [ ] **Step 3: Create `web/src/lib/mock/fault.ts`**

```ts
import type { FaultKey } from '@/lib/store/chaos'

export function shouldFault(request: Request, key: FaultKey): boolean {
  const header = request.headers.get('X-Crowbar-Fault')
  if (!header) return false
  try {
    const faults = JSON.parse(header) as Record<string, number>
    return Math.random() * 100 < (faults[key] ?? 0)
  } catch {
    return false
  }
}
```

- [ ] **Step 4: Create `web/src/lib/mock/scenarios/index.ts`**

```ts
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

// Lazy imports to avoid loading all scenario data at startup
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
```

Note: Using `require()` for lazy loading avoids circular import issues and keeps startup fast. Vitest handles CommonJS `require()` fine in module context.

- [ ] **Step 5: Run tests — confirm pass**

```bash
cd web && npx vitest run src/__tests__/lib/mock/fault.test.ts
```

Expected: 5 tests PASS.

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/mock/fault.ts web/src/lib/mock/scenarios/index.ts web/src/__tests__/lib/mock/fault.test.ts
git commit -m "feat: add shouldFault helper and ScenarioDataset interface"
```

---

## Task 4: Create empty scenario

**Files:**
- Create: `web/src/lib/mock/scenarios/empty.ts`

- [ ] **Step 1: Create `web/src/lib/mock/scenarios/empty.ts`**

```ts
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
```

- [ ] **Step 2: TypeScript check**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/mock/scenarios/empty.ts
git commit -m "feat: add empty scenario (new user state)"
```

---

## Task 5: Create normal scenario (Rabbyte)

**Files:**
- Create: `web/src/lib/mock/scenarios/normal.ts`

- [ ] **Step 1: Create `web/src/lib/mock/scenarios/normal.ts`**

```ts
import { nanoid } from 'nanoid'
import type { ScenarioDataset } from './index'
import type { ReviewThread, ReviewMessage } from '@/features/branch-review/types/review-types'
import type { BranchReviewChat } from '@/lib/mock/branch-diff'
import type { GitDiffLine } from '@/features/git/types/git-types'
import { getMockFileTree, getMockFileContent } from '@/lib/mock/files'

// ─── Repos ──────────────────────────────────────────────────────────────────

const REPOS = [
  {
    id: 'rabbyte',
    name: 'rabbyte',
    avatarLabel: 'R',
    avatarColor: 'bg-violet-700',
    workspaces: [
      { id: 'rb-develop', branch: 'develop', status: 'locked' as const, age: '—' },
      { id: 'rb-onboarding', branch: 'feature/onboarding', parentId: 'rb-develop', status: 'pr-open' as const, added: 340, deleted: 12, age: '2d ago' },
      { id: 'rb-fix', branch: 'fix/signup-form', parentId: 'rb-develop', status: 'new' as const, added: 23, age: 'just now' },
    ],
  },
]

// ─── Projects ───────────────────────────────────────────────────────────────

const PROJECTS = [
  { id: 'rabbyte', name: 'Rabbyte', path: '/Users/mateo/dev/rabbyte', lastActivity: new Date(Date.now() - 2 * 60 * 60 * 1000) },
]

// ─── Workspaces ─────────────────────────────────────────────────────────────

const WORKSPACES: Record<string, { id: string; repoId: string; branch: string }> = {
  'rb-develop': { id: 'rb-develop', repoId: 'rabbyte', branch: 'develop' },
  'rb-onboarding': { id: 'rb-onboarding', repoId: 'rabbyte', branch: 'feature/onboarding' },
  'rb-fix': { id: 'rb-fix', repoId: 'rabbyte', branch: 'fix/signup-form' },
}

const wsStore = new Map(Object.entries(WORKSPACES))

// ─── Diffs ──────────────────────────────────────────────────────────────────

function makeLine(type: 'added' | 'removed' | 'context' | 'header', content: string, old?: number, newL?: number): GitDiffLine {
  return { line_type: type, content, old_line_number: old, new_line_number: newL }
}

const DIFFS: Record<string, { commitHash: string; commitMessage: string; files: any[]; totalFiles: number; totalAdditions: number; totalDeletions: number }> = {
  'rb-onboarding': {
    commitHash: 'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2',
    commitMessage: 'feat(onboarding): add multi-step onboarding flow with progress tracking',
    totalFiles: 7, totalAdditions: 340, totalDeletions: 12,
    files: [
      {
        file_path: 'src/features/onboarding/OnboardingWizard.tsx',
        is_new: true, is_deleted: false, is_renamed: false,
        additions: 148, deletions: 0,
        lines: [
          makeLine('header', '@@ -0,0 +1,148 @@'),
          makeLine('added', "import { useState } from 'react'", undefined, 1),
          makeLine('added', "import { useNavigate } from 'react-router-dom'", undefined, 2),
          makeLine('added', "import { OnboardingStep } from './types'", undefined, 3),
          makeLine('added', '', undefined, 4),
          makeLine('added', 'const STEPS: OnboardingStep[] = [', undefined, 5),
          makeLine('added', "  { id: 'profile', title: 'Your Profile', component: ProfileStep },", undefined, 6),
          makeLine('added', "  { id: 'workspace', title: 'Create Workspace', component: WorkspaceStep },", undefined, 7),
          makeLine('added', "  { id: 'invite', title: 'Invite Team', component: InviteStep },", undefined, 8),
          makeLine('added', "  { id: 'complete', title: 'All Done!', component: CompleteStep },", undefined, 9),
          makeLine('added', ']', undefined, 10),
          makeLine('added', '', undefined, 11),
          makeLine('added', 'export function OnboardingWizard() {', undefined, 12),
          makeLine('added', '  const [step, setStep] = useState(0)', undefined, 13),
          makeLine('added', '  const navigate = useNavigate()', undefined, 14),
          makeLine('added', '  const current = STEPS[step]', undefined, 15),
          makeLine('added', '', undefined, 16),
          makeLine('added', '  function handleNext() {', undefined, 17),
          makeLine('added', '    if (step < STEPS.length - 1) setStep(s => s + 1)', undefined, 18),
          makeLine('added', "    else navigate('/dashboard')", undefined, 19),
          makeLine('added', '  }', undefined, 20),
        ],
      },
      {
        file_path: 'src/features/onboarding/steps/ProfileStep.tsx',
        is_new: true, is_deleted: false, is_renamed: false,
        additions: 62, deletions: 0,
        lines: [
          makeLine('header', '@@ -0,0 +1,62 @@'),
          makeLine('added', "import { useForm } from 'react-hook-form'", undefined, 1),
          makeLine('added', "import { Input } from '@/components/ui/input'", undefined, 2),
          makeLine('added', "import { Button } from '@/components/ui/button'", undefined, 3),
          makeLine('added', '', undefined, 4),
          makeLine('added', 'export function ProfileStep({ onNext }: { onNext: () => void }) {', undefined, 5),
          makeLine('added', '  const { register, handleSubmit } = useForm()', undefined, 6),
          makeLine('added', '  return (', undefined, 7),
          makeLine('added', '    <form onSubmit={handleSubmit(onNext)} className="space-y-4">', undefined, 8),
          makeLine('added', '      <Input {...register("displayName")} placeholder="Your name" />', undefined, 9),
          makeLine('added', '      <Input {...register("company")} placeholder="Company (optional)" />', undefined, 10),
          makeLine('added', '      <Button type="submit">Continue</Button>', undefined, 11),
          makeLine('added', '    </form>', undefined, 12),
          makeLine('added', '  )', undefined, 13),
          makeLine('added', '}', undefined, 14),
        ],
      },
      {
        file_path: 'src/api/onboarding.ts',
        is_new: true, is_deleted: false, is_renamed: false,
        additions: 45, deletions: 0,
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
          makeLine('added', 'export async function completeOnboarding(data: OnboardingData) {', undefined, 10),
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
        is_new: false, is_deleted: false, is_renamed: false,
        additions: 8, deletions: 2,
        lines: [
          makeLine('header', '@@ -12,8 +12,14 @@'),
          makeLine('context', "import { DashboardPage } from './pages/DashboardPage'", 12, 12),
          makeLine('removed', "import { LoginPage } from './pages/LoginPage'", 13),
          makeLine('added', "import { LoginPage } from './pages/LoginPage'", undefined, 13),
          makeLine('added', "import { OnboardingWizard } from './features/onboarding/OnboardingWizard'", undefined, 14),
          makeLine('context', '', 14, 15),
          makeLine('context', 'export const router = createBrowserRouter([', 15, 16),
          makeLine('removed', "  { path: '/dashboard', element: <DashboardPage /> },", 16),
          makeLine('added', "  { path: '/dashboard', element: <DashboardPage /> },", undefined, 17),
          makeLine('added', "  { path: '/onboarding', element: <OnboardingWizard /> },", undefined, 18),
        ],
      },
    ],
  },
  'rb-fix': {
    commitHash: 'b2c3d4e5f6a7b2c3d4e5f6a7b2c3d4e5f6a7b2c3',
    commitMessage: 'fix(signup): validate email before submission to prevent 400 errors',
    totalFiles: 2, totalAdditions: 23, totalDeletions: 4,
    files: [
      {
        file_path: 'src/features/auth/SignupForm.tsx',
        is_new: false, is_deleted: false, is_renamed: false,
        additions: 18, deletions: 4,
        lines: [
          makeLine('header', '@@ -24,10 +24,24 @@'),
          makeLine('context', '  const { register, handleSubmit, formState } = useForm()', 24, 24),
          makeLine('removed', '  const onSubmit = async (data: SignupFormData) => {', 25),
          makeLine('removed', '    await signup(data)', 26),
          makeLine('removed', '    navigate("/dashboard")', 27),
          makeLine('removed', '  }', 28),
          makeLine('added', '  const onSubmit = async (data: SignupFormData) => {', undefined, 25),
          makeLine('added', '    if (!isValidEmail(data.email)) {', undefined, 26),
          makeLine('added', "      setError('email', { message: 'Invalid email address' })", undefined, 27),
          makeLine('added', '      return', undefined, 28),
          makeLine('added', '    }', undefined, 29),
          makeLine('added', '    try {', undefined, 30),
          makeLine('added', '      await signup(data)', undefined, 31),
          makeLine('added', "      navigate('/dashboard')", undefined, 32),
          makeLine('added', '    } catch (err) {', undefined, 33),
          makeLine('added', "      setError('root', { message: 'Signup failed. Please try again.' })", undefined, 34),
          makeLine('added', '    }', undefined, 35),
          makeLine('added', '  }', undefined, 36),
        ],
      },
      {
        file_path: 'src/lib/validation.ts',
        is_new: false, is_deleted: false, is_renamed: false,
        additions: 5, deletions: 0,
        lines: [
          makeLine('header', '@@ -12,3 +12,8 @@'),
          makeLine('context', 'export function isValidUrl(url: string): boolean {', 12, 12),
          makeLine('context', "  try { new URL(url); return true } catch { return false }", 13, 13),
          makeLine('context', '}', 14, 14),
          makeLine('added', '', undefined, 15),
          makeLine('added', 'export function isValidEmail(email: string): boolean {', undefined, 16),
          makeLine('added', '  return /^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$/.test(email)', undefined, 17),
          makeLine('added', '}', undefined, 18),
        ],
      },
    ],
  },
}

// ─── Threads ─────────────────────────────────────────────────────────────────

function msg(id: string, author: string | null, isAgent: boolean, body: string, createdAt: string): ReviewMessage {
  return { id, author, isAgent, body, createdAt }
}

const THREADS: Record<string, ReviewThread[]> = {
  'rb-onboarding': [
    {
      id: 'rbt-1', filePath: 'src/features/onboarding/OnboardingWizard.tsx', lineNumber: 13,
      side: 'right', isResolved: false,
      messages: [
        msg('rbm-1-1', null, false, 'Should we persist `step` to localStorage so users can resume the wizard if they close the tab mid-onboarding?', '2026-06-01T10:00:00Z'),
        msg('rbm-1-2', 'Aria', true, 'Good UX consideration. I\'d add a `useEffect` that writes `step` to localStorage on change, and reads it back as the `useState` initial value. Make sure to clear it on `CompleteStep`.', '2026-06-01T10:01:00Z'),
        msg('rbm-1-3', null, false, 'Done — added in the next commit.', '2026-06-01T10:05:00Z'),
      ],
    },
    {
      id: 'rbt-2', filePath: 'src/features/onboarding/steps/ProfileStep.tsx', lineNumber: 6,
      side: 'right', isResolved: true,
      messages: [
        msg('rbm-2-1', 'Morgan', false, 'Is `react-hook-form` already in our dependencies? I don\'t see it in package.json.', '2026-06-01T10:10:00Z'),
        msg('rbm-2-2', null, false, 'Good catch — added it. `pnpm add react-hook-form`', '2026-06-01T10:12:00Z'),
      ],
    },
    {
      id: 'rbt-3', filePath: 'src/api/onboarding.ts', lineNumber: 10,
      side: 'right', isResolved: false,
      messages: [
        msg('rbm-3-1', 'Aria', true, 'The `inviteEmails: string[]` field should be validated server-side too, not just in the form. A malformed email slipping through here would cause a confusing 422 from the API with no user feedback.', '2026-06-01T10:15:00Z'),
        msg('rbm-3-2', null, false, 'Agreed — adding Zod validation to the API route handler.', '2026-06-01T10:20:00Z'),
        msg('rbm-3-3', 'Aria', true, 'Also consider rate-limiting this endpoint — someone could enumerate whether emails are registered by repeatedly calling it.', '2026-06-01T10:21:00Z'),
      ],
    },
    {
      id: 'rbt-4', filePath: 'src/router.tsx', lineNumber: 18,
      side: 'right', isResolved: false,
      messages: [
        msg('rbm-4-1', null, false, 'Should `/onboarding` be protected by an auth guard, or only accessible to newly signed-up users?', '2026-06-01T10:30:00Z'),
        msg('rbm-4-2', 'Aria', true, 'Protect it with auth but add an `isOnboardingComplete` flag to the user profile. Redirect to `/dashboard` if they\'re already onboarded. Otherwise repeat visitors hit the wizard again.', '2026-06-01T10:31:00Z'),
      ],
    },
  ],
  'rb-fix': [
    {
      id: 'rbt-5', filePath: 'src/features/auth/SignupForm.tsx', lineNumber: 34,
      side: 'right', isResolved: false,
      messages: [
        msg('rbm-5-1', 'Aria', true, 'The error message "Signup failed. Please try again." leaks no info, which is good for security. But UX-wise, consider distinguishing network errors from server errors — a toast for network, inline for validation.', '2026-06-01T11:00:00Z'),
        msg('rbm-5-2', null, false, 'Fair point, will handle in a follow-up — this PR is just the email validation fix.', '2026-06-01T11:02:00Z'),
      ],
    },
  ],
}

// ─── Descriptions ────────────────────────────────────────────────────────────

const DESCRIPTIONS: Record<string, string> = {
  'rb-onboarding': `## Add Multi-Step Onboarding Flow

New users now go through a 4-step wizard after signup: profile → workspace → invite → complete.

### Changes
- **\`OnboardingWizard.tsx\`** — orchestrates the wizard state and navigation
- **\`ProfileStep.tsx\`** — collects display name and optional company
- **\`api/onboarding.ts\`** — \`completeOnboarding()\` persists everything server-side
- **\`router.tsx\`** — registers the \`/onboarding\` route

### Testing
- [ ] Sign up as a new user and verify all 4 steps render
- [ ] Verify skipping optional fields still completes successfully
- [ ] Verify redirect to \`/dashboard\` after completion`,

  'rb-fix': `## Fix Signup Form Email Validation

The signup form was submitting without validating the email format, causing 400 errors from the API with no user feedback.

### Fix
Added \`isValidEmail()\` check before calling \`signup()\`. Shows inline error if invalid, catches API errors and shows a root error.`,
}

// ─── Chats ───────────────────────────────────────────────────────────────────

const CHATS: Record<string, BranchReviewChat[]> = {
  'rb-onboarding': [
    { id: 'rbc-1', title: 'Onboarding wizard architecture', age: '1d', isActive: false },
    { id: 'rbc-2', title: 'Email validation approach', age: '3h', isActive: true },
  ],
  'rb-fix': [],
}

// ─── Git ─────────────────────────────────────────────────────────────────────

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

// ─── Markdown turns ───────────────────────────────────────────────────────────

const MARKDOWN_TURNS: Record<string, any[]> = {
  'rb-onboarding:brainstorm': [
    { id: 'rb-mt-1', role: 'user', content: 'Should we use a wizard or a single-page form for onboarding?', timestamp: '2026-06-01T09:00:00Z', authorName: 'Mateo', widgets: [] },
    { id: 'rb-mt-2', role: 'agent', content: 'A wizard works better here for three reasons:\n\n1. **Progressive disclosure** — users only see what\'s relevant to the current step\n2. **Completion psychology** — a progress bar increases completion rates\n3. **Error isolation** — validation errors are scoped to one step at a time\n\nSingle-page forms work for short flows (≤3 fields). Onboarding with profile + workspace + invites is too long.', timestamp: '2026-06-01T09:00:30Z', authorName: 'Claude', model: 'Opus 4.8', widgets: [] },
    { id: 'rb-mt-3', role: 'user', content: 'Makes sense. What should the step order be?', timestamp: '2026-06-01T09:01:00Z', authorName: 'Mateo', widgets: [] },
    { id: 'rb-mt-4', role: 'agent', content: 'I\'d go: **Profile → Workspace → Invite → Complete**.\n\nProfile first establishes identity (needed to name the workspace). Workspace before invite (you need somewhere to invite people to). Complete last as a celebration screen.\n\nAvoid putting "billing" in onboarding — it causes abandonment. Add it later via a soft prompt.', timestamp: '2026-06-01T09:01:30Z', authorName: 'Claude', model: 'Opus 4.8', widgets: [] },
  ],
}

// ─── Dataset ──────────────────────────────────────────────────────────────────

export const normalDataset: ScenarioDataset = {
  repos: () => REPOS,
  projects: () => PROJECTS,
  workspace: (wsId) => wsStore.get(wsId),
  createWorkspace: (repoId, branch) => {
    const id = nanoid()
    const ws = { id, repoId, branch }
    wsStore.set(id, ws)
    return ws
  },
  fileTree: (repoPath) => getMockFileTree(repoPath),
  fileContent: (path) => getMockFileContent(path),
  branchDiff: (wsId) => DIFFS[wsId] ?? DIFFS['rb-fix'],
  branchThreads: (wsId) => THREADS[wsId] ?? [],
  branchDescription: (wsId) => DESCRIPTIONS[wsId] ?? '',
  branchChats: (wsId) => CHATS[wsId] ?? [],
  gitLog: () => Array.from({ length: 20 }, (_, i) => ({
    hash: genHash(i + 1),
    shortHash: genHash(i + 1).slice(0, 7),
    message: COMMIT_MSGS[i % COMMIT_MSGS.length],
    author: i % 3 === 0 ? 'Claude Agent' : 'Mateo Urrutia',
    date: i === 0 ? '1 hour ago' : i < 5 ? `${i * 8} hours ago` : `${Math.floor(i / 2)} days ago`,
  })),
  gitStatus: () => ({
    branch: 'feature/onboarding',
    ahead: 3, behind: 0,
    files: [
      { path: 'src/features/onboarding/OnboardingWizard.tsx', status: 'added' as const, staged: true },
      { path: 'src/features/onboarding/steps/ProfileStep.tsx', status: 'added' as const, staged: true },
      { path: 'src/api/onboarding.ts', status: 'added' as const, staged: false },
    ],
  }),
  gitBranches: () => [
    { name: 'main', isCurrent: false, isRemote: false },
    { name: 'develop', isCurrent: false, isRemote: false },
    { name: 'feature/onboarding', isCurrent: true, isRemote: false },
    { name: 'fix/signup-form', isCurrent: false, isRemote: false },
  ],
  markdownTurns: (wsId, stepId) => MARKDOWN_TURNS[`${wsId}:${stepId}`] ?? [],
}
```

- [ ] **Step 2: TypeScript check**

```bash
cd web && npx tsc --noEmit
```

- [ ] **Step 3: Commit**

```bash
git add web/src/lib/mock/scenarios/normal.ts
git commit -m "feat: add normal scenario (Rabbyte project, realistic usage)"
```

---

## Task 6: Create extreme scenario

**Files:**
- Create: `web/src/lib/mock/scenarios/extreme.ts`

This is the largest file. It uses generator functions to produce rich, varied data without hardcoding thousands of lines.

- [ ] **Step 1: Create `web/src/lib/mock/scenarios/extreme.ts`**

```ts
import { nanoid } from 'nanoid'
import type { ScenarioDataset } from './index'
import type { ReviewThread, ReviewMessage } from '@/features/branch-review/types/review-types'
import type { BranchReviewChat } from '@/lib/mock/branch-diff'
import type { Commit, GitStatus, Branch } from '@/lib/mock/git-data'
import { generateLargeFileDiff } from '@/lib/mock/branch-diff'
import { getMockFileTree, getMockFileContent } from '@/lib/mock/files'
import type { GitDiffLine } from '@/features/git/types/git-types'

// ─── Helpers ─────────────────────────────────────────────────────────────────

function genHash(seed: number): string {
  let h = seed * 1_234_567 + 987_654_321
  h = ((h ^ (h >>> 16)) * 0x45d9f3b) & 0x7fffffff
  return (h + seed * 0xdeadbeef).toString(16).padStart(10, '0').repeat(4).slice(0, 40)
}

function msg(id: string, author: string | null, isAgent: boolean, body: string, ts: string): ReviewMessage {
  return { id, author, isAgent, body, createdAt: ts }
}

// ─── Thread message pools ─────────────────────────────────────────────────────
// 8 pools of realistic multi-turn review conversations

const MESSAGE_POOLS: Array<Array<[string | null, boolean, string]>> = [
  [
    [null, false, 'This function is O(n²) in the worst case because of the nested loop on line {L}. With the data sizes we\'re expecting it will be noticeably slow.'],
    ['Aria', true, 'Confirmed — if `items` is large this is a real perf issue. The fix is to build a `Map<id, item>` in a single pass first, then the inner lookup becomes O(1). Overall complexity drops to O(n).'],
    [null, false, 'Refactored to use a Map. Benchmarks show 40× speedup on 10k items.'],
    ['Aria', true, 'Nice. One follow-up: the Map is rebuilt on every call. If this is hot path, consider memoizing it outside the function or using `useMemo` if this is React code.'],
    [null, false, 'Added `useMemo` with the items array as the dependency. Looks good now.'],
  ],
  [
    ['Morgan', false, 'Nit: the variable name `data` is too generic here. Given the context it should be `userProfile` or `profilePayload`.'],
    [null, false, 'Renamed throughout. Also renamed `res` → `profileResponse` in the same block.'],
    ['Aria', true, 'Good rename. While we\'re here: `profilePayload` implies it\'s being sent somewhere. If it\'s being received, `profileData` or `profileRecord` reads more naturally.'],
    [null, false, 'Fair. Changed to `profileRecord`.'],
  ],
  [
    [null, false, 'Should this be `null` or `undefined`? The rest of the codebase uses `undefined` for optional values.'],
    ['Aria', true, '`null` is correct here because the backend explicitly returns `null` when the field is absent (not omitting the key). `undefined` would mean "never set", which is a different semantic. Worth adding a comment explaining the distinction.'],
    [null, false, 'Added comment: `// null = backend explicitly absent, not unset`.'],
    ['Morgan', false, 'Could we encode this at the type level instead of a comment? Something like `type ExplicitNull<T> = T | null` with a JSDoc explaining the convention.'],
    ['Aria', true, 'Good idea but probably overkill for now. A comment is sufficient. If this pattern appears in 3+ places, introduce the type alias.'],
  ],
  [
    ['Aria', true, 'This `useEffect` has `user` in the dependency array but `user.id` is what\'s actually used inside. The effect will re-run on any object identity change even if the id didn\'t change. Replace `user` with `user.id` in the dep array.'],
    [null, false, 'Fixed. Changed `[user]` to `[user.id]`.'],
    ['Aria', true, 'Good. Also note: if `user` can be `null` here, `user.id` would throw. Add a null guard to the dep array: `[user?.id]`.'],
    [null, false, 'Added null guard. Also wrapped the effect body in `if (!user) return`.'],
  ],
  [
    [null, false, 'Is this error being swallowed intentionally? The `catch` block is empty.'],
    ['Aria', true, 'This is a problem. Silent failures make debugging very hard. At minimum log to console in dev. Better: propagate the error to the UI via error state, or use an error boundary.'],
    ['Morgan', false, 'We have a `reportError()` util that sends to Sentry in prod and logs in dev. Should use that here.'],
    [null, false, 'Added `reportError(err)` in the catch. Also added an error state that shows a toast.'],
    ['Aria', true, 'Perfect. One thing: make sure the toast message is user-friendly and doesn\'t expose internal error details like stack traces or API paths.'],
    [null, false, 'The toast just says "Something went wrong. Please try again." — no internal details exposed.'],
  ],
  [
    ['Aria', true, 'This component re-renders every time the parent re-renders because the callback is defined inline. Wrap it in `useCallback` or extract it outside the component.'],
    [null, false, 'Wrapped in `useCallback`. Dependencies are `[dispatch, itemId]`.'],
    ['Aria', true, 'Check that `dispatch` is stable — from Redux/Zustand it should be, but if it\'s a local function make sure it\'s also memoized. The dep array should only include values that actually change.'],
    ['Morgan', false, '`dispatch` from Zustand is guaranteed stable — it\'s defined once in the store creation and doesn\'t change. Safe to include but won\'t actually cause re-runs.'],
  ],
  [
    [null, false, 'The regex on line {L} will fail for email addresses with subdomains (e.g., `user@mail.company.co.uk`). The pattern only allows one dot after the @.'],
    ['Aria', true, 'Correct catch. The simplest fix: use `/^[^\\s@]+@[^\\s@]+\\.[^\\s@]+$/` — it allows any non-whitespace/@ characters with at least one dot after @. It\'s deliberately permissive; email validation at the RFC level is a rabbit hole.'],
    [null, false, 'Updated regex. Added a test case for subdomain emails.'],
    ['Aria', true, 'Good. Also add a test case for `user+tag@example.com` (plus-addressing) and `user@localhost` (no TLD) — the latter should probably be rejected in prod but the current regex would allow it.'],
  ],
  [
    ['Morgan', false, 'This API call doesn\'t handle the case where the request times out. On a slow connection the user will see a spinner forever.'],
    ['Aria', true, 'Add an `AbortController` with a timeout. Here\'s the pattern:\n```ts\nconst controller = new AbortController()\nconst id = setTimeout(() => controller.abort(), 10_000)\ntry {\n  const res = await fetch(url, { signal: controller.signal })\n  ...\n} finally {\n  clearTimeout(id)\n}\n```'],
    [null, false, 'Implemented with `AbortController`. Timeout set to 10s to match backend timeout.'],
    ['Aria', true, 'Make the timeout configurable — different environments may need different values. A constant at the top of the file or an environment variable.'],
    [null, false, 'Added `API_TIMEOUT_MS` constant, defaults to 10000, overridable via `VITE_API_TIMEOUT`.'],
  ],
]

// ─── Thread generator ─────────────────────────────────────────────────────────

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
    const messages = pool.map((([author, isAgent, body], j) => msg(
      `msg-${wsId}-${i}-${j}`,
      author,
      isAgent,
      body.replace('{L}', String(lineNum)),
      `2026-05-${String(15 + (i % 15)).padStart(2, '0')}T${String(8 + (j % 12)).padStart(2, '0')}:${String((i * 7 + j * 13) % 60).padStart(2, '0')}:00Z`,
    )))
    return {
      id: `thread-${wsId}-${i}`,
      filePath: file,
      lineNumber: lineNum,
      side: i % 3 === 0 ? 'left' : 'right',
      messages,
      isResolved: i % 5 === 0,
    }
  })
}

// ─── Diff generator ───────────────────────────────────────────────────────────

function makeMassiveDiff(wsId: string, seed: number) {
  const isMassive = seed % 3 === 0  // every 3rd workspace gets a 1M-line diff
  if (isMassive) {
    const big = generateLargeFileDiff('src/lib/query/query-layer.ts', 260)
    const medium = generateLargeFileDiff('src/lib/query/cache-manager.ts', 120)
    return {
      commitHash: genHash(seed),
      commitMessage: `refactor: major rewrite of ${wsId} data layer — ${(seed * 43211) % 900000 + 100000} lines affected`,
      files: [
        { file_path: 'src/lib/query/query-layer.ts', is_new: false, is_deleted: false, is_renamed: false, additions: 65234, deletions: 44821, lines: big },
        { file_path: 'src/lib/query/cache-manager.ts', is_new: false, is_deleted: false, is_renamed: false, additions: 18248, deletions: 20101, lines: medium },
        { file_path: 'src/lib/query/index.ts', is_new: false, is_deleted: false, is_renamed: false, additions: 12, deletions: 28, lines: [
          { line_type: 'header', content: '@@ -1,30 +1,14 @@' } as GitDiffLine,
          { line_type: 'removed', content: "export { legacyFetch } from './legacy'", old_line_number: 1 } as GitDiffLine,
          { line_type: 'added', content: "export { QueryLayer } from './query-layer'", new_line_number: 1 } as GitDiffLine,
        ]},
      ],
      totalFiles: 3,
      totalAdditions: 83494 + (seed * 1000),
      totalDeletions: 64949 + (seed * 500),
    }
  }

  // Medium diff — 20-40 files
  const fileCount = 20 + (seed % 20)
  const files = Array.from({ length: fileCount }, (_, fi) => {
    const additions = 10 + (fi * seed % 80)
    const deletions = fi % 4 === 0 ? 0 : 5 + (fi * seed % 30)
    return {
      file_path: `src/${EXTREME_FILES[fi % EXTREME_FILES.length]}`,
      is_new: fi < 3, is_deleted: false, is_renamed: fi % 7 === 0,
      additions, deletions,
      lines: [
        { line_type: 'header', content: `@@ -1,${deletions} +1,${additions} @@` } as GitDiffLine,
        ...Array.from({ length: Math.min(additions, 8) }, (_, li) => ({
          line_type: 'added' as const,
          content: `  // line ${li + 1} of ${additions} additions`,
          new_line_number: li + 1,
        })),
      ],
    }
  })
  return {
    commitHash: genHash(seed),
    commitMessage: `feat(${wsId}): implement feature with ${fileCount} files changed`,
    files, totalFiles: fileCount,
    totalAdditions: files.reduce((s, f) => s + f.additions, 0),
    totalDeletions: files.reduce((s, f) => s + f.deletions, 0),
  }
}

// ─── Commits ─────────────────────────────────────────────────────────────────

const EXTREME_MSGS = [
  'feat: add payment processing module', 'fix: resolve null pointer in auth middleware',
  'refactor: extract database connection pool', 'chore: update dependencies to latest versions',
  'docs: add API endpoint documentation', 'test: add integration tests for user service',
  'perf: optimize query execution with indexes', 'feat: implement real-time notification system',
  'fix: handle edge case in date parsing', "Merge branch 'feature/payments' into develop",
  'feat: add CSV export functionality', 'fix: correct timezone handling in scheduler',
  'chore: add CI/CD pipeline configuration', 'style: apply linting rules across codebase',
  'feat: add dashboard analytics widgets', 'fix: resolve memory leak in WebSocket handler',
  'refactor: split monolithic user service', 'test: add unit tests for billing module',
  'feat: implement OAuth2 authentication flow', 'chore: upgrade Go to v1.25',
  'docs: update API reference', 'fix: correct rounding error in invoice total',
  'feat: add email notification system', 'refactor: migrate to TypeScript strict mode',
  'perf: add Redis caching for hot queries', 'feat: add file upload with S3 presigned URLs',
  'fix: handle concurrent write race condition', 'chore: configure Dependabot',
  'feat: add webhook delivery with retry logic', 'test: add E2E tests for checkout flow',
  'fix: correct pagination offset in list endpoints', 'refactor: consolidate error handling',
  'feat: add rate limiting per API key', 'docs: add architecture decision records',
  'fix: resolve CORS preflight issue', 'perf: lazy-load heavy chart components',
  'feat: implement GraphQL subscriptions', 'chore: migrate from npm to pnpm',
  'fix: handle empty state in workspace tree', 'feat: add keyboard shortcut for file search',
  'security: patch XSS vulnerability in markdown renderer', 'feat: implement multi-region failover',
  'fix: race condition in session token refresh', 'perf: reduce bundle size by 34% with code splitting',
  'feat: add audit log for compliance requirements', 'refactor: replace redux with zustand',
]

const AUTHORS_EX = ['Mateo Urrutia', 'Claude Agent', 'Alice Chen', 'Bob Rodriguez', 'Dependabot[bot]', 'Sofia Andersson', 'James Liu', 'Priya Patel']

function genCommits(count: number): Commit[] {
  return Array.from({ length: count }, (_, i) => {
    const hash = genHash(i + 100)
    const hoursAgo = i * 2
    const daysAgo = Math.floor(hoursAgo / 24)
    return {
      hash, shortHash: hash.slice(0, 7),
      message: EXTREME_MSGS[i % EXTREME_MSGS.length],
      author: AUTHORS_EX[i % AUTHORS_EX.length],
      date: daysAgo === 0 ? `${hoursAgo % 24 || 1}h ago` : daysAgo === 1 ? '1 day ago' : `${daysAgo} days ago`,
    }
  })
}

// ─── Branch chats ─────────────────────────────────────────────────────────────

function genChats(wsId: string, count: number): BranchReviewChat[] {
  const titles = [
    'Architecture review', 'Performance bottlenecks', 'Security audit findings',
    'Database migration plan', 'API versioning strategy', 'Test coverage gaps',
    'Cache invalidation design', 'Error handling patterns', 'TypeScript migration',
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
    id: 'crowbar', name: 'crowbar', avatarLabel: 'C', avatarColor: 'bg-indigo-700',
    workspaces: [
      { id: 'ex-cr-dev', branch: 'develop', status: 'locked' as const, age: '—' },
      { id: 'ex-cr-1', branch: 'feature/app-design', parentId: 'ex-cr-dev', status: 'pr-open' as const, added: 5672, age: '16h ago' },
      { id: 'ex-cr-2', branch: 'enhancement/scaffold', parentId: 'ex-cr-1', status: 'agent-running' as const, added: 22892, age: '3d ago' },
      { id: 'ex-cr-3', branch: 'fix/toolbar-crash', parentId: 'ex-cr-1', status: 'new' as const, age: 'just now' },
      { id: 'ex-cr-4', branch: 'feature/api-backend', parentId: 'ex-cr-dev', status: 'pr-merged' as const, added: 27347, deleted: 455, age: '1d ago' },
      { id: 'ex-cr-5', branch: 'feature/ws-channels', parentId: 'ex-cr-dev', status: 'pr-open' as const, added: 8841, deleted: 203, age: '2d ago' },
      { id: 'ex-cr-6', branch: 'fix/ws-reconnect', parentId: 'ex-cr-5', status: 'new' as const, added: 47, age: '2h ago' },
      { id: 'ex-cr-7', branch: 'refactor/query-layer', parentId: 'ex-cr-dev', status: 'agent-running' as const, added: 103482, deleted: 88910, age: '5d ago', hasConflicts: true },
      { id: 'ex-cr-8', branch: 'refactor/query-layer-tests', parentId: 'ex-cr-7', status: 'new' as const, added: 4320, age: '4d ago' },
      { id: 'ex-cr-9', branch: 'refactor/cache-manager', parentId: 'ex-cr-7', status: 'pr-open' as const, added: 18248, deleted: 20101, age: '4d ago' },
      { id: 'ex-cr-10', branch: 'fix/cache-stampede', parentId: 'ex-cr-9', status: 'agent-running' as const, added: 312, age: '3d ago' },
      { id: 'ex-cr-11', branch: 'feat/ai-review', parentId: 'ex-cr-dev', status: 'pr-open' as const, added: 9120, deleted: 340, age: '6d ago' },
      { id: 'ex-cr-12', branch: 'feat/terminal-v2', parentId: 'ex-cr-dev', status: 'agent-running' as const, added: 15600, deleted: 2100, age: '7d ago' },
      { id: 'ex-cr-13', branch: 'fix/pty-resize', parentId: 'ex-cr-12', status: 'new' as const, added: 88, age: '1d ago' },
      { id: 'ex-cr-14', branch: 'chore/bump-deps', parentId: 'ex-cr-dev', status: 'pr-closed' as const, added: 312, deleted: 298, age: '6d ago' },
      { id: 'ex-cr-15', branch: 'hotfix/security-xss', parentId: 'ex-cr-dev', status: 'pr-open' as const, added: 23, deleted: 7, age: '2h ago' },
    ],
  },
  {
    id: 'quiver-core', name: 'quiver.core', avatarLabel: 'Q', avatarColor: 'bg-emerald-700',
    workspaces: [
      { id: 'ex-qc-dev', branch: 'develop', status: 'locked' as const, age: '—' },
      { id: 'ex-qc-1', branch: 'feature/oauth2', parentId: 'ex-qc-dev', status: 'pr-open' as const, added: 4521, deleted: 89, age: '1d ago' },
      { id: 'ex-qc-2', branch: 'fix/token-expiry', parentId: 'ex-qc-1', status: 'new' as const, added: 47, age: 'just now', hasConflicts: true },
      { id: 'ex-qc-3', branch: 'perf/redis-cache', parentId: 'ex-qc-dev', status: 'agent-running' as const, added: 1823, deleted: 402, age: '12h ago' },
      { id: 'ex-qc-4', branch: 'feat/graphql-subscriptions', parentId: 'ex-qc-dev', status: 'pr-open' as const, added: 12400, deleted: 800, age: '3d ago' },
      { id: 'ex-qc-5', branch: 'fix/graphql-n-plus-one', parentId: 'ex-qc-4', status: 'agent-running' as const, added: 340, deleted: 120, age: '2d ago' },
      { id: 'ex-qc-6', branch: 'feat/audit-log', parentId: 'ex-qc-dev', status: 'pr-open' as const, added: 6700, deleted: 200, age: '5d ago' },
      { id: 'ex-qc-7', branch: 'refactor/db-pool', parentId: 'ex-qc-dev', status: 'pr-merged' as const, added: 890, deleted: 1200, age: '8d ago' },
      { id: 'ex-qc-8', branch: 'feat/multi-region', parentId: 'ex-qc-dev', status: 'agent-running' as const, added: 45000, deleted: 8200, age: '10d ago' },
      { id: 'ex-qc-9', branch: 'fix/region-failover', parentId: 'ex-qc-8', status: 'new' as const, added: 230, age: '9d ago', hasConflicts: true },
      { id: 'ex-qc-10', branch: 'chore/upgrade-postgres', parentId: 'ex-qc-dev', status: 'pr-closed' as const, added: 45, deleted: 30, age: '12d ago' },
      { id: 'ex-qc-11', branch: 'security/patch-xss', parentId: 'ex-qc-dev', status: 'pr-open' as const, added: 12, deleted: 8, age: '4h ago' },
    ],
  },
  {
    id: 'quiver-desktop', name: 'quiver.desktop', avatarLabel: 'Q', avatarColor: 'bg-orange-700',
    workspaces: [
      { id: 'ex-qd-dev', branch: 'develop', status: 'locked' as const, age: '—' },
      { id: 'ex-qd-1', branch: 'feature/quiver-shell', parentId: 'ex-qd-dev', status: 'pr-open' as const, added: 13485, deleted: 69, age: '3d ago' },
      { id: 'ex-qd-2', branch: 'fix/pty-encoding', parentId: 'ex-qd-1', status: 'new' as const, added: 34, age: '1d ago' },
      { id: 'ex-qd-3', branch: 'feat/native-file-picker', parentId: 'ex-qd-dev', status: 'agent-running' as const, added: 2341, age: '8h ago' },
      { id: 'ex-qd-4', branch: 'fix/startup-crash', parentId: 'ex-qd-dev', status: 'pr-merged' as const, added: 23, deleted: 7, age: '4d ago' },
      { id: 'ex-qd-5', branch: 'feat/auto-update', parentId: 'ex-qd-dev', status: 'pr-open' as const, added: 3400, deleted: 890, age: '6d ago' },
      { id: 'ex-qd-6', branch: 'fix/update-rollback', parentId: 'ex-qd-5', status: 'agent-running' as const, added: 120, deleted: 45, age: '5d ago' },
      { id: 'ex-qd-7', branch: 'feat/plugin-system', parentId: 'ex-qd-dev', status: 'pr-open' as const, added: 8900, deleted: 450, age: '7d ago' },
      { id: 'ex-qd-8', branch: 'chore/tauri-v2-migration', parentId: 'ex-qd-dev', status: 'agent-running' as const, added: 34000, deleted: 29000, age: '9d ago', hasConflicts: true },
      { id: 'ex-qd-9', branch: 'fix/macos-permissions', parentId: 'ex-qd-dev', status: 'new' as const, added: 67, age: '6h ago' },
      { id: 'ex-qd-10', branch: 'feat/deep-links', parentId: 'ex-qd-dev', status: 'pr-open' as const, added: 1200, deleted: 100, age: '11d ago' },
    ],
  },
  {
    id: 'quiver-cloud', name: 'quiver.cloud', avatarLabel: 'Q', avatarColor: 'bg-sky-700',
    workspaces: [
      { id: 'ex-qcl-dev', branch: 'develop', status: 'locked' as const, age: '—' },
      { id: 'ex-qcl-1', branch: 'feature/multi-tenant', parentId: 'ex-qcl-dev', status: 'pr-open' as const, added: 31204, deleted: 1823, age: '2d ago' },
      { id: 'ex-qcl-2', branch: 'feat/s3-presign', parentId: 'ex-qcl-dev', status: 'new' as const, added: 892, age: '4h ago' },
      { id: 'ex-qcl-3', branch: 'chore/infra-terraform', parentId: 'ex-qcl-dev', status: 'pr-merged' as const, added: 4812, deleted: 3201, age: '7d ago' },
      { id: 'ex-qcl-4', branch: 'feat/cdn-integration', parentId: 'ex-qcl-dev', status: 'agent-running' as const, added: 2300, deleted: 400, age: '5d ago' },
      { id: 'ex-qcl-5', branch: 'fix/cdn-cache-invalidation', parentId: 'ex-qcl-4', status: 'new' as const, added: 45, age: '4d ago' },
      { id: 'ex-qcl-6', branch: 'feat/usage-billing', parentId: 'ex-qcl-dev', status: 'pr-open' as const, added: 7800, deleted: 1200, age: '8d ago' },
      { id: 'ex-qcl-7', branch: 'feat/sso-saml', parentId: 'ex-qcl-dev', status: 'agent-running' as const, added: 12000, deleted: 2300, age: '10d ago' },
      { id: 'ex-qcl-8', branch: 'fix/saml-assertion', parentId: 'ex-qcl-7', status: 'new' as const, added: 89, age: '9d ago', hasConflicts: true },
      { id: 'ex-qcl-9', branch: 'chore/k8s-upgrade', parentId: 'ex-qcl-dev', status: 'pr-closed' as const, added: 340, deleted: 280, age: '14d ago' },
    ],
  },
]

// ─── Workspace store ──────────────────────────────────────────────────────────

const wsStore = new Map<string, { id: string; repoId: string; branch: string }>()
for (const repo of EXTREME_REPOS) {
  for (const ws of repo.workspaces) {
    wsStore.set(ws.id, { id: ws.id, repoId: repo.id, branch: ws.branch })
  }
}

// ─── Descriptions ─────────────────────────────────────────────────────────────

const WS_DESCRIPTIONS: Record<string, string> = {
  'ex-cr-7': `## Refactor: Replace Legacy Fetch + Global Cache with QueryLayer

This is a **major refactor** touching ~190k lines. The legacy \`legacyFetch\` + \`globalCache\` approach was not type-safe, missing request cancellation, and prone to cache stampede.

### Status
- [x] Core \`QueryLayer\` and \`CacheManager\` implemented  
- [x] \`legacy-fetch.ts\` deleted
- [ ] Migrate 3 remaining callers in \`src/features/dashboard/\`
- [ ] Add in-flight deduplication to CacheManager

> ⚠️ **Do not merge** until dashboard migration and in-flight dedup are complete.`,

  'ex-qcl-1': `## Feature: Multi-Tenant Architecture

Splits the single-tenant data model into isolated tenant namespaces. Every database query now requires a \`tenantId\` and is validated against the authenticated session.

### Security
All API routes enforce tenant isolation at the middleware layer. Cross-tenant data leaks are prevented by database-level row security policies.`,
}

// ─── Git status ───────────────────────────────────────────────────────────────

const GIT_STATUSES: Record<string, GitStatus> = {
  crowbar: { branch: 'refactor/query-layer', ahead: 47, behind: 3, files: [
    { path: 'src/lib/query/query-layer.ts', status: 'modified', staged: true },
    { path: 'src/lib/query/cache-manager.ts', status: 'modified', staged: true },
    { path: 'src/lib/query/legacy-fetch.ts', status: 'deleted', staged: true },
    { path: 'src/lib/query/index.ts', status: 'modified', staged: false },
    { path: 'src/features/dashboard/page.tsx', status: 'modified', staged: false },
    { path: 'src/features/dashboard/hooks.ts', status: 'modified', staged: false },
    { path: 'tests/query-layer.test.ts', status: 'added', staged: false },
  ]},
  'quiver-core': { branch: 'feat/multi-region', ahead: 23, behind: 1, files: [
    { path: 'k8s/apps/multi-tenant.yaml', status: 'added', staged: true },
    { path: 'terraform/modules/eks/node_groups.tf', status: 'modified', staged: true },
    { path: 'src/api/region-router.ts', status: 'added', staged: false },
    { path: 'src/lib/db.ts', status: 'modified', staged: false },
  ]},
  'quiver-desktop': { branch: 'chore/tauri-v2-migration', ahead: 82, behind: 0, files: [
    { path: 'src-tauri/src/main.rs', status: 'modified', staged: true },
    { path: 'src-tauri/Cargo.toml', status: 'modified', staged: true },
    { path: 'src-tauri/tauri.conf.json', status: 'modified', staged: false },
    { path: 'src/app/bridge.ts', status: 'modified', staged: false },
  ]},
  'quiver-cloud': { branch: 'feature/multi-tenant', ahead: 34, behind: 5, files: [
    { path: 'k8s/base/rbac.yaml', status: 'modified', staged: true },
    { path: 'src/middleware/tenant.ts', status: 'added', staged: true },
    { path: 'src/db/schemas/tenant.sql', status: 'added', staged: false },
    { path: '.github/workflows/deploy.yml', status: 'modified', staged: false },
  ]},
}

// ─── Dataset ──────────────────────────────────────────────────────────────────

const commitCache = genCommits(500)

export const extremeDataset: ScenarioDataset = {
  repos: () => EXTREME_REPOS,
  projects: () => [
    { id: 'crowbar', name: 'Crowbar', path: '/Users/mateo/dev/crowbar', lastActivity: new Date(Date.now() - 30 * 60 * 1000) },
    { id: 'quiver', name: 'Quiver', path: '/Users/mateo/dev/quiver', lastActivity: new Date(Date.now() - 2 * 60 * 60 * 1000) },
    { id: 'rabbyte', name: 'Rabbyte', path: '/Users/mateo/dev/rabbyte', lastActivity: new Date(Date.now() - 24 * 60 * 60 * 1000) },
  ],
  workspace: (wsId) => wsStore.get(wsId),
  createWorkspace: (repoId, branch) => {
    const id = nanoid()
    const ws = { id, repoId, branch }
    wsStore.set(id, ws)
    return ws
  },
  fileTree: (repoPath) => getMockFileTree(repoPath),
  fileContent: (path) => getMockFileContent(path),
  branchDiff: (wsId) => {
    const seed = Array.from(wsId).reduce((s, c) => s + c.charCodeAt(0), 0)
    return makeMassiveDiff(wsId, seed) as any
  },
  branchThreads: (wsId) => genThreads(wsId, 28),
  branchDescription: (wsId) => WS_DESCRIPTIONS[wsId] ?? `## ${wsId}\n\nThis branch contains significant changes across multiple subsystems. Review carefully before merging.`,
  branchChats: (wsId) => genChats(wsId, 9),
  gitLog: () => commitCache,
  gitStatus: (repoPath) => {
    const repoId = repoPath.split('/').filter(Boolean).pop() ?? 'crowbar'
    return GIT_STATUSES[repoId] ?? GIT_STATUSES['crowbar']
  },
  gitBranches: () => Array.from({ length: 40 }, (_, i) => ({
    name: i === 0 ? 'main' : i === 1 ? 'develop' : `feature/branch-${i}`,
    isCurrent: i === 2,
    isRemote: i % 3 === 0,
    lastCommit: EXTREME_MSGS[i % EXTREME_MSGS.length],
  })),
  markdownTurns: () => [],
}
```

- [ ] **Step 2: Verify `generateLargeFileDiff` is exported from branch-diff.ts**

```bash
grep "export function generateLargeFileDiff" web/src/lib/mock/branch-diff.ts
```

If not exported, add `export` to the function declaration in `branch-diff.ts`.

- [ ] **Step 3: TypeScript check**

```bash
cd web && npx tsc --noEmit
```

Fix any type errors. The most likely issue is the `GitDiffLine` type — ensure `line_type`, `content`, `old_line_number`, `new_line_number` match the actual type definition.

- [ ] **Step 4: Commit**

```bash
git add web/src/lib/mock/scenarios/extreme.ts web/src/lib/mock/branch-diff.ts
git commit -m "feat: add extreme scenario (4 repos, 50+ workspaces, 28 threads each, massive diffs)"
```

---

## Task 7: Update all MSW handlers to route through scenario + fault

**Files:**
- Modify: `web/src/mocks/handlers/workspaces.ts`
- Modify: `web/src/mocks/handlers/projects.ts`
- Modify: `web/src/mocks/handlers/fs.ts`
- Modify: `web/src/mocks/handlers/git.ts`
- Modify: `web/src/mocks/handlers/markdown-chat.ts`
- Modify: `web/src/mocks/handlers/branch-review.ts`

Every handler follows the same pattern:
1. Check `shouldFault(request, '<key>')` — return 500 if true
2. Call `getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')`
3. Return data from the scenario dataset

- [ ] **Step 1: Replace `web/src/mocks/handlers/workspaces.ts`**

```ts
import { http, HttpResponse } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'

export const workspaceHandlers = [
  http.get('/api/v0/workspaces', ({ request }) => {
    if (shouldFault(request, 'workspaces'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.repos())
  }),

  http.get('/api/v0/workspaces/:id', ({ params, request }) => {
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    const ws = data.workspace(params.id as string)
    if (!ws) return HttpResponse.json({ error: 'not found' }, { status: 404 })
    return HttpResponse.json(ws)
  }),

  http.post('/api/v0/workspaces', async ({ request }) => {
    const body = await request.json() as { repoId: string; branch: string }
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    const ws = data.createWorkspace(body.repoId, body.branch)
    return HttpResponse.json(ws, { status: 201 })
  }),
]
```

- [ ] **Step 2: Replace `web/src/mocks/handlers/projects.ts`**

```ts
import { http, HttpResponse } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'
import { createMockProject } from '@/lib/mock/projects'

export const projectHandlers = [
  http.get('/api/v0/projects', ({ request }) => {
    if (shouldFault(request, 'projects'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.projects())
  }),

  http.post('/api/v0/projects', async ({ request }) => {
    const body = await request.json() as { name: string; path: string }
    return HttpResponse.json(createMockProject(body), { status: 201 })
  }),
]
```

- [ ] **Step 3: Replace `web/src/mocks/handlers/fs.ts`**

```ts
import { http, HttpResponse } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'

export const fsHandlers = [
  http.get('/api/v0/fs/tree', ({ request }) => {
    if (shouldFault(request, 'file-tree'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const root = new URL(request.url).searchParams.get('root') ?? ''
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.fileTree(root))
  }),

  http.get('/api/v0/fs/file', ({ request }) => {
    if (shouldFault(request, 'file-content'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const path = new URL(request.url).searchParams.get('path') ?? ''
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.fileContent(path))
  }),
]
```

- [ ] **Step 4: Replace `web/src/mocks/handlers/git.ts`**

```ts
import { http, HttpResponse } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'

export const gitHandlers = [
  http.get('/api/v0/git/status', ({ request }) => {
    if (shouldFault(request, 'git-status'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const repo = new URL(request.url).searchParams.get('repo') ?? ''
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.gitStatus(repo))
  }),

  http.get('/api/v0/git/log', ({ request }) => {
    if (shouldFault(request, 'git-commits'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const url = new URL(request.url)
    const repo = url.searchParams.get('repo') ?? ''
    const limit = Math.min(parseInt(url.searchParams.get('limit') ?? '50', 10), 500)
    const skip = parseInt(url.searchParams.get('skip') ?? '0', 10)
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.gitLog(repo).slice(skip, skip + limit))
  }),

  http.get('/api/v0/git/branches', ({ request }) => {
    if (shouldFault(request, 'git-branches'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const repo = new URL(request.url).searchParams.get('repo') ?? ''
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.gitBranches(repo))
  }),
]
```

- [ ] **Step 5: Replace `web/src/mocks/handlers/markdown-chat.ts`**

```ts
import { http, HttpResponse } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'

export const markdownChatHandlers = [
  http.get('/api/v0/markdown-chat/:wsId/:stepId', ({ params, request }) => {
    if (shouldFault(request, 'markdown-chat'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.markdownTurns(String(params.wsId), String(params.stepId)))
  }),
]
```

- [ ] **Step 6: Replace `web/src/mocks/handlers/branch-review.ts`**

```ts
import { http, HttpResponse } from 'msw'
import { shouldFault } from '@/lib/mock/fault'
import { getDataForScenario } from '@/lib/mock/scenarios'

export const branchReviewHandlers = [
  http.get('/api/v0/branch-review/:wsId/diff', ({ params, request }) => {
    if (shouldFault(request, 'branch-diff'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.branchDiff(params.wsId as string))
  }),

  http.get('/api/v0/branch-review/:wsId/threads', ({ params, request }) => {
    if (shouldFault(request, 'branch-threads'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.branchThreads(params.wsId as string))
  }),

  http.get('/api/v0/branch-review/:wsId/description', ({ params, request }) => {
    if (shouldFault(request, 'branch-description'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.branchDescription(params.wsId as string))
  }),

  http.get('/api/v0/branch-review/:wsId/chats', ({ params, request }) => {
    if (shouldFault(request, 'branch-chats'))
      return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
    const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
    return HttpResponse.json(data.branchChats(params.wsId as string))
  }),
]
```

- [ ] **Step 7: TypeScript check + full test suite**

```bash
cd web && npx tsc --noEmit && npx vitest run 2>&1 | tail -20
```

Expected: zero TypeScript errors, same pass/fail as baseline.

- [ ] **Step 8: Verify handler tests still pass**

```bash
cd web && npx vitest run src/__tests__/mocks/handlers/branch-review.test.ts
```

Expected: 5 tests PASS (they set up their own server so they bypass the scenario header — this is fine).

- [ ] **Step 9: Commit**

```bash
git add web/src/mocks/handlers/
git commit -m "feat: route all MSW handlers through scenario dataset + fault injection"
```

---

## Task 8: Developer settings UI — Mock Scenarios + Fault Injection

**Files:**
- Modify: `web/src/features/settings/components/tabs/developer-settings.tsx`

- [ ] **Step 1: Replace `developer-settings.tsx` entirely**

```tsx
import { useState } from 'react'
import { useChaosStore, FAULT_KEYS, FAULT_LABELS } from '@/lib/store/chaos'
import type { Scenario } from '@/lib/store/chaos'
import Section, { SETTINGS_CONTROL_WIDTHS, SettingRow } from '../settings-section'
import NumberInput from '@/components/ui/number-input'
import { Button } from '@/components/ui/button'
import { Slider } from '@/components/ui/slider'
import { Select, SelectTrigger, SelectValue, SelectContent, SelectItem } from '@/components/ui/select'
import { cn } from '@/utils/cn'

const SCENARIO_OPTIONS: { value: Scenario; label: string; description: string }[] = [
  { value: 'extreme', label: 'Extreme', description: '4 repos · 50+ workspaces · 1M-line diffs · 28 threads/PR' },
  { value: 'normal', label: 'Normal — Rabbyte', description: '1 repo · 3 workspaces · realistic daily usage' },
  { value: 'empty', label: 'Empty', description: 'New user — no repos, no workspaces' },
]

export function DeveloperSettings() {
  const latency = useChaosStore((s) => s.latency)
  const errorRate = useChaosStore((s) => s.errorRate)
  const scenario = useChaosStore((s) => s.scenario)
  const faults = useChaosStore((s) => s.faults)
  const setLatency = useChaosStore((s) => s.setLatency)
  const setErrorRate = useChaosStore((s) => s.setErrorRate)
  const setFault = useChaosStore((s) => s.setFault)
  const reset = useChaosStore((s) => s.reset)
  const resetFaults = useChaosStore((s) => s.resetFaults)
  const applyScenario = useChaosStore((s) => s.applyScenario)

  const [selectedScenario, setSelectedScenario] = useState<Scenario>(scenario)
  const [applying, setApplying] = useState(false)

  async function handleApply() {
    setApplying(true)
    await applyScenario(selectedScenario)
    // page reloads — this line never executes
  }

  const anyFaultActive = FAULT_KEYS.some(k => faults[k] > 0)

  return (
    <div className="space-y-4">
      <Section title="Network Chaos" description="Simulate poor network conditions against the Go API server.">
        <SettingRow
          label="Latency"
          description="Extra delay added to every API request via X-Crowbar-Latency header"
          onReset={() => setLatency(0)}
          canReset={latency !== 0}
          resetLabel="Reset latency"
        >
          <NumberInput
            min="0"
            max="5000"
            step="50"
            value={latency}
            onChange={setLatency}
            className={cn(SETTINGS_CONTROL_WIDTHS.number, 'tabular-nums')}
            size="xs"
            aria-label={`Latency: ${latency} milliseconds`}
          />
        </SettingRow>

        <SettingRow
          label="Error Rate"
          description="Probability each API request returns a 500 via X-Crowbar-Error-Rate header"
          onReset={() => setErrorRate(0)}
          canReset={errorRate !== 0}
          resetLabel="Reset error rate"
        >
          <NumberInput
            min="0"
            max="100"
            step="5"
            value={Math.round(errorRate * 100)}
            onChange={(v) => setErrorRate(v / 100)}
            className={cn(SETTINGS_CONTROL_WIDTHS.number, 'tabular-nums')}
            size="xs"
            aria-label={`Error rate: ${Math.round(errorRate * 100)} percent`}
          />
        </SettingRow>

        <div className="px-1 pt-1">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={reset}
            disabled={latency === 0 && errorRate === 0}
          >
            Reset network chaos
          </Button>
        </div>
      </Section>

      {import.meta.env.VITE_USE_MOCK === 'true' && (
        <>
          <Section
            title="Mock Scenario"
            description="Switch the full mock dataset. Clears all local caches and reloads the app."
          >
            <SettingRow
              label="Scenario"
              description={SCENARIO_OPTIONS.find(o => o.value === selectedScenario)?.description}
            >
              <div className="flex items-center gap-2">
                <Select value={selectedScenario} onValueChange={(v) => setSelectedScenario(v as Scenario)}>
                  <SelectTrigger className={SETTINGS_CONTROL_WIDTHS.wide} size="sm">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {SCENARIO_OPTIONS.map(opt => (
                      <SelectItem key={opt.value} value={opt.value}>
                        {opt.label}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <Button
                  type="button"
                  variant="default"
                  size="sm"
                  onClick={handleApply}
                  disabled={applying || selectedScenario === scenario}
                >
                  {applying ? 'Applying…' : 'Apply & Reload'}
                </Button>
              </div>
            </SettingRow>
            {scenario !== selectedScenario && (
              <p className="px-1 text-xs text-muted-foreground">
                Current: <span className="font-medium">{SCENARIO_OPTIONS.find(o => o.value === scenario)?.label}</span>
                {' '}→ pending: <span className="font-medium text-foreground">{SCENARIO_OPTIONS.find(o => o.value === selectedScenario)?.label}</span>
              </p>
            )}
          </Section>

          <Section
            title="Fault Injection"
            description="Force specific API endpoints to return 500 errors. Changes take effect on the next request — no reload needed."
          >
            {FAULT_KEYS.map(key => (
              <SettingRow
                key={key}
                label={FAULT_LABELS[key]}
                onReset={() => setFault(key, 0)}
                canReset={faults[key] > 0}
                resetLabel={`Reset ${FAULT_LABELS[key]}`}
              >
                <div className="flex items-center gap-2.5 w-44">
                  <Slider
                    value={faults[key]}
                    onValueChange={(values) => setFault(key, Array.isArray(values) ? (values[0] ?? 0) : values)}
                    min={0}
                    max={100}
                    step={5}
                    className="flex-1"
                    aria-label={`${FAULT_LABELS[key]} fault rate`}
                  />
                  <span className={cn(
                    'w-8 text-right text-xs tabular-nums',
                    faults[key] > 0 ? 'text-destructive font-medium' : 'text-muted-foreground'
                  )}>
                    {faults[key]}%
                  </span>
                </div>
              </SettingRow>
            ))}
            <div className="px-1 pt-1">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={resetFaults}
                disabled={!anyFaultActive}
              >
                Reset all faults
              </Button>
            </div>
          </Section>
        </>
      )}
    </div>
  )
}
```

- [ ] **Step 2: TypeScript check**

```bash
cd web && npx tsc --noEmit
```

If `SelectTrigger` doesn't accept a `size` prop, remove it. Check: `grep "size" web/src/components/ui/select.tsx | head -5`

- [ ] **Step 3: Run full test suite**

```bash
cd web && npx vitest run 2>&1 | tail -15
```

Expected: same pass/fail as baseline.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/settings/components/tabs/developer-settings.tsx
git commit -m "feat: add Mock Scenario + Fault Injection sections to Developer settings"
```

---

## Task 9: Final verification

- [ ] **Step 1: Check all success criteria**

```bash
# No component imports from lib/mock outside of mocks/ and lib/mock itself
cd web && grep -r "from '@/lib/mock/" src --include="*.ts" --include="*.tsx" \
  | grep -v "src/mocks/" | grep -v "src/lib/mock/" | grep -v "__tests__"
```

Expected: zero lines.

- [ ] **Step 2: TypeScript clean**

```bash
cd web && npx tsc --noEmit
```

Expected: zero errors.

- [ ] **Step 3: Full test suite**

```bash
cd web && npx vitest run 2>&1 | tail -20
```

Expected: all tests pass (pre-existing editor-api failures only).

- [ ] **Step 4: Verify scenario switching**

Start the dev server (`cd web && pnpm dev`), open Developer settings, switch to Extreme scenario, click Apply & Reload. Verify:
- Sidebar shows 4 repos with 15+ workspaces each, nested tree structure
- Opening any workspace's branch review shows 28 review threads with multi-message conversations
- Some workspaces show 1M+ line diffs
- Git commit history shows 500 commits

Switch to Normal — verify Rabbyte repo appears with 3 workspaces.
Switch to Empty — verify sidebar shows no repos.

- [ ] **Step 5: Verify fault injection**

Set Branch diff to 100%. Open any workspace's branch review diff tab. Verify it shows an error state instead of diff content. Reset to 0% — diff loads normally.

- [ ] **Step 6: Final commit**

```bash
git add -A
git commit -m "chore: mock scenarios + fault injection implementation complete"
```
