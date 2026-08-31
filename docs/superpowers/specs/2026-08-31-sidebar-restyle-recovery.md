# Sidebar restyle recovery — verify, then finish, not redo

**Date:** 2026-08-31

**Status:** recovery spec, open for review.

**Design authority — unchanged.** This document revises **no** design decision.
The design is closed and remains correct:

- `docs/superpowers/specs/2026-08-28-sidebar-and-pane-surface-design.md` — the
  surface spec, ten laws, exact numbers, the rejected-approaches table.
- `docs/superpowers/specs/2026-08-23-unified-sidebar-design.md` — the model
  spec, four row kinds, two facts, the placement tree.
- The design canvas, `Main.dc.html` / **"The sidebar, live"** artboard —
  <https://claude.ai/code/artifact/5a1008de-282b-494a-bd8d-1b9c123efdee> — is
  the pixel-level ground truth every screenshot in this doc gets compared
  against.

**Triggered by:** a live build shown to the user looked nothing like the
design — wrong proportions, a light theme with a photographic background, a
trust-prompt banner covering the composer, "tab management didn't change a
thing" — after ~137 commits on this branch and, in the user's words, "$1k"
and "+136k -56k lines" spent "preserving near the same behaviors."

**Method.** Four independent, read-only code audits (row/tree/spaces/Recents;
pane/tab-bar/file-explorer-card; theming/window-chrome/trust-modal; and a full
238-turn read-through of the design session transcript to catch anything the
written spec dropped) plus direct verification of the highest-stakes claims.
Nothing in this document was driven live — per instruction, the Tauri MCP
was not touched. **Phase 0 below exists because of that**, not despite it.

---

## 0. The finding that reframes everything else

**The cost/line-count figure is for the wrong diff.** `git diff --stat
develop...HEAD` is 1250 files, +135,538/−57,405 — but `develop` is months
behind and that range includes the agents-engine v3 rearchitecture, the
permission-levels redesign, the mixed-transport API work, and more, none of
which is the sidebar restyle. Isolating to work done **since the design
closed** (`git diff --stat 588ae600..HEAD -- web/`): **333 files,
+21,483/−29,452.** Still a large diff — this was always going to be a
substantial rewiring — but roughly a sixth of the number being quoted, and it
nets *negative* (more deleted than added), which is the shape a genuine
simplification produces, not a shape that suggests random flailing.

**The component code is, to a first approximation, already spec-conformant.**
Across four audits reading `ROW_BASE`/`ROW_SUB_ACTION`, `sidebar-row.tsx`,
`space-header.tsx`, `space-scroller.tsx`, `recents-band.tsx`, the tab bar, the
pane split tree, `pane-border.ts`, and the drop-zone code, the overwhelming
majority of what was checked **matches the spec's numbers and rules exactly**
— including several places where the code comments *cite the spec section
number directly* (e.g. `pane-border.ts` cites the 8ms→106ms WKWebView
regression from spec §7.4 verbatim). "Tab management didn't change a thing"
does not hold up against `HEAD`: the chat is not a closeable tab, there is no
"Editor" tab (a code comment states it was deliberately removed), the pane
layout is a real recursive binary tree, and the `SPLIT_*` constants from
§11's numbers table are wired unmodified. Detail in §2.

**The leading hypothesis for the screenshot's brokenness is a stale or
unbuilt frontend, not broken components.** Row geometry, sidebar width, and
glyph-selection logic are all correct in source; a build that never picked up
the restyle's Tailwind classes (or a packaged Tauri binary never rebuilt
after these commits — a known hazard, see the daemon-hot-restart precedent)
would produce *exactly* the symptom in the screenshot: default browser
spacing and type size with icons still rendering fine, because icons are SVG
and unaffected by a missing utility-class bundle. This is **not confirmed** —
see Phase 0, which exists specifically to confirm or kill it before any
further work is authorized.

