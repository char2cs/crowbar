# New Conversation Entry Points — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add "New Conversation" actions to the tab bar `+` dropdown and the About tab's Conversations section header.

**Architecture:** Two independent wiring tasks. Task 1 adds `onNewConversation` to `TabNewButton` and wires it in `tab-bar.tsx` via `openContent({ type: 'crowbarChat', wsId: nanoid() })`. Task 2 adds the same action to `AboutTab` via a new `onNewConversation` prop wired in `branch-review-pane.tsx`. No new stores, no API calls.

**Tech Stack:** React, `nanoid`, `@phosphor-icons/react`, `@/components/ui/button`, `@/components/ui/dropdown-menu`, Vitest + `@testing-library/react`.

---

## File Map

| Action | Path | Responsibility |
|---|---|---|
| **Modify** | `web/src/features/tabs/components/tab-new-button.tsx` | Add `onNewConversation` prop + menu item + separator |
| **Modify** | `web/src/features/tabs/components/tab-bar.tsx` | Wire `onNewConversation` callback using `nanoid()` |
| **Modify** | `web/src/features/branch-review/components/about-tab.tsx` | Add `onNewConversation` prop + `+` button next to Conversations title |
| **Modify** | `web/src/features/branch-review/components/branch-review-pane.tsx` | Add `handleNewConversation` and pass to `AboutTab` |
| **Create** | `web/src/__tests__/features/tabs/components/tab-new-button.test.tsx` | Smoke + menu item test |
| **Create** | `web/src/__tests__/features/branch-review/components/about-tab-new-conversation.test.tsx` | Button render + click test |

---

### Task 1: Tab bar `+` button — New Conversation item

**Files:**
- Modify: `web/src/features/tabs/components/tab-new-button.tsx`
- Modify: `web/src/features/tabs/components/tab-bar.tsx`
- Create: `web/src/__tests__/features/tabs/components/tab-new-button.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/tabs/components/tab-new-button.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import TabNewButton from '@/features/tabs/components/tab-new-button'

const baseProps = {
  isBottomPane: false,
  disablePaneActions: false,
  isInSplit: false,
  onNewConversation: vi.fn(),
  onNewTerminal: vi.fn(),
  onOpenUrl: vi.fn(),
  onClosePane: vi.fn(),
}

describe('TabNewButton', () => {
  it('renders the + trigger button', () => {
    render(<TabNewButton {...baseProps} />)
    expect(screen.getByRole('button', { name: 'New tab' })).toBeDefined()
  })

  it('calls onNewConversation when "New Conversation" is clicked', async () => {
    const onNewConversation = vi.fn()
    render(<TabNewButton {...baseProps} onNewConversation={onNewConversation} />)
    await userEvent.click(screen.getByRole('button', { name: 'New tab' }))
    await userEvent.click(screen.getByText('New Conversation'))
    expect(onNewConversation).toHaveBeenCalledOnce()
  })
})
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
cd web && npx vitest run src/__tests__/features/tabs/components/tab-new-button.test.tsx
```
Expected: FAIL — `onNewConversation` prop does not exist yet.

- [ ] **Step 3: Update `tab-new-button.tsx`**

Replace the full file content of `web/src/features/tabs/components/tab-new-button.tsx`:

