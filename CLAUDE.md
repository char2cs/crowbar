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