**None of this means nothing is broken.** Four real, confirmed defects
survive the audits (§1), one dimension is unverified pending a live check
(§2.3), and the closed design left one real gap unanswered (§3). This
document's job is to fix exactly those, verify against a fresh build first,
and then run the systematic 1:1 parity sweep the user asked for (§4) — not to
authorize another six-figure-line rewrite of code that mostly already matches
the spec.

---

## 1. Confirmed real defects

Each entry: what's wrong, where, why it happened, what closes it. All four
were independently confirmed by direct code reading (file:line cited), not
inferred from the screenshots.

### 1.1 — The "New Project" row is a live violation of law 1 / §2 / §10

**Where:** `web/src/components/layout/sidebar-tree-chrome.tsx:56-77`, mounted
at `web/src/components/layout/sidebar-tree-surface.tsx:121` as a **sibling**
of `<SpaceScroller>`, not inside any space's scroll region.

**Spec:** §2 — *"There is no 'New Project' row at the foot: a project is a
space, not an item in someone else's list."* §13's handoff table repeats it:
the rail *"loses the tab bar, the context pill and the New Project row."*
§10's rejected table lists a sidebar switcher and a second sidebar for the
same reason: it's chrome that costs a row at every height.

**Why it's still there:** the component's own comment admits it verbatim —
*"carried over verbatim from workspace-tree.tsx / sidebar-tree-panel.tsx...
it is an action, not a row with a place in any one project's hierarchy, so it
renders once below every space rather than once per panel."* This was a
deliberate carry-over, not an oversight — but it's the one piece of chrome
that survived the restyle unexamined.

**The real complication, and why this isn't a one-line delete:**
`ImportProjectModal` — the only way to add a **second** project after the
zero-project OOBE screen — is wired to nothing else. Grep confirms its only
two live mounts are `sidebar-tree-chrome.tsx` and `oobe-screen.tsx`. Deleting
the row without relocating its trigger removes the ability to add a project
at all once past onboarding. **This is a real gap the closed design spec
never answered** — see §3.

### 1.2 — `hasView` is dead in the tree: the tree's central law never fires

**Where:** `web/src/components/sidebar/lib/rows-from-repo.ts:40,90,107,119`
— every row-construction site hardcodes `hasView: false`. No code path in
`rows-from-repo.ts`, `rows-for-project.ts`, `space-scroller.tsx`, or
`sidebar-tree.tsx` ever sets it `true`.

**Spec:** §3.2, law-adjacent — *"A row with a view is grey — all of them,
the focused one included."* `sidebar-row.tsx:129` correctly greys the label
on `row.hasView`, so the rendering side is right; the **input** is wrong.
Recents gets the equivalent right (`recents-band.tsx:217`,
`hasView={isLive}` sourced from real pane state), so the defect is isolated
to the tree side.

**Impact:** open a chat into a pane, look at the tree — the row you're
looking at never dims. This is very likely a contributor to the "which one
am I looking at" confusion the user is describing, independent of any build
issue.

**Fix direction:** `rows-from-repo.ts` needs the same live-pane-membership
signal `recents-band.tsx` already reads (whatever store slice backs
`isLive` there — trace it from `recents-band.tsx:217` upward) threaded into
each row's `hasView` instead of the literal `false`. This is wiring, not new
mechanism.

### 1.3 — Chats have no trash — a disclosed stopgap, not a hidden bug, but still open

**Where:** `web/src/components/sidebar/sidebar-row.tsx:71-81`, `deletable =
row.kind !== 'chat'`.

**Spec:** §9 — *"Every row that owns something carries a trash: chats,
workspaces, folders, repos..."*

**Why:** the in-code comment is unusually candid and correct: *"NOTHING
DELETES A CHAT YET — not here, not in Recents, not in the Chats panel:
`deleteChat` (agent-api.ts) has no caller anywhere in the app, and the
removal tray that every other row's trash routes through... has no chat
subject to plan one with. So the control is ABSENT rather than
present-and-broken."* That is the right call for a stopgap — a lying control
is worse than an absent one — but it is a real, current gap against the
closed spec.

**This is a frontend-only wiring gap, not a missing backend feature.** The
backend already ships everything needed:

