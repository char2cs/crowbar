# Glass Progressive-Blur Band Sizing — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Cut the GPU/compositor cost of the composer's 7-layer progressive-blur "dissolve" effect during streaming, with **zero visible change** to the effect.

**Architecture:** Each `.dissolve-layer` currently spans the full `.dissolve` box (`inset: 0`) and relies on `mask-image` to reveal only its own band — meaning the browser computes a full-box blur (up to 64px radius) even though most of that computed area is masked to zero alpha and thrown away. Resizing each layer's own box to just the band it visibly contributes to — plus a safety margin equal to its own blur radius, since `backdrop-filter` blur samples backdrop pixels from outside its box as input even though output is clipped to the box — cuts the computed area substantially, especially for the expensive 16/32/64px layers. Mask-image stops are re-derived as absolute `calc()` offsets (not `%`) so they land at the exact same pixel position regardless of the box's new (smaller, dynamic) size.

**Tech Stack:** CSS only (`composer.css`). No JS/React changes.

**Spec:** `docs/superpowers/specs/2026-08-30-chat-scale-and-glass-perf-design.md`

## Global Constraints

- **Zero visible change is the acceptance bar**, not "close enough." This plan deliberately excludes trimming layer count or throttling update frequency — both were rejected because they trade a sliver of fidelity for speed. If the pixel diff in Task 2 shows any visible seam, fix the math, don't loosen the bar.
- All values derive from `--dissolve-h: calc(var(--agent-dock-h, 92px) + 88px)` (unchanged from today's box height) — nothing here hardcodes a pixel height, since `--agent-dock-h` is measured at runtime.

---

### Task 1: Resize each `.dissolve-layer` to its visible band

**Files:**
- Modify: `web/src/features/agent/styles/composer.css:86-170`

**Interfaces:** None — pure CSS, no component/prop changes.

**Why:** Every layer's mask reveals only a fraction of the box it's computed over. Layer 7 (the most expensive, 64px blur) is masked to show only its bottom 30% — the browser still blurs the full box today. Shrinking each layer's own `top`/`bottom` to `[a% of H − r, (100−b)% of H − r]` (where `a`/`b` are that layer's visible mask band in % of H, and `r` is its blur radius, used as a safety margin since blur sampling reads beyond the box even though output is clipped to it) keeps the exact same visible pixels while shrinking the computed area.

- [ ] **Step 1: Replace lines 86-170 of `composer.css`**

Keep the existing prose comment above `.agent-chat .dissolve` (lines 61-85) — it's still accurate, the technique hasn't changed, only how much area each layer computes over. Replace the rule block itself:

