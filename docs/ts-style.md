# TypeScript style manifest

Applies to `app/panes` (React, Vite). It is the TypeScript translation of the owner's Rabbyte manifest: the language-independent rules carry over, the Go-only rules are replaced by the TS mechanism that does the same job. The project rules in `CLAUDE.md` (web section) stay in force; where they differ, the stricter one wins. Rust: `docs/rust-style.md`. Swift: `docs/swift-style.md`. Review: `docs/plans/2026-11-14-code-review-checklist.md`.

Scope: the script checks files that are new on this branch or edited in the working tree. Legacy files that moved from `web/` unchanged are not scanned, so old code does not drown the signal; touching one puts it under the rules.

Enforcement tags: **eslint** = `bunx eslint . --max-warnings 0` (`app/panes/eslint.config.js`); **tsc** = `bunx tsc --noEmit` (strict); **script** = `tools/scripts/check-ts-rules.sh`; **knip** = `bun run knip`; **review** = reviewer agent.

Sources: [typescript-eslint rules](https://typescript-eslint.io/rules/) (`no-explicit-any`, `ban-ts-comment`, `no-non-null-assertion`, `no-unused-vars`), [typescript-eslint: avoiding anys](https://typescript-eslint.io/blog/avoiding-anys/), [React: you might not need an effect](https://react.dev/learn/you-might-not-need-an-effect), [React: keeping components pure](https://react.dev/learn/keeping-components-pure), [Rules of Hooks](https://react.dev/reference/rules), [knip](https://knip.dev), Clean Code (R. Martin).

| Manifest rule | TS rule | Enforcement |
|---|---|---|
| M1/M30 small functions, few params | At most 4 parameters (an options object beyond that); a function does one thing and fits a screen. | eslint `max-params` 4, `max-lines-per-function`; review |
| M2/M3 shallow nesting, early returns | At most 2 nesting levels in a function body; guard clauses; no `else` after `return`/`throw`. | eslint `max-depth` 2, `no-else-return`; review |
| M5 named conditions | A compound condition is bound to a named `const` before the `if`. | review |
| M6 iterators | `map`/`filter`/`find`/`some`/`reduce` over hand loops. | eslint `unicorn/no-for-loop`; review |
| M7 comments | No narrative comments. A `//` comment explains why, in at most 3 consecutive lines. No block comments, no commented-out code, no incident or PR history. TODO/FIXME only with an issue reference (`#123` or URL). | script `comment`, `commented-code`, `todo` |
| M8 one concept per file | One exported component, class or store per file, named after it in kebab-case (`MyComponent` in `my-component.tsx`). Types a component needs may share its file. | script `multi-concept`, `file-name` |
| M9/M26/M27 errors | Errors are values or typed `Error` subclasses carrying the failed operation and cause (`new Error('load project: ...', { cause })`). No empty `catch`, no swallowed rejection, no floating promise. | eslint `no-empty`, `@typescript-eslint/no-floating-promises`, `SWALLOWED_FAILURE_RULES`; review |
| M10 entry per module | Each module exports one entry named like the file; `index.ts` is only for a directory that is a real public surface. | review |
| M14/M15 public surface | Export only what another module imports (knip enforces); internals are not exported. | knip |
| M16/M17 layering | `lib/` and `sdk/` never import from `components/` or `features/`; stores never import `components/` (CLAUDE.md). | eslint `no-restricted-imports`; review |
| M21/M29 naming | Intent-revealing names, no abbreviations; files kebab-case; hooks `useX`; booleans read as questions (`isReady`). | script `file-name`; review |
| M22 banned names | No file or directory named `util(s)`, `common`, `helper(s)`, `base`, `misc` in new files. Name the concept. | script `module-name` |
| M24 no globals | No module-level `let`/`var`, no module-level `new Map/Set`, no `window.x =`/`globalThis.x =`. State lives in a store, a class instance or a hook and is passed in. | script `global` |
| M28/M38 injected dependencies | Collaborators (clock, socket, storage, fetch) are parameters or constructor arguments, never imported singletons reached for inside logic. | review |
| M32 no magic values | Named `const` for numbers and strings with meaning. | review |
| M33 no flag parameters | An enum/union or two functions instead of a boolean parameter. | review |
| M36 duplicate once | Extract on the third repetition. | review |
| M37 no dead code | Delete code you make unused in the same change; knip stays clean; no `export` kept for completeness. | knip, eslint `no-unused-vars` |
| types | No `any` (use `unknown` and narrow), no `!` non-null assertion, no `as` casts to widen. `@ts-ignore` and `@ts-nocheck` are banned; `@ts-expect-error` and `eslint-disable` carry the rule and ` -- reason` on the same line. | script `any`, `non-null`, `suppress`; eslint `ban-ts-comment`, `no-explicit-any` |
| no barrels | No `export * from`; import from the defining module. An index file that only re-exports is rejected. | script `barrel` |
| no console | No `console.log/debug/info` in source; use the report/log path. | script `console` |
| M39-M41 tests | Behaviour through the public API; table-driven (`it.each`); at most 3 `vi.mock` per file; no asserting class names or reading `src/` text; no real sleeps (fake timers, `waitFor`, events); no `.skip`/`.only` (a known bug is an issue). | script `mock-count`, `test-style`, `sleep`, `test-skip` |
| M42/M43 test files | A test for `src/X/foo.ts` is `src/__tests__/X/foo.test.ts` (CLAUDE.md mirror layout); no `<unit>-<scenario>.test.ts`; a new source file with functions has its test file; no orphan tests; tests outside `__tests__` are rejected. Exemptions are listed in `tools/scripts/ts-test-exemptions.txt` and only shrink. | script `test-missing`, `test-orphan`, `test-scenario`, `test-location` |
| M44/M45 size | A file is at most 500 lines; split by concept, never by trimming. | script `size` |

## How to run

```
tools/scripts/check-ts-rules.sh              # changed files vs develop plus working tree
tools/scripts/check-ts-rules.sh --all        # every file under src/ and entries/
tools/scripts/check-ts-rules.sh --self-test  # checker against tools/scripts/testdata/ts-rules
tools/scripts/check-ts-rules.sh --full       # also eslint, tsc, prettier on changed files, in the inventory-app container on a /tmp copy
tools/scripts/check-standards.sh             # Rust + Swift + TS in one run (add --self-test, --full)
```
