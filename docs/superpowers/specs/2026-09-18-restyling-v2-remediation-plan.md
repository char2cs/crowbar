# restyling/v2 remediation — root causes, fixes, and the live verification pass

**Date:** 2026-09-18

**Status:** plan, open. `fix/restyling-v2-followups` (15 commits off `origin/develop` @ `c9db38eb4`, 231 files, all CI-equivalent gates green — tsc, eslint, prettier, targeted vitest, react-doctor, `go test -race` full suite, black-box + route audit) holds most of the fixes below. **Nothing in it has been live-verified through the actual app yet**, and it is not merged or shipped. This document is what gets checked off before it is.

**Why a plan instead of another loop.** The audit-and-patch loop found ~70 symptoms that collapse into four design gaps the code answered inconsistently. Patching each symptom already produced one regression (a fix in round 1 broke legacy-chat owner resolution; the next round caught it). A plan names the actual decisions once so the remaining work — and the live pass — has a target to check against instead of another symptom list.

---

## 1. The three showstoppers, by status

### A. Cannot create a thread or a branch thread — FIXED on the branch, not yet live-verified

**Root cause (two parts, both hit on real production data):**
1. `ResolveForkParent`/the fork-parent walk (`api/internal/app/usecases/chat/internal/tree/walk.go`) only ever walked chats and folders — never a workspace's own anchor. Forking off a **chatless** workspace row (a repo header, an untouched locked branch, any row identified by its workspace id rather than an owning chat) passed placement validation, minted, then 409'd (`ErrNoForkParent`) at the fork step itself.
2. Every workspace/folder/repo created **before the #179 Node migration** has no `Node` row at all. Both the workspace-anchor fix above and the plain "resolve this id as a create parent" path 404'd with `parent <id>: not found` for that entire class of legacy record — which is most of a real, months-old production install.

**Fix:** `ensureWorkspaceAnchor` mints the anchor `Node` on first touch; `walk.go`'s `workspaceAnchorsReachable` + `repoRootFallback` make the fork-parent walk see it; a repo-root folder's own fallback (`RepoRoots.DefaultWorkspaceOf`) covers the folder-entry-point variant. Commits `5303fffa0`, `5c103538b`, `9d77be815`. Regression coverage: `TestRegression_ResolveForkParent_*`, `TestRegression_ForkUnderRepoRootFolder_*`, `TestRegression_CreateUnderLegacyNodelessWorkspace_*` (black-box, `api/tests`).

**Still open:** none known. **Live check required:** thread AND branch creation from every row kind (space header, repo header, locked default branch, ordinary fork, a chat, a folder), on data shaped like production (a workspace/chat that predates today — the dev instance's own data is all fresh, so this needs either an imported real-shaped fixture or the production diagnostic in §3 below to actually confirm).

### B. No reorder indicator between two repo header rows — ROOT-CAUSED, NOT YET FIXED

Confirmed live: both repo rows carry the right DOM marker (`data-sidebar-branch-drop`), the hit test resolves the correct target row, but the drop-line never paints. Repo-vs-chat and repo-vs-folder both work; only repo-vs-repo is dark.

**Mechanism** — `web/src/components/sidebar/lib/sidebar-drop-policy.ts:319-325`:
```js
if (target.kind === 'branch') {
  return target.repoIcon && target.repoIcon.projectId === repoIcon.projectId
    ? REORDER_MODES : NO_MODES
}
```
`target` is reconstructed **per pointermove**, straight off DOM attributes, by `tree-dnd/drop-dom.ts`'s `read()` (`use-sidebar-drag.ts`'s live `hitTest`). `SIDEBAR_DRAG_ROW_SPEC` (`use-sidebar-drag.ts`) never declares `repoIcon` as one of the strings/flags it reconstructs — so `target.repoIcon` is **always `undefined`** during a live hover, and this check always falls to `NO_MODES`. The commit-time resolver (`subjectsFor`, used by `onDrop`) doesn't have this gap, which is exactly why only the *indicator* is missing, not (necessarily) the eventual drop.

**Fix (pick one, both are small):**
- Add the target row's `projectId` as a real DOM string attribute in `SIDEBAR_DRAG_ROW_SPEC.strings` and read it back in this check, mirroring how `path`/`inRecents` already round-trip; or
- Resolve the live per-frame project-scope check via `resolveRowRepo`/the repos store the same way every *other* branch of `allowedModes` already does, instead of trusting a `repoIcon` field the live hit-test can never populate.

Second one is more consistent with the rest of the file (every other target-scope check in `allowedModes` already goes through `resolveRowRepo`) — prefer it unless it turns out `resolveRowRepo` can't reach a bare repo id cheaply per-frame.

