# Crowbar Web — Developer Conventions

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