```css
.agent-chat .dissolve {
  --dissolve-h: calc(var(--agent-dock-h, 92px) + 88px);
  position: absolute;
  left: 0;
  right: var(--agent-scrollbar-w, 0px);
  bottom: 0;
  height: var(--dissolve-h);
  z-index: 1;
  pointer-events: none;
}
.agent-chat .dissolve-layer {
  position: absolute;
  left: 0;
  right: 0;
  /* top/bottom set per layer below — each layer only spans the band its own
     mask actually reveals, plus a margin equal to its own blur radius (blur
     sampling reads backdrop pixels from outside its box even though the
     OUTPUT stays clipped to the box, so shrinking the box doesn't change
     what's visible — it changes how much area the browser recomputes every
     frame). Mask stops are absolute `calc()` offsets from each layer's own
     (now-smaller) top, not percentages of the full dissolve height — that
     keeps them landing at the same pixel regardless of the box's new size,
     which itself varies with the dynamic --agent-dock-h. */
}
.agent-chat .dissolve-layer:nth-child(1) {
  --top: 0px;
  top: var(--top);
  bottom: max(0px, calc(var(--dissolve-h) * 0.6 - 1px));
  backdrop-filter: blur(1px);
  -webkit-backdrop-filter: blur(1px);
  mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0 - var(--top)),
    black calc(var(--dissolve-h) * 0.1 - var(--top)),
    black calc(var(--dissolve-h) * 0.3 - var(--top)),
    transparent calc(var(--dissolve-h) * 0.4 - var(--top))
  );
  -webkit-mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0 - var(--top)),
    black calc(var(--dissolve-h) * 0.1 - var(--top)),
    black calc(var(--dissolve-h) * 0.3 - var(--top)),
    transparent calc(var(--dissolve-h) * 0.4 - var(--top))
  );
}
.agent-chat .dissolve-layer:nth-child(2) {
  --top: max(0px, calc(var(--dissolve-h) * 0.1 - 2px));
  top: var(--top);
  bottom: max(0px, calc(var(--dissolve-h) * 0.5 - 2px));
  backdrop-filter: blur(2px);
  -webkit-backdrop-filter: blur(2px);
  mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0.1 - var(--top)),
    black calc(var(--dissolve-h) * 0.2 - var(--top)),
    black calc(var(--dissolve-h) * 0.4 - var(--top)),
    transparent calc(var(--dissolve-h) * 0.5 - var(--top))
  );
  -webkit-mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0.1 - var(--top)),
    black calc(var(--dissolve-h) * 0.2 - var(--top)),
    black calc(var(--dissolve-h) * 0.4 - var(--top)),
    transparent calc(var(--dissolve-h) * 0.5 - var(--top))
  );
}
.agent-chat .dissolve-layer:nth-child(3) {
  --top: max(0px, calc(var(--dissolve-h) * 0.15 - 4px));
  top: var(--top);
  bottom: max(0px, calc(var(--dissolve-h) * 0.4 - 4px));
  backdrop-filter: blur(4px);
  -webkit-backdrop-filter: blur(4px);
  mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0.15 - var(--top)),
    black calc(var(--dissolve-h) * 0.3 - var(--top)),
    black calc(var(--dissolve-h) * 0.5 - var(--top)),
    transparent calc(var(--dissolve-h) * 0.6 - var(--top))
  );
  -webkit-mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0.15 - var(--top)),
    black calc(var(--dissolve-h) * 0.3 - var(--top)),
    black calc(var(--dissolve-h) * 0.5 - var(--top)),
    transparent calc(var(--dissolve-h) * 0.6 - var(--top))
  );
}
.agent-chat .dissolve-layer:nth-child(4) {
  --top: max(0px, calc(var(--dissolve-h) * 0.2 - 8px));
  top: var(--top);
  bottom: max(0px, calc(var(--dissolve-h) * 0.3 - 8px));
  backdrop-filter: blur(8px);
  -webkit-backdrop-filter: blur(8px);
  mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0.2 - var(--top)),
    black calc(var(--dissolve-h) * 0.4 - var(--top)),
    black calc(var(--dissolve-h) * 0.6 - var(--top)),
    transparent calc(var(--dissolve-h) * 0.7 - var(--top))
  );
  -webkit-mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0.2 - var(--top)),
    black calc(var(--dissolve-h) * 0.4 - var(--top)),
    black calc(var(--dissolve-h) * 0.6 - var(--top)),
    transparent calc(var(--dissolve-h) * 0.7 - var(--top))
  );
}
.agent-chat .dissolve-layer:nth-child(5) {
  --top: max(0px, calc(var(--dissolve-h) * 0.4 - 16px));
  top: var(--top);
  bottom: max(0px, calc(var(--dissolve-h) * 0.1 - 16px));
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0.4 - var(--top)),
    black calc(var(--dissolve-h) * 0.6 - var(--top)),
    black calc(var(--dissolve-h) * 0.8 - var(--top)),
    transparent calc(var(--dissolve-h) * 0.9 - var(--top))
  );
  -webkit-mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0.4 - var(--top)),
    black calc(var(--dissolve-h) * 0.6 - var(--top)),
    black calc(var(--dissolve-h) * 0.8 - var(--top)),
    transparent calc(var(--dissolve-h) * 0.9 - var(--top))
  );
}
.agent-chat .dissolve-layer:nth-child(6) {
  --top: max(0px, calc(var(--dissolve-h) * 0.6 - 32px));
  top: var(--top);
  bottom: 0px;
  backdrop-filter: blur(32px);
  -webkit-backdrop-filter: blur(32px);
  mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0.6 - var(--top)),
    black calc(var(--dissolve-h) * 0.8 - var(--top))
  );
  -webkit-mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0.6 - var(--top)),
    black calc(var(--dissolve-h) * 0.8 - var(--top))
  );
}
.agent-chat .dissolve-layer:nth-child(7) {
  --top: max(0px, calc(var(--dissolve-h) * 0.7 - 64px));
  top: var(--top);
  bottom: 0px;
  backdrop-filter: blur(64px);
  -webkit-backdrop-filter: blur(64px);
  mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0.7 - var(--top)),
    black calc(var(--dissolve-h) * 1 - var(--top))
  );
  -webkit-mask-image: linear-gradient(
    to bottom,
    transparent calc(var(--dissolve-h) * 0.7 - var(--top)),
    black calc(var(--dissolve-h) * 1 - var(--top))
  );
}
```

- [ ] **Step 2: Sanity-check the CSS parses**

```
~/.bun/bin/bun tsc --noEmit
```

(CSS itself has no compile step here, but confirm no adjacent TS/TSX referencing these class names broke — grep for `.dissolve` usages outside this file first: `grep -rn "dissolve" web/src --include=*.tsx`.)

---

### Task 2: Verify zero visible change and measure the win, live

**Files:** None. Verification-only.

- [ ] **Step 1: Launch the dev-desktop app** (reuse an already-running instance for this worktree if one exists — do not start a second one).

- [ ] **Step 2: Screenshot diff at two `--agent-dock-h` values**

Using `mcp__tauri__webview_screenshot`, capture the composer's dissolve region (a) with the composer in its single-line state and (b) after typing enough to wrap it to multiple lines (changes `--agent-dock-h`), before and after this change. Compare pixel-for-pixel (or crop to the dissolve band and diff visually) — any visible difference means the math in Task 1 is wrong for that `--agent-dock-h` value; fix the formulas, don't ship a visible seam.

- [ ] **Step 3: Performance trace during streaming**

Using `mcp__tauri__ipc_monitor` or a performance trace capture available through the Tauri MCP tools, record frame timing while a long reply streams (so `follow-scroll.ts`'s rAF loop is continuously active) — before this change (checkout the prior commit, rebuild, capture) and after. Confirm measured frame time / compositor cost during that window drops. Report the before/after numbers.

- [ ] **Step 4: Confirm no regression in the scrollbar-edge fix**

Scroll the transcript with content long enough to show a scrollbar; confirm the glass still stops short of the scrollbar track (the fix from commit `e8f90ae7`) — this task only changes each layer's `top`/`bottom`, not `right`, so it should be unaffected, but confirm visually since it's cheap to check.
