# Mock Scenarios + Fault Injection — Design Spec

**Date:** 2026-06-01
**Status:** Approved — proceed to implementation

---

## Problem

The app now loads all data from the API layer (MSW in dev). This is correct, but it means developers have no way to see the app in different states — a power user with many repos, a new empty user, or a user whose branch diff API is broken. This spec defines a system to switch between three rich mock scenarios and inject per-component failures, all from the existing Developer settings panel.

---

## Architecture Overview

```
Chaos store (Zustand + persist)
  ├── latency         — existing, ms added to every request
  ├── errorRate       — existing, global % fail
  ├── scenario        — NEW: 'extreme' | 'normal' | 'empty'
  └── faults          — NEW: Record<FaultKey, number>  (0–100 %)

apiFetch (lib/api.ts)
  └── adds X-Crowbar-Scenario + X-Crowbar-Fault headers when VITE_USE_MOCK=true

lib/mock/scenarios/
  ├── index.ts        — ScenarioDataset type + getDataForScenario(name)
  ├── extreme.ts      — extreme dataset (self-contained)
  ├── normal.ts       — normal/Rabbyte dataset (wraps existing mock fns)
  └── empty.ts        — empty state dataset

lib/mock/fault.ts     — shouldFault(request, key) helper

MSW handlers          — each reads scenario header, routes to dataset; reads fault header, may 500

Developer settings UI — two new sections gated behind VITE_USE_MOCK:
  ├── Mock Scenarios  — scenario picker + Apply & Reload button
  └── Fault Injection — per-component failure % sliders
```

---

## Section 1 — Chaos Store

### Extended shape

```ts
export type Scenario = 'extreme' | 'normal' | 'empty'

export type FaultKey =
  | 'workspaces' | 'projects'
  | 'file-tree' | 'file-content'
  | 'branch-diff' | 'branch-threads' | 'branch-description' | 'branch-chats'
  | 'git-commits' | 'git-status' | 'git-branches'
  | 'markdown-chat'

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
  applyScenario: (s: Scenario) => Promise<void>
}
```

### `persist` middleware required

`scenario` must survive across page reloads because `applyScenario` sets the scenario then immediately calls `window.location.reload()`. Without persist, the reload would reset scenario back to 'normal' before `HydrationGate` fetches.

Persist key: `'crowbar.chaos'`. Partialize to only `{ scenario, faults }` — latency and errorRate are ephemeral per session.

### `applyScenario` action

```ts
applyScenario: async (newScenario) => {
  // 1. Persist new scenario before reload
  set({ scenario: newScenario })

  // 2. Clear all IDB stores that hold scenario-dependent data
  const db = await getDB()
  await Promise.all([
    db.clear('query-cache'),       // React Query cache
    db.clear('branch-review'),     // threads, description, merge strategy
    db.clear('workspace-hierarchy'),
    db.clear('sidebar-ui'),
  ])

  // 3. Reload — HydrationGate will fetch with the new scenario header
  window.location.reload()
}
```

---

## Section 2 — apiFetch Header Injection

`lib/api.ts` — add to `apiFetch` when `VITE_USE_MOCK=true`:

```ts
if (import.meta.env.VITE_USE_MOCK === 'true') {
  const { scenario, faults } = useChaosStore.getState()
  chaosHeaders['X-Crowbar-Scenario'] = scenario
  const active = Object.entries(faults).filter(([, v]) => v > 0)
  if (active.length > 0) {
    chaosHeaders['X-Crowbar-Fault'] = JSON.stringify(Object.fromEntries(active))
  }
}
```

The real backend receives unknown headers and ignores them. No conditional needed — the headers are always sent in mock mode.

---

## Section 3 — Scenario Dataset Interface

```ts
// lib/mock/scenarios/index.ts
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
```

`getDataForScenario(scenario: string): ScenarioDataset` — returns the right dataset, defaulting to normal.

---

## Section 4 — Fault Injection Helper

```ts
// lib/mock/fault.ts
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

Every MSW handler wraps its response:
```ts
if (shouldFault(request, 'branch-diff'))
  return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
```

---

## Section 5 — Scenario Datasets

### Empty scenario (`lib/mock/scenarios/empty.ts`)

All methods return empty values:
- `repos()` → `[]`
- `projects()` → `[]`
- `workspace()` → `undefined`
- `branchDiff()` → `{ commitHash: '', commitMessage: '', files: [], totalFiles: 0, totalAdditions: 0, totalDeletions: 0 }`
- `branchThreads()` → `[]`
- `branchDescription()` → `''`
- `branchChats()` → `[]`
- `gitLog()` → `[]`
- `gitBranches()` → `[]`
- `markdownTurns()` → `[]`
- `fileTree()` → empty root node
- `fileContent()` → `''`

### Normal scenario (`lib/mock/scenarios/normal.ts`)

**Repo: Rabbyte** — 1 repo, 3 workspaces:
- `rb-develop` (locked, develop branch)
- `rb-auth` (feature/onboarding, pr-open, +340 -12)
- `rb-fix` (fix/signup-form, new, +23)

Diffs: realistic 5–10 file changes. 3–5 review threads per workspace, 2–3 messages each (including at least one AI reviewer comment). 15–20 commits. 2 branch chats per workspace.

The normal scenario wraps the existing `lib/mock/` functions where appropriate, or provides fresh Rabbyte-themed data.

### Extreme scenario (`lib/mock/scenarios/extreme.ts`)

**4 repos** with 15+ workspaces each, 3–4 nesting levels deep:
- `crowbar` — 16 workspaces
- `quiver-core` — 14 workspaces
- `quiver-desktop` — 12 workspaces
- `quiver-cloud` — 11 workspaces

**Diffs**: Every workspace has a diff. At least 3 workspaces have 1M+ line diffs (using the existing `generateLargeFileDiff` helper). Others have 50–200 file changes.

**Review threads — must be abundant:**
- Minimum 25 threads per workspace in extreme scenario
- Each thread has 4–8 messages
- Mix of human and AI reviewer (`isAgent: true`) messages
- Threads spread across multiple files, various line numbers
- Some resolved, some open, some with contentious multi-turn debates
- Include threads that reference code bugs, security concerns, performance issues, naming nitpicks

**Commits**: 250+ commits per workspace. Commit messages are realistic (feat/fix/chore/refactor patterns).

**Branch chats**: 8–10 per workspace, mix of active/inactive.

**Markdown turns**: 10+ pre-seeded turns per workspace chat including multi-turn technical debates.

---

## Section 6 — MSW Handler Updates

Every handler gains the same two-line prefix:

```ts
if (shouldFault(request, '<key>'))
  return HttpResponse.json({ error: 'simulated failure' }, { status: 500 })
