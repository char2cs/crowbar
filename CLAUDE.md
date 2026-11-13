# Crowbar — Developer Conventions

## Fixing bugs
- A fix starts with a failing test that reproduces it.
- Never fix a symptom with a timer, poll, retry, extra flag, or resync effect.
  Find the owner of the state. If the fix needs a second copy of some state,
  stop and write a plan in docs/plans/ instead.
- A known bug is an issue, not a `t.Skip`.

## Hygiene
- Delete code you make unused in the same change. No exports kept "for API
  completeness"; deadcode/knip must stay clean.
- No stubs that ship: a UI must not be wired to a method that returns [] or false.
- Comments explain *why* in ≤3 lines — no incident narratives or PR history.
- Don't commit plans-as-scratch, QA logs, spikes, test output, placeholders.
- No second library for a job already covered (icons: phosphor; DnD: dnd-kit;
  headless UI: base-ui; highlighting: shiki).

## Tests
- Test behaviour through the public API; never add a test only for coverage.
- Extend the unit's existing test file instead of `<unit>-<scenario>.test.ts`
  (CI rejects new `*_coverage_test.go`, `*_extra_test.go`, `*_gaps_test.go`).
- ≤3 `vi.mock` per file; no asserting Tailwind class names or reading src/ text.
- No sleeps: channels, `require.Eventually`, `waitFor`, fake timers.

## Lint suppressions
- Every nolint / eslint-disable / react-doctor-disable / ts-expect-error names
  the rule and gives a reason on the same line. Never loosen a lint config,
  coverage floor, or budget to get green.
- Baselines (`api/deadcode-baseline.txt`, `web/knip.json` ignoreIssues, the
  `*_BASELINE` lists in `web/eslint.config.js`, golangci-lint's `new-from-rev`)
  only shrink: fix an entry, delete it; never add one.

## Go (`api/`)
- Build and test with `-tags noEmbed`; lint with `make lint` (new code) and
  `make lint-backlog` (everything); `make deadcode` for the dead-code gate.
- No `...ForTest` methods or test-only package vars in non-test files; use
  export_test.go or an injected config/clock.
- Required dependencies are constructor arguments, not nil-checked setters.
- A test that needs file permissions enforced calls
  `testutil.RequirePermissionEnforcement(t)` (skips as root and on Windows).

# Web (`web/`)

Checks CI runs: `bunx eslint . --max-warnings 0`, `bun run knip` (dead code),
`bunx tsc --noEmit`, `bunx prettier --check .`, `bun run test:coverage`,
`bun run lint:doctor` (react-doctor; CI blocks on warnings).

## Test file location

All test files live in `web/src/__tests__/` mirroring the `web/src/` structure.

**Rule:** A test for `web/src/features/X/lib/foo.ts` goes in `web/src/__tests__/features/X/lib/foo.test.ts`.

Do **not** create `features/X/tests/` directories — the co-located pattern was retired in favour of the mirror structure.

Use `@/` imports (not relative `../../`) inside test files so they don't break when moved.

## Component file naming

All component files use **kebab-case**: `my-component.tsx`, not `MyComponent.tsx`.

The exported React component name remains PascalCase:
```tsx
// file: my-component.tsx
export function MyComponent() { ... }
```

## Store patterns

- Use `useXxxStore((state) => state.specificField)` with a **narrow selector** — never `useXxxStore()` with no selector.
- Use `useXxxStore.getState()` only inside event handlers and `useEffect` bodies — never in the component render path.
- Stores must not import from `components/` — move side effects (toasts, DOM interactions) to components that watch store state via `useEffect`.

## State management

- Per-workspace state lives in the workspace store registry (`features/workspace/stores/`).
- Global app state lives in `features/window/stores/` or `features/settings/`.
- `lib/store/` is for server-state-adjacent structures (conversations, projects sidebar).

## Go daemon (`api/`)

Layout and flow are described in `api/ARCHITECTURE.md`. Enforced by `api/.golangci.yml` (funlen 100 lines/50 statements, gocyclo 15, nestif 2, revive early-return, gofumpt) and `make pr-checks`:

- Implementation details live in `internal/` sub-packages; one domain concept per file; source files under 500 lines.
- One test file per source file (`foo.go` → `foo_test.go`), struct-only files excepted.
- Early returns over `else`; at most three indentation levels per function.
- Wrap errors with context: `fmt.Errorf("op: ctx: %w", err)`; check every error.
- Tests are deterministic: no `time.Sleep` — wait on a channel, a WaitGroup or a condition with a deadline (`tests/kit` watchers).
- Benchmarks (`*_bench_test.go`) for hot paths; build and test with `-tags noEmbed`.
