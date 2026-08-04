# Crowbar — Developer Conventions

## ⚠️ Where this repo is going: `native/` + `api/`, and nothing else

Crowbar is migrating to a **Rust-native GPUI desktop app**. When it lands, the
repo holds two things:

| directory | fate |
|---|---|
| **`native/`** | ✅ **the app.** Rust + GPUI. The only frontend. |
| **`api/`** | ✅ **the daemon.** Go. Survives unchanged; `native/` is its client. |
| `web/` | ❌ **legacy — will be deleted.** The React frontend. |
| `desktop/` | ❌ **legacy — will be deleted.** The Tauri shell. |

**This is a removal, not a coexistence.** `web/` and `desktop/` are not being
refactored, modernised or gradually absorbed — they are being **deleted** once
`native/` reaches parity. Read that as an instruction about where effort goes.

### What follows from it

- **Do not add features to `web/` or `desktop/`.** Fixes to keep them working
  during the migration are fine; new capability belongs in `native/`.
- **`native/` must not depend on `web/` or `desktop/`.** Anything reaching
  outside `native/` is a thing that breaks on deletion day. `check-invariants.sh`
  enforces this for `native/crates/`.
  - The **only** exceptions are port-time parity tools whose whole job is
    comparing against the reference app (`crowbar-ui/tools/gen-theme.py`,
    `scripts/gen-extract.ts`). They are exempted **by path**, marked in their own
    headers, and die with `web/`. Nothing in a shipping build may depend on them.
- **`native/` → `api/` is legitimate and permanent.** The daemon survives.
- **Never build migration bridges between the two frontends.** A shared crate
  consumed by both points the dependency backwards — the codebase with a delete
  date owning an edge into the one that survives. Duplicate instead; the copy in
  `web/`/`desktop/` is going away. (This repo already made that mistake once,
  with the daemon sidecar.)

### How the port is run

Work proceeds in **user-reviewed vertical slices**, not component-by-component.
A slice is done when it runs in the real binary, against the real daemon, and the
user has seen it beside the React app and accepted it.

- Spec: `docs/superpowers/specs/2026-08-04-slice-based-port-method-design.md`
- Crate rules, the `PATH` gotcha, and the four gates: `native/README.md`
- **`native/QUEUE.md` is a findings ARCHIVE, not a work queue.** It documents the
  retired component-parity method. Its recorded hazards are still true; its item
  list is not. Do not resume work from it.

---

# Legacy: `web/` — React frontend conventions

**Everything below applies to `web/`, which is scheduled for deletion.** It is
kept so the app stays coherent until `native/` replaces it. Do not use these
conventions as a model for `native/` — that is a Rust workspace with its own
rules in `native/README.md`.

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
