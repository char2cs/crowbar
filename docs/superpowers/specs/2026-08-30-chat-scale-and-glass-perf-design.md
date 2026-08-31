# Chat Scale & Glass Perf — Design

## Problem

Two production-readiness gaps found investigating a reported FPS drop during streaming:

1. **Scale.** The transcript mounts one full interactive Plate editor (the composer's
   entire ~13-plugin stack) per message, permanently, with no virtualization. Confirmed
   via static read of `markdown-message.tsx`, `agent-transcript.tsx`, `message-row.tsx`:
   this is the dominant cost for a chat carrying thousands of turns — not Slate
   normalization (each message is its own small, independent editor; that axis is fine),
   not a broad Zustand selector (messages live in local hook state, already well-scoped).
   It's raw per-instance editor weight × unbounded DOM mount count.
2. **The composer's "dissolve" glass effect.** `composer.css:86-170` stacks 7
   `backdrop-filter: blur()` layers (1px→64px) to fake a progressive blur (a documented,
   deliberate technique — CSS has no native ramped blur). `follow-scroll.ts` drives a
   real per-frame rAF scroll loop for the full duration of a streamed reply, so all 7
   blurs recomposite every frame of that loop. This is the FPS drop.

## Decisions

- **Plate remains the only rendering engine, everywhere.** No `react-markdown`
  (`[[feedback_plate_is_the_only_markdown]]`, HARD rule, already cost real time once via
  a silent v9 breakage). No hand-authored parallel component set either — see below for
  how `PlateStatic` avoids that trap.
- **`PlateStatic` (from `platejs/static`) replaces the interactive editor for every
  settled (non-streaming) message.** Verified live (not just read) against this app's
  real `chatComposerPlugins`: `createStaticEditor({ plugins: chatComposerPlugins, value })`
  + `<PlateStatic editor={...} />` renders byte-identical output to today's
  `usePlateEditor`+`Plate`+`PlateContent readOnly` path for 8 of 10 node families, using
  the *same* plugin array — not a second one. `PlateStatic` carries no `ReactEditor`, no
  `contentEditable`, no selection/input machinery, so it's substantially cheaper to mount
  per message. Two exceptions need small, targeted fixes (Task 1 of the plan) — neither
  is a content/typography difference, both are plugin-level chrome that assumes an
  interactive editor:
  - `LinkFloatingToolbar` (`link-kit.tsx:19`) calls `useEditorRef()` unconditionally —
    needs a static plugin variant without the toolbar.
  - `CalloutElement`'s emoji-picker button needs Plate context — split into a shared
    content component (box + text, used by both paths) and an interactive-only
    icon-picker wrapper.
  - Everything else (headings, paragraph, marks, lists, code blocks, images, tables, raw
    HTML) needs **no changes** — same local components, same output, confirmed by a live
    diff.
- **Only the actively-streaming message uses the full interactive editor** (needs
  `applyStreamedValue`'s patch-in-place). Everything else — including rows that are
  currently mounted and visible, not just off-screen ones — uses `PlateStatic`. This is
  the actual fix for root-cause #1 (editor-instance weight), independent of windowing.
- **DOM-level windowing on top, via TanStack Virtual** (`@tanstack/react-virtual@^3.14.9`,
  already a dependency, already used for dynamic-height lists —
  `git-history-list.tsx` — and flattened heterogeneous rows —
  `changed-files-tree.tsx` + `flattenChangedFilesTree`). Caps concurrently-mounted DOM
  nodes regardless of total loaded message count, on top of the PlateStatic win.
- **`use-transcript-anchor.ts` / `follow-scroll.ts` need zero changes.** Confirmed by
  reading both fully: the bottom-anchor logic works entirely off `el.scrollTop` /
  `el.scrollHeight` / `el.clientHeight` arithmetic on the scroll container — it has no
  knowledge of individual rows, so it's already virtualizer-agnostic as long as the
  virtualized region reports a correct total height.
- **`mergeMessages` keeps turnId-keying** (`use-chat-messages.ts:27-35`) — load-bearing,
  pinned by an existing test (`mergeMessages: keyed by turnId, not sequence` in
  `use-chat-messages.test.ts`), fixes a live-reported dup-row bug. Not touched.
- **Evidence-recovery burst load** (`loadInitial`, up to `EVIDENCE_RECOVERY_MAX_PAGES(100)
  × MESSAGE_PAGE_SIZE(100)` = 10,000 messages applied synchronously in one tight loop on
  chat-open) gets yielded between pages, so opening a chat with deep history doesn't
  block the main thread in one long task.
- **Out of scope for this plan, on purpose:** evicting/pruning old messages from
  `messages` state itself (a sliding window at the state level, not just the DOM level).
  `mergeMessages`'s rebuild-and-sort cost at realistic scale (a few thousand entries) is
  sub-few-ms per poll tick — not a frame-budget threat once editor/DOM weight is fixed —
  and eviction has real correctness risk (interacts with scroll-anchor `stuck` state and
  `loadOlder`'s cursor bookkeeping) that deserves its own dedicated design. Flagged
  explicitly, not silently dropped.
- **Glass fix: resize compute area, don't change the effect.** Each `.dissolve-layer`
  currently spans the full `.dissolve` box (`inset: 0`) regardless of how little of it
  its own `mask-image` actually reveals. Resizing each layer's box to only the band it
  visibly contributes to (plus a safety margin equal to its own blur radius, to avoid any
  edge-sampling difference) cuts GPU compute area — especially for the expensive 16/32/64px
  layers — with mask-image stops re-derived as absolute `calc()` offsets (not %) so they
  land at the exact same pixel regardless of the dynamic `--agent-dock-h`. Provably
  pixel-identical output.
  - Rejected on purpose: trimming the layer count, throttling blur update frequency
    during motion. Both trade a sliver of fidelity for speed — at odds with "no quality
    loss at all." Worth a follow-up once the safe fix is measured, not part of this plan.

## Non-goals

- State-level message eviction (see above).
- Editable/interactive transcript rows (confirmed read-only today; if that ever changes,
  the PlateStatic swap needs revisiting — it assumes rows never need to become editable
  in place).
- Any change to `use-transcript-anchor.ts` or `follow-scroll.ts`.

## Verification

- Node-type parity test (byte-identical output, interactive vs. static path) — codifies
  the live experiment that grounded this design, becomes a permanent regression gate.
- Scale test asserting mounted row/editor DOM count stays flat as loaded-message count
  grows (mirrors `changed-files-tree.scale.test.tsx`).
- Glass fix: Tauri dev-desktop screenshot diff at a couple of `--agent-dock-h` values,
  plus a performance trace comparing frame cost during a streaming scroll, before/after.
