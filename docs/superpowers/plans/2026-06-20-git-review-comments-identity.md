# Git Review FE — Inline Comments + Markdown + GitHub Identity (Plan 4 of 4)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use `- [ ]`.

**Goal:** GitHub-style inline comment threads on the react-diff-view review surface: gutter "+" + range-select to anchor a thread to a line/range, threads render inline as widgets, expand into human↔agent conversations with **Markdown (Write/Preview)** and **real GitHub identities** (name/@login/avatar), wired to the first-class `/threads` REST + live WebSocket. Retire the two legacy comment prototypes.

**Architecture:** Comments anchor via **react-diff-view's `widgets` (keyed by `getChangeKey`) + `renderGutter`** (NOT the old custom line renderer). The data layer realigns `review-api` to `/threads` and the `branchReview` slice gains range + two-way-resolve + upsert; a `/threads` WS subscription (mirroring `git/status`) keeps threads live. A new backend identity endpoint (`gh api user` + git-config fallback) feeds the composer/cards.

**Tech Stack:** Go (identity endpoint), React 19, zustand, react-diff-view, react-markdown + remark-gfm + **shiki**, vitest + Go integration tests.

## Global Constraints
- `@/components/ui/*` + CSS tokens only; no hardcoded colors. AGENT badge + identity avatars via tokens.
- Files kebab-case; narrow selectors; `getState()` only in handlers/effects. Per-workspace state from the registry on global surfaces.
- `/threads` paths (entity-scoped under `workspaceBase(wsId)`): `POST /threads` `{filePath,line,startLine,endLine,side,author,isAgent,body}`; `POST /threads/:id/replies` `{author,isAgent,body}`; `PATCH /threads/:id` `{isResolved}` (two-way); `GET /threads` list (also a WS stream). `side` is `'old'|'new'` end-to-end (map left→old, right→new).
- FE tests mirror under `web/src/__tests__/`; run `cd web && ./node_modules/.bin/vitest run <path>` (pnpm off PATH; **never `npm install`** here — use pnpm if a dep is needed). Go: `cd api && go test -tags integration ./tests/... -run <Name>`.
- Verify live in Tauri before claiming done (Task 6). The dev daemon must be the rebuilt Plan-1 binary; vite must be running.

---

### Task 1: Backend — current GitHub/git identity endpoint

**Files:**
- Create: `api/internal/engine/git/identity.go` (or a provider method) — `CurrentIdentity(ctx, repoPath) (Identity, error)`
- Create: `api/internal/domain/git/identity.go` — `Identity { Login, DisplayName, AvatarURL string }`
- Create/extend: a v0 endpoint `GET .../workspaces/:wsId/identity` (or repo-scoped `.../identity`) + DTO + route registration
- Test: `api/tests/identity_test.go` (`//go:build integration`)

**Interfaces produced:** `GET .../identity` → `{ login, displayName, avatarUrl }`. Resolution: try `gh api user --jq '{login,name,avatar_url}'` (or parse JSON) → `{login, name→displayName, avatar_url→avatarUrl}`; on error/absence fall back to `git -C <repo> config user.name` (→ displayName, login/avatar empty). Never error the request — always return a best-effort identity (empty fields allowed).