**Also noticed, unconfirmed:** both repos' `main` branches show a persistent "Branch needs provisioning" warning triangle even while a session is actively running on a fork of one of them. Not investigated; flag for the live pass (§4) since it's visually prominent.

### C. "Claude receives the message but Crowbar never shows working or the response" (production) — NOT REPRODUCED, NEEDS A PRODUCTION-SIDE DIAGNOSTIC

This is the most serious of the three — it's the core chat loop — and the one I have the least evidence on. What's established:

- **Not caused by anything in `fix/restyling-v2-followups`.** `git diff origin/develop..HEAD` touches zero files on the hook-ingestion/turn path (`internal/turn/*`, `cmd/crowbar/hook*.go`, `internal/engine/agents/**`, `internal/api/v0/endpoints/chat/handlers/hooks.go`) or on the daemon's runner/segment registry that hook delivery is keyed against. The chat-tree files this branch touched (`chat.go`, `chat_owner.go`, `internal/tree/owning_chat.go`) sit on placement/ownership resolution, not on the hook→WS-broadcast path for an already-running turn.
- **Not reproducible on the dev instance.** A real message sent to an existing live session round-tripped correctly — user bubble, then a genuine model response with badge and timestamp, no stuck state — on this exact branch's code.
- **One gap in that test:** it exercised an *existing* chat's hook stream, not a **brand-new** chat's first-ever hook registration. Given how much this session rewrote about chat/workspace identity at creation time (`EnsureOwner`, the chatless-workspace owner-minting path), a first-hook-registration-specific bug is still plausible and untested.
- Given today's whole pattern — repeatedly, a code path works on fresh dev data and breaks only on data shaped like a real, months-old production history (empty `chat.Type`, missing `Node` rows, HomeID-less folders) — the leading hypothesis is a **legacy-data incompatibility somewhere in session/runner identity resolution**, not a logic bug that would show up on any data. That is a guess, not a finding; nothing above confirms it.

**This needs a read-only production diagnostic before it can be scoped as code work**, because it isn't reproducible anywhere else:
1. Check the daemon log for actual hook-delivery attempts arriving for a real, recent message (`grep` the request log around the timestamp of a message the user sent that got no visible response) — did the daemon receive the hook at all, did it 4xx/5xx, or does it never show up (pointing at the CLI-side forwarder, not the daemon)?
2. Check for a wedged/stale runner or a daemon that needs a restart — `fix/hook-spool-connection-leak` (merged to `develop` 2026-09-13, a connection-leak-per-delivery bug in the hook-spool drain loop) predates the nightly the user is running, so it should already be in production, but worth confirming the daemon hasn't been running long enough to have hit some other slow leak.
3. Test whether the symptom is universal or scoped to chats created after a specific point (before vs. after the restyle-v2 nightly install) — that split is the direct signal for "legacy session-identity resolution" vs. "something environmental" (auth, provider CLI path, permissions).
4. This is read-only diagnostic work against the user's **real** `~/.crowbar` / production socket — never a mutating action, never launched from a dev harness. Treat it with the same care as the very first investigation in this whole effort.

**I am not fixing or claiming anything about C in this branch.** It gets its own investigation once §1's diagnostic has an answer to root-cause against.

---

## 2. What's already fixed on the branch (the four design gaps behind ~50 of the ~70 symptoms found)

Everything below is committed, gated green, **not yet live-verified**.

1. **No canonical sibling set per level.** `Node.ParentID == ""` was one global bucket shared by every project's home rows, every repo's own root, and every workspace's anchor point — a repo reorder could count another repo's *internal* branch rows as siblings, root-born home chats had no `Node` row so a repo could never land between them, and leaving a level renumbered only same-kind rows. Fixed with an explicit per-level scope (`forestScope`/`rootMember`/`foreign`, `home_level.go`) and root chats minted at `NextSlot` instead of arriving nodeless. Commits `5c103538b`, `af12f9784`, `9d77be815`.
2. **Legacy/chatless rows had no adoption path.** A workspace, chat, folder or repo that predates the Node migration, or a workspace with no owning chat, was invisible or refused by every writer instead of being adopted on first touch. Fixed via lazy `Node` minting on read/write touch, `domain.Chat.IsChat`/`EffectiveType` in place of a raw `Type ==` check, and `Folder.HomeID`/`ListInHome` project-scoping. Commits `5303fffa0`, `3760391b4`, `9d77be815`, `520c260c4`.
3. **Four different tie-break rules for equal `order`.** Render, the frontend's own reorder planner, the repo-PATCH backend and the chat-PATCH backend each broke ties differently, so the first drag on all-zero data (every fresh project's real starting state) could visibly disagree with itself. Fixed with one shared `Rank`+`createdAt` model on both backends and one shared comparator on the client (`row-order.ts` / `build-repo-tree.ts`). Commits `520c260c4`, `af12f9784`.
4. **Placement collateral wasn't broadcast, and the client re-read a stale projection.** Reordering one row server-side renumbers its siblings as a side effect; only the dragged row was ever announced, so the sidebar's other rows silently drifted from the daemon's real state until an unrelated reseed happened to fix it — and a direct-applied placement response could itself be reverted by the next cache-sourced rebuild. Fixed: `announceRepoPlacement` fans out every shifted row; the client's applied-placement write goes through the same IndexedDB cache path a rebuild reads from (`applied-placement.ts`). Commits `3760391b4`, `520c260c4`, `664f393da`.