const data = getDataForScenario(request.headers.get('X-Crowbar-Scenario') ?? 'normal')
```

Then calls `data.<method>(params)` instead of the old direct mock call.

Handlers to update: `workspaces.ts`, `projects.ts`, `fs.ts`, `git.ts`, `markdown-chat.ts`, `branch-review.ts`, `conversations.ts`.

---

## Section 7 — Developer Settings UI

### Placement

Two new `Section` blocks added to `DeveloperSettings`, wrapped:
```tsx
{import.meta.env.VITE_USE_MOCK === 'true' && (
  <>
    <MockScenariosSection />
    <FaultInjectionSection />
  </>
)}
```

### MockScenariosSection

```
Mock Scenarios
  "Simulates different user states. Applying clears all local caches and reloads."

  Scenario   [Normal ▾]   [Apply & Reload]
```

- `Select` / `SelectTrigger` / `SelectValue` / `SelectContent` / `SelectItem`
- Options: `{ value: 'extreme', label: 'Extreme — power user' }`, `{ value: 'normal', label: 'Normal — Rabbyte project' }`, `{ value: 'empty', label: 'Empty — new user' }`
- Button calls `useChaosStore.getState().applyScenario(selected)`
- Button is disabled while applying (async — brief spinner before reload)
- `SettingRow` layout: label="Scenario", children=`<Select> + <Button>`

### FaultInjectionSection

```
Fault Injection
  "Force specific API endpoints to return 500 errors at the given probability."

  Workspaces          [slider]  0%
  Projects            [slider]  0%
  File tree           [slider]  0%
  File content        [slider]  0%
  Branch diff         [slider]  0%
  Branch threads      [slider]  0%
  Branch description  [slider]  0%
  Branch chats        [slider]  0%
  Git commits         [slider]  0%
  Git status          [slider]  0%
  Git branches        [slider]  0%
  Markdown chat       [slider]  0%

  [Reset all faults]
```

- Each row: `SettingRow` with label + `<Slider>` (0–100, step 5) + numeric display `"XX%"`
- `Slider` from `@/components/ui/slider`, `SliderValue` for the label
- `canReset` / `onReset` per row when value > 0
- Global "Reset all faults" button — calls `useChaosStore.getState().reset()` (or a new `resetFaults` action)
- Changes take effect immediately on next request (no reload needed)

---

## Section 8 — Files Changed

| File | Action |
|------|--------|
| `lib/store/chaos.ts` | Extend with scenario, faults, persist, applyScenario |
| `lib/api.ts` | Add scenario + fault headers in mock mode |
| `lib/mock/fault.ts` | Create — shouldFault helper |
| `lib/mock/scenarios/index.ts` | Create — ScenarioDataset type + router |
| `lib/mock/scenarios/extreme.ts` | Create — extreme dataset |
| `lib/mock/scenarios/normal.ts` | Create — normal/Rabbyte dataset |
| `lib/mock/scenarios/empty.ts` | Create — empty dataset |
| `mocks/handlers/workspaces.ts` | Route through scenario + fault |
| `mocks/handlers/projects.ts` | Route through scenario + fault |
| `mocks/handlers/fs.ts` | Route through scenario + fault |
| `mocks/handlers/git.ts` | Route through scenario + fault |
| `mocks/handlers/markdown-chat.ts` | Route through scenario + fault |
| `mocks/handlers/branch-review.ts` | Route through scenario + fault |
| `features/settings/components/tabs/developer-settings.tsx` | Add two new sections |

---

## Success Criteria

1. Switching to **Extreme** and applying shows 4 repos with 15+ workspaces each in the sidebar
2. Opening any branch-review in Extreme shows a large diff AND 25+ review threads with multi-message conversations
3. Switching to **Normal** shows only the Rabbyte repo with 3 workspaces
4. Switching to **Empty** shows "no repos" empty state
5. Setting Branch diff fault to 100% makes the diff panel show an error state instead of content
6. Setting Workspaces fault to 100% makes the sidebar empty (fetch fails)
7. Changing scenario clears IDB and reload shows fresh data — no stale threads from previous scenario bleed through
8. With `VITE_USE_MOCK=false`, the Scenario and Fault Injection sections do not render
9. Real backend dev mode: `X-Crowbar-Scenario` header sent but backend ignores it gracefully
