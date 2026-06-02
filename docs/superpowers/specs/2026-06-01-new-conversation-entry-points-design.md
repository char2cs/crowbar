# New Conversation Entry Points — Design Spec

**Date:** 2026-06-01

---

## Problem

There is no way to start a new conversation. The About tab's Conversations section only lists and opens existing ones. The tab bar `+` menu only has Terminal and Open URL.

---

## Decision

Add "New Conversation" in two places:

1. **Tab bar `+` dropdown** — as the first item in the menu, above a separator from Terminal/URL
2. **About tab Conversations section** — a small `+` button next to the section title

Both immediately open a new `crowbarChat` pane with a fresh `nanoid()` as the conversation ID. No API call at creation time (UI-first; backend association happens later).

---

## Changes

### 1. `tab-new-button.tsx`

Add `onNewConversation: () => void` to `TabNewButtonProps`.

Add a `DropdownMenuItem` at the top of the menu content, followed by a `DropdownMenuSeparator`:

```tsx
<DropdownMenuItem onClick={onNewConversation}>
  <Chat className="text-muted-foreground" />
  New Conversation
</DropdownMenuItem>
<DropdownMenuSeparator />
```

Use `Chat` from `@phosphor-icons/react`.

### 2. `tab-bar.tsx`

Wire `onNewConversation` on the `TabNewButton` render site (around line 432):

```tsx
onNewConversation={() => {
  setActivePane(paneId)
  openContent({ type: 'crowbarChat', wsId: nanoid(), name: 'New conversation' })
}}
```

Import `nanoid` from `'nanoid'` (already in the project).

### 3. `about-tab.tsx`

Add `onNewConversation: () => void` to `AboutTabProps`.

In the Conversations section header, replace the bare `FrameTitle` with a flex row:

```tsx
<div className="flex items-center justify-between">
  <FrameTitle className="text-base">Conversations</FrameTitle>
  <Button
    variant="ghost"
    size="icon-xs"
    onClick={onNewConversation}
    tooltip="New conversation"
    aria-label="New conversation"
  >
    <Plus weight="bold" />
  </Button>
</div>
```

Import `Plus` from `@phosphor-icons/react` and `Button` from `@/components/ui/button`.

### 4. `branch-review-pane.tsx`

Add `handleNewConversation` alongside the existing `handleOpenConversation`:

```tsx
function handleNewConversation() {
  store.getState().bufferActions.openContent({
    type: 'crowbarChat',
    wsId: nanoid(),
    name: 'New conversation',
  })
}
```

Import `nanoid` from `'nanoid'`. Pass `onNewConversation={handleNewConversation}` to `<AboutTab>`.

---

## No new stores, no API calls, no new state

The `crowbarChat` deduplication key is the conversation ID (`wsId`). A fresh `nanoid()` guarantees a unique pane every time. The workspace association is implicit — the tab opens in the current workspace's pane group — and will be persisted server-side in a future iteration.

---

## Success criteria

1. Tab bar `+` → "New Conversation" opens a new chat pane.
2. About tab Conversations `+` button opens a new chat pane.
3. Both actions work independently of whether the Conversations list is empty or populated.
4. No hardcoded colors; all UI from `@/components/ui/*`.