- [ ] **Step 1:** Write the failing integration test: import a writable workspace (its repo sets `git config user.name "t"`), `GET .../identity`, assert 200 with a non-empty `displayName` (works whether `gh` is present or not — the git-config fallback guarantees "t"). Run; confirm fail (route 404).
- [ ] **Step 2:** Implement the engine `CurrentIdentity` (gh-first, git-config fallback, no hard error), the domain `Identity`, the DTO + handler + route (mirror an existing simple GET endpoint's wiring). Wire into the router/container.
- [ ] **Step 3:** Run the test; pass. `cd api && go build ./internal/... && go vet ./internal/...` clean. Commit: `feat(api): current GitHub/git identity endpoint`.

---

### Task 2: FE — realign `/threads` API + slice model + identity client

**Files:**
- Modify: `web/src/features/git/api/review-api.ts` (thread methods → `/threads`; add `listThreads`); `web/src/lib/types.ts` (`ThreadDTO`/`ThreadReplyDTO` add `startLine`,`endLine`,`isAgent`)
- Modify: `web/src/features/workspace/stores/slices/branch-review-slice.ts` (`ReviewThread` + `startLine`/`endLine`, `side:'old'|'new'`; `setReviewThreadResolved(id, isResolved)` two-way; `upsertReviewThread(thread)`; keep optimistic helpers)
- Create: `web/src/features/git/api/identity-api.ts` + `web/src/features/git/hooks/use-current-identity.ts` (fetch + cache the identity for the active ws)
- Test: `web/src/__tests__/features/git/api/review-api.test.ts` (extend), `branch-review-slice.test.ts` (extend)

**Interfaces produced:**
- `openThread(wsId, {filePath,line,startLine,endLine,side,author,isAgent,body})` → POST `${workspaceBase(wsId)}/threads`; `replyToThread(wsId, threadId, {author,isAgent,body})` → POST `.../threads/:id/replies`; `setThreadResolved(wsId, threadId, isResolved)` → PATCH `.../threads/:id`; `listThreads(wsId)` → GET `.../threads`.
- `useCurrentIdentity(wsId): { login, displayName, avatarUrl } | null`.
- slice: `upsertReviewThread`, `setReviewThreadResolved(id, isResolved)`.

- [ ] **Step 1:** Update `lib/types.ts` `ThreadDTO`/`ThreadReplyDTO` to include `startLine`,`endLine`,`isAgent` (backend now sends them). Realign the three `review-api` thread methods to the `/threads` paths + payloads above (note `replies` plural, drop `/review` prefix, two-way resolve). Add `listThreads`. Add `identity-api.ts` (`getIdentity(wsId)`).
- [ ] **Step 2:** Extend the slice: `ReviewThread` gains `startLine`/`endLine` and `side:'old'|'new'`; add `upsertReviewThread(thread)` (merge-by-id) + `setReviewThreadResolved(id, isResolved)` (two-way). Add `use-current-identity.ts` (fetch once per ws, cache).
- [ ] **Step 3:** Update/extend the review-api + slice tests for the new paths/shapes (assert `/threads` URLs, two-way resolve, upsert merge). Run green. `tsc` no new errors.
- [ ] **Step 4:** Commit: `feat(git-fe): realign /threads API + slice (range, two-way resolve, upsert) + identity client`.

---

### Task 3: FE — `/threads` WebSocket subscription (live threads)

**Files:**
- Create: `web/src/features/workspace/stores/hooks/use-workspace-threads-stream.ts` (or add an effect block to `use-workspace-effects.ts`)
- Modify: `web/src/features/git/components/branch-review-pane.tsx` (seed threads from `listThreads`; stop sourcing threads from `getReview`)
- Test: `web/src/__tests__/.../use-workspace-threads-stream.test.ts` (smoke: a pushed ThreadDTO frame → `upsertReviewThread`)

**Interfaces produced:** a subscription mirroring the `git/status` block in `use-workspace-effects.ts`: `wsManager.subscribe(`${workspaceBase(wsId)}/threads`, frame => upsertReviewThread(mapThread(frame)))`, seeded once via `listThreads(wsId)`; cleanup on unmount.

- [ ] **Step 1:** Implement the subscription hook/effect (model on the existing `git/status` subscribe block — dedupe, cleanup). On each ThreadDTO frame, map → slice `upsertReviewThread`. Seed via `listThreads(wsId)` before/at subscribe.
- [ ] **Step 2:** In `branch-review-pane.tsx`, seed threads from `listThreads` and remove the wholesale `getReview.threads` replace (so the WS stream + optimistic writes aren't clobbered). `getReview` still hydrates description/mergeStrategy/diff.
- [ ] **Step 3:** Smoke test the mapping/upsert. Run green. Commit: `feat(git-fe): live /threads WebSocket subscription`.

---

### Task 4: FE — inline comment widgets on react-diff-view (gutter "+" + range + thread)

**Files:**
- Modify: `web/src/features/git/components/diff/review-diff-view.tsx` (per-file `<Diff>` gains `renderGutter` "+" + `widgets` + change-selection state)
- Create: `web/src/features/git/lib/diff-change-key.ts` (`changeKeyToAnchor` / `anchorToChangeKey` mapping between react-diff-view `getChangeKey` and `{filePath, side, line}`)
- Modify: `web/src/features/git/components/review-thread-item.tsx` (render markdown bodies, AGENT badge, two-way resolve/reopen, identity; reply via the markdown composer)
- Test: `web/src/__tests__/features/git/components/diff/review-diff-view.test.tsx` (extend), `review-thread-item.test.tsx`

**Interfaces consumed:** react-diff-view `renderGutter({change, side, ...})`, `widgets` (`Record<changeKey, ReactNode>`), `getChangeKey(change)`; slice `branchReview.threads`; `openThread`/`replyToThread`/`setThreadResolved`; `useCurrentIdentity`.

- [ ] **Step 1:** `diff-change-key.ts`: derive a stable anchor `{filePath, side:'old'|'new', line}` from a react-diff-view change (use `getChangeKey` + side), and the reverse. Unit-test the round-trip for insert/delete/normal changes.
- [ ] **Step 2:** In `ReviewDiffView`, add `renderGutter` that shows a hover "+" on a line; clicking opens an inline composer anchored to that change (single line; range via shift-select across gutters captured in local state → `{startLine,endLine}`). Build the `widgets` map: for each thread matching this file, place a `<ReviewThreadItem>` widget at its anchor change key. New comments call `openThread(wsId, {...anchor, author, isAgent:false, body})` from `useCurrentIdentity`; optimistic via the slice helper, reconciled by the WS stream.
- [ ] **Step 3:** Upgrade `ReviewThreadItem`: render message bodies through `MarkdownPreview` (Task 5 adds shiki); show identity (avatar/name/@login) — agent messages get the **AGENT** badge; reply box = the markdown composer; **two-way** resolve/reopen via `setThreadResolved(wsId, id, !resolved)`. Outdated: if a thread's anchor change key is not present in the current diff, render it collapsed with an "Outdated" badge (kept, not dropped).
- [ ] **Step 4:** Tests: a thread in the store renders as an inline widget at its line; clicking the gutter "+" opens a composer; submitting calls `openThread` with the right anchor + author. Run green. tsc clean.
- [ ] **Step 5:** Commit: `feat(git-fe): inline comment threads on react-diff-view (gutter + range + widgets)`.

---

### Task 5: FE — Markdown Write/Preview + shiki; retire legacy prototypes

**Files:**
- Modify: `web/src/features/panes/lib/markdown.tsx` (add shiki for fenced code)
- Modify: `web/src/features/git/components/review-thread-item.tsx` / the composer to use Write/Preview (`comment-composer.tsx` already does Write/Preview — reuse it)
- Delete/retire: the `diff-pane.tsx` Alt-click ephemeral comment prototype (and update `pane-container.tsx` if it mounts it for the review surface); the manual-form `review-thread-panel.tsx` creation path (keep a read-only "all threads" list if useful, else remove)
- Test: a markdown render test (code fence highlighted), and confirm no dangling imports after deletions

- [ ] **Step 1:** Add shiki highlighting to `MarkdownPreview`'s fenced code (lazy-init a highlighter; guard for Tauri/SSR). Keep it the single renderer used by composer Preview + thread cards. Test a fenced code block renders highlighted markup.
- [ ] **Step 2:** Ensure the composer (`comment-composer.tsx`) Write/Preview is wired as the new-comment + reply editor in `ReviewThreadItem`/the inline widget (Task 4 used it; confirm Preview uses `MarkdownPreview`).
- [ ] **Step 3:** Retire `diff-pane.tsx`'s local Alt-click thread prototype (remove the ephemeral useState comment system; if `pane-container` mounts `DiffPane` only for the generic `diff` buffer, leave that buffer's non-comment behavior intact — only remove the throwaway comment overlay). Retire the manual filePath/line form in `review-thread-panel.tsx`. Fix imports.
- [ ] **Step 4:** Run `cd web && ./node_modules/.bin/vitest run src/__tests__/features/git` green; tsc no new errors. Commit: `feat(git-fe): markdown Write/Preview + shiki; retire legacy comment prototypes`.

---

### Task 6: Live Tauri verification (Plan-4 gate + full end-to-end)

Controller-run via Tauri MCP (daemon = rebuilt Plan-1 binary; vite running).

- [ ] **Step 1:** Open the review Diff tab. Hover a diff line → confirm the gutter "+" appears; click it → an inline markdown composer opens. Type markdown, toggle Preview (confirm it renders), submit → confirm the thread appears inline anchored to that line, attributed to the current **GitHub/git identity** (name/@login/avatar). No console errors.
- [ ] **Step 2:** Reply to the thread; resolve it (collapses) and reopen it (two-way). Confirm the thread persists across a webview reload (it's backed by `/threads`, not ephemeral) and arrives via the WS stream.
- [ ] **Step 3:** Confirm the legacy Alt-click prototype is gone and the manual-form path is retired. Screenshot/DOM-verify; record in the ledger. Fix any runtime issues before declaring done.

## Self-Review
- Spec coverage: identity endpoint (T1) ✓; `/threads` realign + range + two-way resolve + upsert (T2) ✓; live WS (T3) ✓; inline gutter "+" + range + anchored widgets on react-diff-view (T4) ✓; human+agent + AGENT badge + identity (T4/T5) ✓; markdown Write/Preview + shiki (T5) ✓; outdated handling (T4) ✓; retire prototypes (T5) ✓; live verify (T6) ✓.
- Placeholder scan: outdated-handling + range-select are specified behaviors, not deferrals. Agent posting producer is the agentic phase (out of scope) — here we honor `isAgent` rendering + the wire.
- Type consistency: `openThread/replyToThread/setThreadResolved/listThreads`, `upsertReviewThread`, `setReviewThreadResolved`, `useCurrentIdentity`, the `{filePath,side,line}` anchor are consistent across tasks.
- Dependency: backend `/threads` author/isAgent/range (Plan 1) + identity endpoint (T1) — both available.