```tsx
import {
  Chat,
  GlobeHemisphereWest as Globe,
  Plus,
  SidebarSimple as PanelLeftClose,
  TerminalWindow as Terminal,
} from "@phosphor-icons/react";
import React from "react";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";

interface TabNewButtonProps {
  isBottomPane: boolean;
  disablePaneActions: boolean;
  isInSplit: boolean;
  onNewConversation: () => void;
  onNewTerminal: () => void;
  onOpenUrl: () => void;
  onClosePane: () => void;
}

const TabNewButton = React.memo(function TabNewButton({
  isBottomPane,
  disablePaneActions,
  isInSplit,
  onNewConversation,
  onNewTerminal,
  onOpenUrl,
  onClosePane,
}: TabNewButtonProps) {
  if (isBottomPane) return null;

  return (
    <div className="flex shrink-0 items-center gap-1 pl-0.5">
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              variant="ghost"
              size="icon-xs"
              className="shrink-0 text-muted-foreground"
              aria-label="New tab"
            />
          }
        >
          <Plus weight="bold" size={12} />
        </DropdownMenuTrigger>
        <DropdownMenuContent side="bottom" align="start" className="min-w-[160px]">
          <DropdownMenuItem onClick={onNewConversation}>
            <Chat className="text-muted-foreground" />
            New Conversation
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={onNewTerminal}>
            <Terminal className="text-muted-foreground" />
            New Terminal
          </DropdownMenuItem>
          <DropdownMenuItem onClick={onOpenUrl}>
            <Globe className="text-muted-foreground" />
            Open URL
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      {!disablePaneActions && isInSplit && (
        <Button
          onClick={onClosePane}
          variant="ghost"
          size="icon-xs"
          className="shrink-0 text-muted-foreground"
          tooltip="Close Split"
          tooltipSide="bottom"
          aria-label="Close split pane"
        >
          <PanelLeftClose />
        </Button>
      )}
    </div>
  );
});

export default TabNewButton;
```

- [ ] **Step 4: Wire `onNewConversation` in `tab-bar.tsx`**

In `web/src/features/tabs/components/tab-bar.tsx`, add `nanoid` to the imports at the top of the file:

```ts
import { nanoid } from 'nanoid'
```

Then find the `TabNewButton` render site (around line 432) and add `onNewConversation`:

```tsx
<TabNewButton
  isBottomPane={isBottomPane}
  disablePaneActions={disablePaneActions}
  isInSplit={isInSplit}
  onNewConversation={() => { setActivePane(paneId); openContent({ type: 'crowbarChat', wsId: nanoid(), name: 'New conversation' }); }}
  onNewTerminal={() => { setActivePane(paneId); openContent({ type: 'terminal' }); }}
  onOpenUrl={() => { setActivePane(paneId); openContent({ type: 'webViewer', url: 'https://' }); }}
  onClosePane={() => closePane(paneId)}
/>
```

- [ ] **Step 5: Run the test to confirm it passes**

```bash
cd web && npx vitest run src/__tests__/features/tabs/components/tab-new-button.test.tsx
```
Expected: PASS (2 tests).

- [ ] **Step 6: Run TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "tab-new-button\|tab-bar" | head -10
```
Expected: no errors on these files.

- [ ] **Step 7: Commit**

```bash
git add web/src/features/tabs/components/tab-new-button.tsx \
        web/src/features/tabs/components/tab-bar.tsx \
        web/src/__tests__/features/tabs/components/tab-new-button.test.tsx
git commit -m "feat(tabs): add New Conversation to tab bar + menu"
```

---

### Task 2: About tab — `+` button next to Conversations title

**Files:**
- Modify: `web/src/features/branch-review/components/about-tab.tsx`
- Modify: `web/src/features/branch-review/components/branch-review-pane.tsx`
- Create: `web/src/__tests__/features/branch-review/components/about-tab-new-conversation.test.tsx`

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/branch-review/components/about-tab-new-conversation.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { describe, it, expect, vi } from 'vitest'
import { AboutTab } from '@/features/branch-review/components/about-tab'

vi.mock('@/features/branch-review/stores/branch-review-data-store', () => ({
  useBranchReviewDataStore: vi.fn((selector) => selector({
    data: { status: 'success', data: { chats: [] }, fetchedAt: 0 },
    fetch: vi.fn(),
  })),
}))

vi.mock('@uiw/react-codemirror', () => ({ default: () => null }))
vi.mock('@codemirror/lang-markdown', () => ({ markdown: () => ({}) }))

const baseProps = {
  wsId: 'ws-1',
  description: '',
  onDescriptionChange: vi.fn(),
  onOpenConversation: vi.fn(),
  onNewConversation: vi.fn(),
}

describe('AboutTab — new conversation', () => {
  it('renders a new conversation button', () => {
    render(<AboutTab {...baseProps} />)
    expect(screen.getByRole('button', { name: 'New conversation' })).toBeDefined()
  })

  it('calls onNewConversation when the + button is clicked', async () => {
    const onNewConversation = vi.fn()
    render(<AboutTab {...baseProps} onNewConversation={onNewConversation} />)
    await userEvent.click(screen.getByRole('button', { name: 'New conversation' }))
    expect(onNewConversation).toHaveBeenCalledOnce()
  })
})
```