```
api/internal/api/v0/endpoints/chat/routes.go:112-114
  repoScoped.POST("/chats/:id/promote", h.Promote)
  repoScoped.DELETE("/chats/:id", h.Delete)
  repoScoped.GET("/chats/:id/delete-preview", h.DeletePreview)
```

with `api/internal/app/usecases/chat/promote.go` and
`.../internal/tree/delete_preview.go` behind them, both with their own
regression tests. The frontend wrapper `deleteChat(wsId, id, init)`
(`agent-api.ts:990`) already resolves through `chatBase(wsId)` →
`/v0/projects/:p/repos/:r/chats/:id`, which is the *correct*, current
repo-scoped route — it is not pointed at a stale URL. The gap is purely: wire
a chat row's trash button to `deleteChat` + the existing
`delete-confirm-dialog.tsx` / removal-tray flow, the same way every other row
kind already does it.

### 1.4 — §3.5's "chat can become a worktree" dropdown was never built

**Where:** confirmed absent by direct inspection — `sidebar-row.tsx`'s
leading-glyph span (`RowGlyph`) has no click handler or dropdown; only
`affordance-row.tsx`'s empty-container control implements the
chat-vs-worktree split menu.

**Spec:** §3.5 — *"A chat that could become a worktree (because its parent
is a valid worktree or repository) carries the same dropdown on its own
mark."*

**Backend readiness:** same as §1.3 — `POST /chats/:id/promote` already
exists and is tested (`promote_test.go`, `regression_promote_test.go`). This
is a frontend affordance to build (reuse `affordance-row.tsx`'s existing
dropdown pattern on the row's glyph), not a new backend capability.

---

## 2. Audited and confirmed conformant — do not re-touch

Listed explicitly so a fix pass doesn't waste budget re-litigating things
that already match the spec. Each was verified against the actual file, not
assumed.

**Row geometry & recipes** — `ROW_BASE` (`workspace-row-base.ts:1-4`) is
exactly `h-9 px-1.5 mx-1.5 my-0.5 gap-1.5 text-[13px]`; the 10px-vs-6px right
inset is a correct `pr-2.5` override; `ROW_SUB_ACTION`/`_HOVER` match the
spec's recipe verbatim (`rounded-lg`/10px, `hover:bg-sidebar-element-hover`);
every hand-rolled mark is `size-3`; the 16px/20px glyph exception is applied
only at the two documented sites (project-home row, space header); working
rows swap the glyph for the spinner *in place*; trailing-cluster order is
trash → + → chevron; no `ROW_ACTIVE`/raised treatment anywhere in the tree
(the rejected "focus exemption" from §10 is genuinely absent, if currently
moot per §1.2).

**Spaces** — `space-header.tsx` has no ground at rest, reveals
chevron+overflow only on hover, keeps the chevron rotated while folded;
folding hides only the tree, Recents stays, inside one shared
`<ScrollArea>`; space marks live in the window chrome's dead middle at the
documented opacities; the carousel is `x-mandatory` snap, scrollbar hidden,
armed only by `wheel`/`touchstart`, with the `clientWidth === 0` collapse
guard from §11 reproduced; no repo-to-repo divider lines inside a space;
`.row` never sets `touch-action: none`.

**Recents** — four states derive correctly (`recents-entries.ts`); the × is
absent only on `working`; population/order rules match §5.6 (never re-sorted
by a state change); the shell/member split matches §5.3's recipe. Two
low-confidence style notes only, not bugs: the dormant set shell paints a
faint idle tint where §5.3's literal wording says "nothing is lit" (worth a
30-second visual call, not a rewrite), and there's no per-member "focused"
distinction inside a live set yet — currently moot until §1.2 is fixed.