Plus a long tail of smaller, independent bugs — the persisted `collapsedRepos`/`collapsedProjects` gate from the pre-restyle build hiding repos/projects with no UI left to un-collapse them, the home-resolver latching one cold-start failure forever, a ghost "Untitled chat" row at project-home top level, the "failed" badge on a create swallowing the daemon's real error, the desktop proxy turning a daemon cold-start into a hard 502 with no retry, `DELETE` on a repo's default-checkout owner chat cascading `git worktree remove` onto the repo's **main** checkout. Full list with file:line is in the discovery artifacts (`api/tests/regression_*_test.go` and the `web/src/__tests__/**` files this branch added — every one of them is a live regression test for one of these, named for the symptom).

**One already-caught regression, now fixed:** round 1's fix to owner-chat resolution (`ownerCandidates`) dropped every *titled* legacy chat from consideration, demoting a real pre-existing conversation. Caught by the next audit round, fixed in `9d77be815`.

---

## 3. Verification plan (the live pass)

Everything above is "gates green," which is necessary and has already been wrong once this session (twice, counting the original workspace-list fix that needed two follow-up PRs to actually render correctly). Nothing gets called done until it's been driven through the real Tauri app. One driver at a time, on the isolated dev instance (never the production socket, never a second dev instance).

1. **Showstopper repros, first:** A and B above, plus C's diagnostic (§1.C) run separately against production, read-only.
2. **Row-kind × verb sweep:** for every row kind (space header, repo header, locked default branch, ordinary fork, chatless branch, chat, thread-of-chat, folder, recents entry) — create thread, create branch, rename, trash/undo, open/navigate, collapse/expand, reorder (before/after/into, every kind against every other kind at the levels that should allow it), context menu items.

   **The context-menu verbs are a BY-HAND leg of this sweep — a driver must never open that menu.** The sidebar's right-click menu is the OS's own `NSMenu` (`web/src/components/ui/context-menu.tsx:216-247` → `crowbar-bridge.ts`'s `showNativeContextMenu`, reached from `row-context-menu.tsx`'s capture-phase `contextmenu` listener), and an open `NSMenu` runs a nested modal run loop on the same main thread that serves the tauri-plugin-mcp-bridge IPC. While it is up, every bridge call — `webview_execute_js`, `webview_screenshot`, `webview_keyboard`, `manage_window` — times out, and `driver_session` stop/start does not recover it; the app is not crashed, it is parked in the menu's run loop. That is the platform behaving correctly (a native menu is modal and is not in the element tree), not an app defect, so there is nothing to fix in the app and no automation flag worth adding: a driver-only fallback menu would verify the rendered Base UI path the user never sees. **Rename, Lock/Unlock, Import branches and "New folder" live only in this menu** (plus the repo row's `data-control="repo-menu"` button, which opens the same one), so those four verbs are exercised by hand. Their underlying actions are reachable from a driver by other routes — the row controls, and double-click-to-rename. **Recovery if a synthetic `contextmenu` is dispatched anyway:** steal focus from another app — `osascript -e 'tell application "Finder" to activate'` dismisses the menu and the bridge answers immediately. Synthetic Escape (osascript keystrokes, `CGEventPost`) does nothing without Accessibility permission for the driving process.
3. **Legacy-data shape:** since almost every bug this session found only showed up on data older than today, seed the dev instance with a fixture that actually looks like production — a workspace/chat/folder/repo with no `Node` row, a chat with empty `Type`, a home folder with no `HomeID` — and re-run the sweep above against it, not just against fresh data.
4. **Cold start:** collapse/expand state, IndexedDB cleared, full process restart — confirm every repo header still renders, no duplicate or ghost rows, no pending placeholders stuck.
5. **The "needs provisioning" triangle** noted under B — confirm whether it's a real state bug or stale UI.

Only after this passes does the branch merge to `develop` (as its own PR, on request) and get considered for a nightly.