- [ ] **Step 2: Run the test to confirm it fails**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/components/about-tab-new-conversation.test.tsx
```
Expected: FAIL — `onNewConversation` prop does not exist yet.

- [ ] **Step 3: Update `about-tab.tsx`**

In `web/src/features/branch-review/components/about-tab.tsx`:

1. Add `Plus` to the phosphor import and `Button` to the ui import at the top:
```tsx
import { Plus } from '@phosphor-icons/react'
import { Button } from '@/components/ui/button'
```

2. Add `onNewConversation: () => void` to `AboutTabProps`:
```ts
interface AboutTabProps {
  wsId: string
  description: string
  onDescriptionChange: (value: string) => void
  onOpenConversation: (id: string) => void
  onNewConversation: () => void
}
```

3. Destructure it in the function signature:
```ts
export function AboutTab({ wsId, description, onDescriptionChange, onOpenConversation, onNewConversation }: AboutTabProps) {
```

4. Replace the Conversations section header (the bare `<FrameTitle>`) with a flex row containing the title and `+` button. Find this in the file:
```tsx
<FrameTitle className="text-base">Conversations</FrameTitle>
```
Replace with:
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
    <Plus weight="bold" size={12} />
  </Button>
</div>
```

- [ ] **Step 4: Wire `handleNewConversation` in `branch-review-pane.tsx`**

In `web/src/features/branch-review/components/branch-review-pane.tsx`:

1. Add `nanoid` import at the top:
```ts
import { nanoid } from 'nanoid'
```

2. Add `handleNewConversation` alongside the existing `handleOpenConversation` (around line 74):
```tsx
function handleNewConversation() {
  store.getState().bufferActions.openContent({
    type: 'crowbarChat',
    wsId: nanoid(),
    name: 'New conversation',
  })
}
```

3. Pass it to `<AboutTab>` (find the existing `AboutTab` render, around line 149):
```tsx
<AboutTab
  wsId={wsId}
  description={description}
  onDescriptionChange={v => store.getState().setBranchReviewDescription(v)}
  onOpenConversation={handleOpenConversation}
  onNewConversation={handleNewConversation}
/>
```

- [ ] **Step 5: Run the test to confirm it passes**

```bash
cd web && npx vitest run src/__tests__/features/branch-review/components/about-tab-new-conversation.test.tsx
```
Expected: PASS (2 tests).

- [ ] **Step 6: Run TypeScript check**

```bash
cd web && npx tsc --noEmit 2>&1 | grep "about-tab\|branch-review-pane" | head -10
```
Expected: no errors on these files.

- [ ] **Step 7: Run full test suite**

```bash
cd web && npx vitest run 2>&1 | tail -10
```
Expected: all tests pass (pre-existing failures in `editor-api.test.ts` etc. are acceptable).

- [ ] **Step 8: Commit**

```bash
git add web/src/features/branch-review/components/about-tab.tsx \
        web/src/features/branch-review/components/branch-review-pane.tsx \
        web/src/__tests__/features/branch-review/components/about-tab-new-conversation.test.tsx
git commit -m "feat(branch-review): add New Conversation button to About tab"
```
