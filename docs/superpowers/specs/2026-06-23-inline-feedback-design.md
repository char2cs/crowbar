# Inline Feedback — Replace Toasts

**Date:** 2026-06-23  
**Branch:** hardening/production-readiness  
**Scope:** Replace all toast notifications (except Editor/LSP) with inline, contextual feedback tied to the element the user interacted with.

---

## Background

Toasts are used throughout the app as a catch-all feedback mechanism. The problems:

1. **No completion signal** — async ops (push, pull, merge) fire a "Starting…" toast and never tell the user when they're done. Users had to reload to see the new state.
2. **Displaced feedback** — a toast in the corner is disconnected from the element that triggered the action.
3. **Lingers forever** — toasts without auto-dismiss pile up and are ignored.

The backend already resolves state through WebSocket streams (asynx pattern). The frontend just needs to surface that state inline rather than via toasts.

**Editor/LSP errors remain as toasts** — out of scope for this spec.

---

## Category 1 — Git Async Ops

**Affected files:** `branch-section.tsx`, `merge-popover.tsx`, `git-actions-menu.tsx`, `git-tag-manager.tsx`, `use-review-comment-layer.tsx`

### States

| State | UI |
|---|---|
| **Idle** | Button enabled, status line shows current state ("Clean · 1 to push", "Up to date", etc.) |
| **In-flight** | Button disabled + spinner icon. Status line text changes: "Pushing to remote…" / "Pulling…" / "Rebasing…" / "Merging…". Animated dot in status line. |
| **Resolved** | WS stream updates `gitStatus` / `activeWs.status`. Status line reflects new state ("Up to date", conflict, etc.). Button returns to idle or disappears. No toast. No badge. |
| **Error** | Inline error text appears below the button row. Includes a short message and a "Retry" link. Dismisses automatically on the next attempt. |

### Rules

- `remoteBusy` / `rebasing` state already exists — wire the spinner and text to it, remove the `toast.info`.
- `toast.error` on failure → replaced with a local `error` state rendered inline below the buttons.
- Merge: on success the panel navigates away — the navigation is the feedback. Remove `toast.info('Merging…')`. Keep error inline in the popover.
- `git-actions-menu.tsx`: replace the loading toast + success toast pattern with a spinner on the action row item and inline error.
- `git-tag-manager.tsx`, `use-review-comment-layer.tsx`: replace success/error toasts with inline state on the relevant row/form element.

### What the WS stream drives

`gitStatus.ahead`, `gitStatus.behind`, `gitStatus.branch`, `activeWs.status` — these already update via the WS stream when the daemon completes the op. The status line re-renders automatically once they arrive; no manual refresh needed beyond the existing `refresh()` call that nudges the poller.

---

## Category 2 — Workspace & Chat CRUD

**Affected files:** `workspace-tree-context.tsx`, `workspace-tree-item.tsx`, `chat-tree-context.tsx`, `chat-tree.tsx`

### Create workspace

1. User types name in the inline input and presses Enter.
2. Input disappears immediately.
3. A new workspace row appears in the list at the expected position with:
   - **Agent-style spinner** in the icon slot (where `WorkspaceBranchIcon` normally renders).
   - Branch name dimmed (`text-muted-foreground`).
   - Row not yet navigable (pointer-events: none or `isLocked`).
4. WS stream confirms creation → spinner swaps back to `WorkspaceBranchIcon`, row becomes active and navigable. Silent.
5. **Error:** spinner slot becomes a red `✕`, name stays dimmed, a small `"failed"` badge appears. Row stays in list; clicking the badge or a retry affordance clears it and re-attempts creation.

### Move / reparent

- **Optimistic:** item moves to the new parent position in the tree immediately on drop.
- Item rendered dimmed + agent spinner in icon slot while the API call is in-flight.
- WS stream confirms → spinner gone, item fully opaque and navigable. Completely silent.
- **Conflicts** are already handled by the backend `pr-conflicts` WS signal and the existing conflict indicator. No new UI.
- **Move error (API failure):** item snaps back to its original position (revert optimistic). The `workspace-tree-context.tsx` move handler must store the pre-move parent/index before calling the API so it can restore on failure. No badge, no toast — the snap-back is the signal.

### Delete

- Optimistic remove from list. Silent.
- Error: item reappears with an error badge. No toast.

### Chat CRUD

- Same pattern: errors surface on the chat row item, not as toasts.
- "Open a workspace first" guard: replace `toast.error` with a disabled state or an inline hint on the button.

### Removed cases

- **"✓ moved" badge** — success is silent.
- **"conflicts ⚠" badge** — handled by existing WS signal.

---

## Category 3 — Clipboard & File Ops

**Affected files:** `use-file-explorer-context-menu.tsx`

### Copy path / copy relative path

- On success: a small `"✓ path copied"` badge appears on the file row in the tree. Fades after **1.5 s** via a timeout + CSS opacity transition.
- On error (clipboard API denied): a small `"✕ copy failed"` label replaces the badge briefly, same fade timing.

### Create file

- On success: the new file row appears with a `"✓ created"` badge that fades after **1.5 s**.
- On error: inline label on the row or at the point of creation in the tree.

### Other context menu ops (refresh, cut, copy item)

- **Refresh:** no toast needed — the file tree rerenders on completion.
- **Cut/Copy item (for paste):** no feedback needed; the paste target will show the result.
- **"Choose a different env file name"** guard: inline validation text in the rename input, not a toast.

### Implementation note

Use a `Map<path, 'copied' | 'created' | 'error'>` in local state (or a ref) keyed by file path. Each entry auto-clears after 1.5 s via `setTimeout`. The badge renders as an absolutely-positioned or flex-end element on the file row.

---

## What stays as toasts

- Editor save failures (`editor-app-store.ts`)
- Read-only workspace guard (`editor-app-store.ts`)
- LSP restart/stop/start errors (`editor-status-actions.tsx`)
- Settings import/export (`developer-settings.tsx`)
- Add repository / import project modals (already modal-contained, errors can stay as toasts for now)

---

## Non-goals

- No redesign of the toast component itself.
- No change to how toasts are triggered in editor/LSP paths.
- No new animation library — use Tailwind transitions and CSS only.