**Deleting (workspaces/folders/repos)** — the working-chat refusal is
unconditional (no `Dialog` rendered at all while working, matching "REFUSE,
not confirm-and-kill"); the idle confirm names both the file count and the
chat count; protected branches correctly carry no trash.

**Tab bar / pane surface, entire §7** — the chat is not a tab (no close, no
drag handle, rendered outside `<SortableContext>`); no "Editor" tab exists
anywhere in `web/src` (one hit, a comment noting its removal); `+` is the
last child of the scroller and is not sortable; a pane with nothing to
switch between draws no bar at all; the pane layout is a real recursive
binary tree (`pane-node-renderer.tsx` recurses on `node.first`/`node.second`
over a real `LayoutNode`), not a fixed two-pane layout; `SPLIT_MIN_HALF_PX`,
`SPLIT_SIDE_BY_SIDE_MIN_PX`, `SPLIT_MIN_STACKED_PX`, `SPLIT_DEFAULT_SIZES`
are all defined once in `use-chat-presentation.ts` and consumed unmodified in
both `agent-chat-pane.tsx` and `pane-container.tsx`, matching §11's numbers
table; split mode is measured on the pane via `ResizeObserver`, never the
window.

**Pane borders/corners, §7.4** — `pane-border.ts`'s `isWindowEdge` matches
"top is never a window edge" verbatim, correctly shields left/right only
while the sidebar is open on that side, and is actually a **superset** of
the spec (it also handles the sidebar-collapsed case). `buildPaneContentStyle`
matches the `2px solid transparent` → `var(--secondary)` / `var(--radius-lg)`
rule from §11.

**Drag and drop** — `getPaneDropZoneFromRect`-equivalent code uses
`threshold = 0.25` with the diagonal corner tie-break, matching §8.1/§11
exactly; the sidebar's tree and Recents share one drag arm
(`use-sidebar-drag.ts`), not two separate implementations — the transcript's
single most emphatic correction ("drag parity... EXACTLY the same") held.

**Store hygiene** — neither `lib/store/sidebar.ts` nor
`lib/store/build-repo-tree.ts` imports from `components/`, per this repo's
own `CLAUDE.md` rule.

**The old `drop-rules.ts` is dead code, not a live duplicate policy.** It's
still present and still exports its own legality logic, but its only live
importers pull the `DragSubject` *type*, not the policy — `grep` for its
policy export's only consumer is its own test file. **This refutes, rather
than confirms, the theory that prohibited drag behavior comes from a
leftover legacy drop-legality implementation.** Delete it as hygiene (§10's
"no legacy path" rule), but don't spend time chasing it as the cause of any
drag-behavior complaint — if prohibited drag behavior is still reproducible
live, look at `sidebar-drop-policy.ts` itself, not at a ghost of the old one.

### 2.3 — Unverified, needs a live check, not a rewrite

- **Pane gutter (4px left/top, 8px between neighbours, §7.4/§11).** The
  literal values weren't located by name in `pane-container.tsx` or
  `pane-node-renderer.tsx` — they may be expressed as a Tailwind class not
  yet isolated. Flag as unverified, not wrong; confirm with a ruler against a
  live two-pane split before writing a fix.
- **The build/vibrancy theory from §0.** macOS `NSVisualEffectMaterial`
  vibrancy (`desktop/src-tauri/src/lib.rs:581-606`, gated by
  `window-vibrancy`, applied async and main-thread-gated) can silently fail
  to attach, in which case a `transparent: true` window shows the raw
  desktop behind it instead of frosted glass — which reads exactly as "a
  photograph behind the UI." Combined with Tailwind's JIT/arbitrary-value
  classes (`text-[13px]`, `size-3`, `px-1.5`) not being in whatever CSS was
  actually served, this fully explains the screenshot's look without
  requiring any of the row/tree/pane component logic to be wrong — which is
  independently corroborated by §2's audit. **Confirm this in Phase 0**
  before assuming a component rewrite is needed anywhere the audits above
  already called conformant.
- **The trust-prompt banner overlapping the composer.** This is a real,
  pre-existing, deliberately-designed feature (`agent-terminal-wait-banner.tsx`
  — Crowbar genuinely cannot answer a CLI's workspace-trust prompt, and the
  banner exists so the user isn't left staring at silence). Its own doc
  comment already acknowledges the tension it's flagged for: *"a
  workspace-trust prompt is disproportionately a first-turn event"* and the
  banner is `absolute inset-x-4 top-2`, landing directly over a blank chat's
  composer because nothing else occupies that space yet. `git log` on this
  component shows no recent sidebar-restyle-era touches — **this is not a
  restyle regression**, it's a pre-existing rough edge that happens to be
  very visible in exactly the scenario (fresh, untrusted worktree) the
  screenshot captured. Worth a real fix (move the banner or reserve space for
  it so it never sits on top of the input), but scope it as its own small
  task, not folded into "the restyle broke this."

---

## 3. Open design question the closed spec never answered

**Where does "add a project" live once the New Project row is gone?**

§1.1 above can't be closed by deletion alone — `ImportProjectModal` has no
other trigger. The closed spec (§2, §4, §10, §13) is explicit that the row
must go and explicit about *why*, but the six "closed" questions in §12 of
the surface spec don't cover this, and the transcript confirms it was never
raised in 238 turns of design conversation. This is a genuine gap in the
closed design, not an implementation bug — it needs a decision, not a fix.

**Recommendation, not yet a decision:** the space marks already live in the
window chrome's dead middle (§4.1) as an icon-only row where *"the lit one is
the state, and the gesture and the click are the same act."* A trailing `+`
mark after the last project's mark, opening `ImportProjectModal`, is the
smallest addition consistent with that pattern — no new chrome region, same
row, same interaction language. Alternatives considered and not
recommended: reintroducing any form of row at the tree's foot (reopens
exactly what §2/§10 closed the door on), or burying it in the command palette
only (project creation is not a "jump to X" action and shouldn't require
recalling that ⌘K does it).

**This needs a one-line answer before Phase 1 touches §1.1** — either
confirm the recommendation above or supply a different placement. Everything
else in this document can proceed without it.

---

## 4. Phases

### Phase 0 — Verify against a fresh build, before fixing anything

The single highest-value next step, and the reason Tauri wasn't touched
while writing this document.

1. `make dev-desktop` from a clean state (confirm the sidecar/frontend
   bundle actually rebuilds — see the dev-restart precedent on stale
   embedded bundles).
2. Screenshot the sidebar in the running app, at rest, in dark mode
   (the design's default) and light mode.
3. Compare directly against the **"The sidebar, live"** artboard
   (`Main.dc.html`) — same window width, same content shape where possible.
4. Check specifically: is spacing compact (36px rows, tight margins) or
   default-browser-sized? Is the background frosted vibrancy or a raw
   photograph? Do open rows in the tree actually grey out? Is the trust
   banner only visible on a genuinely fresh/untrusted worktree, and does it
   overlap the composer there?
5. **If the build was stale** (compact spacing returns on a clean rebuild):
   most of this document's "confirmed defects" is now the *entire* gap —
   proceed straight to Phase 1, and downgrade the urgency of everything in
   §0's build/vibrancy hypothesis to "already fine."
6. **If the live app still looks broken after a clean rebuild:** the
   hypothesis in §0 is wrong or incomplete. Do not proceed to a rewrite —
   drive the Tauri MCP to inspect the actual computed styles on the
   broken elements and file new, evidence-backed findings in this doc's §1
   before touching component code, exactly as §1.1–§1.4 were produced here.

### Phase 1 — Close the four confirmed defects

Each is independently shippable and testable; do them in any order except
§1.1, which needs §3's decision first.

- **1.1** Relocate the "New Project" trigger per §3's decision, delete the
  chrome-level row, move `SidebarTreeChrome`'s remaining pieces
  (`RemovalTray`, dialogs, context menu) to wherever they belong once the
  New Project button is gone.
- **1.2** Thread real pane-membership into `rows-from-repo.ts`'s `hasView`,
  mirroring whatever `recents-band.tsx:217`'s `isLive` already reads.
- **1.3** Wire a chat row's trash to `deleteChat` + the existing
  `delete-confirm-dialog.tsx`/removal-tray flow. Confirm `wsId` resolution
  still works for a bubble chat with no owning workspace (the model spec
  makes `WorkspaceID` optional) before assuming the existing signature is
  sufficient as-is.
- **1.4** Build the promote-dropdown on a chat row's glyph, reusing
  `affordance-row.tsx`'s existing chat/worktree split-menu pattern, wired to
  the already-live `POST /chats/:id/promote`.

Each task needs a regression test asserting the specific law it restores
(e.g., "a row with `hasView: true` renders with the greyed-label class" —
inverted, this test fails on today's code).

### Phase 2 — Systematic 1:1 parity sweep

Once Phase 0 has produced an accurate current screenshot and Phase 1 has
closed the confirmed gaps, do one pass through the design spec as a
checklist — not a rewrite, a verification pass, ticking off what's already
proven conformant in §2 and spending time only where something doesn't
match. Use the transcript's own "if you see X, that's the drift" list as the
specific things to grep for and confirm absent:

- No invented 6px trailing-action radius anywhere (must be 10px / `rounded-lg`).
- No hand-drawn oversized fold caret (must be `fold-away-button.tsx`'s
  `size-3`/`strokeWidth 1.6`).
- No second sidebar, accordion, or segmented-pill switcher for Files/Git.
- No divider line between the file-explorer card's head and body.
- No context pill anywhere (deleted entirely per §7 of the transcript).
- Tree drag and Recents drag are bit-for-bit identical, not parallel
  implementations (already confirmed in §2, re-verify live).
- Recents reorders by drag only, never by click.
- No "Editor" tab (already confirmed absent in source, re-verify it doesn't
  reappear live).
- The busy-drag refusal inerts every other row (opacity 0.26) and reddens
  the ghost — it does not just redden one drop target on hover.
- Git verb copy never restates the source ("Rebase onto X", never "Rebase Y
  onto X").

Where the artboards (`Files.dc.html`, `Git.dc.html`, `Rows.dc.html`,
`Naming.dc.html`, `Pane.dc.html`, `Panes.dc.html`, `Twoup.dc.html`) show a
specific state not yet exercised live, reproduce that state in the running
app and screenshot it side by side with the artboard.

### Phase 3 — Production readiness

- Resolve the vibrancy/theme finding from Phase 0 for real if it's
  confirmed broken (either fix `apply_vibrancy` failure handling or provide
  a non-transparent fallback background — never a raw unblurred desktop).
- Fix the trust-banner/composer overlap (§2.3) as its own small task —
  reposition or reserve space, don't remove the banner.
- Delete `web/src/components/layout/drop-rules.ts` (§2's dead-code finding)
  and its now-orphaned test.
- Confirm `useSidebarNavStore`'s inert `.push()` screen-stack mechanism
  (§1, law 1 audit note) is either genuinely dead and removable, or has a
  real planned caller — don't leave live-looking dead machinery wired into
  `ide-shell.tsx` unexplained.
- Full gate pass: web test suite, `tsc`, lint, react-doctor, coverage floor
  — per this repo's existing CI bar, not a new one invented for this pass.

### Phase 4 — Sign-off

- Fresh Tauri screenshots of: rail at rest (dark + light), a chat open in
  the tree (grey label verified), Recents with all four states populated,
  the file-explorer card open on both Files and Git, a two-up pane split,
  the busy-drag refusal, and a chat being deleted — each placed next to its
  corresponding design artboard.
- Every item in Phase 2's checklist ticked.
- Every confirmed defect in §1 has a regression test that fails on the
  pre-fix code.
- No open item from this document's §2.3 left unresolved without an
  explicit decision recorded.

---

## Global constraints (carried from this repo's `CLAUDE.md`)

- Component files: kebab-case (`my-component.tsx`), PascalCase export.
- Tests mirror `web/src/` under `web/src/__tests__/`, using `@/` imports —
  never a co-located `tests/` directory.
- Store access: narrow `useXxxStore((state) => state.field)` selectors only;
  `.getState()` confined to event handlers/effects; stores never import from
  `components/`.
