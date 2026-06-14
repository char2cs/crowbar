# Imperative Editor Consumers — React off the editor hot path entirely

**Date:** 2026-06-13
**Status:** Design — approved direction (user chose the full rewrite)
**Scope:** The editor area's reactive consumers only (cursor/status sync, LSP overlays, diagnostics/code-lens, content subscriptions). Sidebar untouched. Builds on the decouple (`editor-surface` + manager + satellites).

## 1. Problem (measured, evidence-first)

Real typing was profiled live (user typed ~30 chars): **217 React commits (~7/keystroke)**, vs ~0.3/keystroke in synthetic `editor.trigger` bursts. The gap = **async work that fires *during* real interaction**: LSP `documentChange`→diagnostics→markers, code-lens refresh, autosave, and the per-cursor-move status sync — **all React subscribers re-rendering the editor subtree while you type/scroll/open/switch.** Idle is clean (0 commits); the editor is NOT recreated during typing (ruled out). So the residual sluggishness across **all four** symptoms (open, switch, typing, scroll) shares one root: **React reacting to editor events on the hot path.**

The retained-editor decouple removed the Monaco-recreate and stabilized the shell, but the *consumers* of editor state are still React components that re-render on every editor event. This is the React/webview ceiling — the only way past it (short of a native renderer) is to take those consumers OUT of React.

## 2. Principle

Editor-event reactions become **imperative DOM controllers** attached to the retained Monaco editor, like VSCode "contributions": they listen to Monaco/LSP events and mutate their own DOM directly — no React render, no store round-trip on the hot path. React keeps owning *static* chrome and *infrequent* state; it is removed from the per-keystroke / per-cursor / per-scroll / per-diagnostic path.

## 3. Units (highest-impact, most-isolated first)

### U1 — Cursor/selection status (per-keystroke; do first)
Today every cursor move calls `syncCursorAndSelection` → `setCursorPosition` + `setSelection` on the editor-state store → status-bar (line:col) + other subscribers re-render. That's per-keystroke React churn.
- Replace with an imperative source: the status-bar cursor chip reads cursor position via a lightweight subscription throttled to rAF (one update/frame max), or an imperative text-node update driven by `onDidChangeCursorPosition` writing directly to the chip's DOM. No per-keystroke store setState.
- Keep the store value for features that genuinely need it (go-to-line, extensions), but write it coalesced (rAF/idle), not synchronously per move.

### U2 — Code-lens overlay (re-renders on content change)
Convert the code-lens overlay from a React component re-rendering on content/lens changes to an imperative layer that positions lens widgets via Monaco's view-zone/content-widget API (or a single imperatively-updated DOM layer), updated on a debounced lens fetch — no React reconcile per content change.

### U3 — Completion / hover / signature-help overlays
These re-render as you type. Convert to imperative widgets (Monaco content/overflow widgets) driven by the LSP client events, mutating their DOM directly. Each migrated one-at-a-time, behavior-verified (this is the riskiest set).

### U4 — Diagnostics
Verify markers stay Monaco-native (`setModelMarkers`, already non-React) and that no diagnostics update triggers a React store write that re-renders the editor subtree. If one does, route it imperatively.

### U5 — Content subscription (PaneLspLayer)
`PaneLspLayer` subscribes to active content (`value`) → re-renders on every store content update. Move LSP `value` reads to imperative model reads (`model.getValue()` on demand / on debounced change events), so PaneLspLayer doesn't re-render on content change.

### U6 — Autosave/dirty
Ensure autosave + dirty-indicator updates don't re-render the editor subtree per keystroke (dirty dot is a tiny leaf; autosave is off-hot-path already — verify).

## 4. Acceptance
- Real typing (measured live with the user / trusted input): **≤~1 React commit per keystroke** (down from ~7), ideally ~0 on the steady path.
- Scroll: no React commits per scroll frame.
- Open/switch: content paints fast (already ~15ms) AND no heavy React reconcile frame after.
- Full behavior parity: cursor display, completion, hover, signature, code-lens, rename, diagnostics, go-to-line, autosave, dirty — all identical.
- Verified LIVE with the window foregrounded (rAF active); measurement instrumented via React commit counter + the manager (not querySelector), per the methodology guardrails.

## 5. Risks
- The LSP overlays (U3) are intricate, positioned, stateful React components — converting to imperative widgets is the high-risk core. One-at-a-time, each behind verification.
- Measurement in WKWebView has no real profiler; rely on the React-commit counter + real (user) keystrokes for the typing number, synthetic only for isolation.
- Don't regress the decouple, model-swap, view-state, or resize work.

## 6. Phasing
U1 (cursor-status, biggest per-keystroke, lowest risk) → U5 (content subscription) → U2 (code-lens) → U4 (diagnostics audit) → U3 (hover/completion/signature, one each) → U6 (autosave audit) → final live re-measure of real typing.
