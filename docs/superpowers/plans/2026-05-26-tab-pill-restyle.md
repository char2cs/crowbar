# IDE Tab Bar — Pill Restyle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restyle the IDE tab bar so active tabs render as `rounded-full bg-muted` pills, inactive tabs are transparent, the close `×` button becomes a 14×14 circular ghost, and the ← → + nav/action buttons become 20×20 circular ghosts — all using shadcn/ui tokens, zero hardcoded colours.

**Architecture:** Pure CSS-class changes only. No logic, no store changes, no behaviour changes. Two components touched: `tab-bar-item.tsx` (tab shape + close button) and `tab-bar.tsx` (four nav/action buttons). All existing DnD, keyboard navigation, context menu, pinning, dirty indicator, and split-pane behaviour is preserved.

**Tech Stack:** React, Tailwind CSS v4, shadcn/ui tokens (`bg-muted`, `text-foreground`, `text-muted-foreground`, `bg-foreground/10`), Vitest + Testing Library.

**Spec:** `docs/superpowers/specs/2026-05-26-tab-restyle-design.md`

---

## File Map

| Action | File | What changes |
|--------|------|-------------|
| Modify | `web/src/features/tabs/components/tab-bar-item.tsx` | `Tab` className → pill shape; close `Button` className → circular ghost |
| Modify | `web/src/features/tabs/components/tab-bar.tsx` | 4 buttons: `rounded-md` → `rounded-full`, `min-w-5 px-1` → `w-5 p-0` |
| Create | `web/src/__tests__/features/tabs/components/tab-bar-item.test.tsx` | Visual-contract tests for the pill restyle |

---

## Task 1: Restyle `tab-bar-item.tsx` — pill + circular close button

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar-item.tsx:111,214-219`
- Create: `web/src/__tests__/features/tabs/components/tab-bar-item.test.tsx`

---

- [ ] **Step 1: Write the failing tests**

Create `web/src/__tests__/features/tabs/components/tab-bar-item.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { describe, it, expect, vi } from 'vitest'
import TabBarItem from '@/features/tabs/components/tab-bar-item'
import type { EditorContent } from '@/features/panes/types/pane-content'

vi.mock('@/features/file-explorer/components/file-explorer-icon', () => ({
  FileExplorerIcon: () => <span data-testid="file-icon" />,
}))

const editorBuffer: EditorContent = {
  id: 'buf-1',
  type: 'editor',
  path: '/project/bar.ts',
  name: 'bar.ts',
  content: '',
  savedContent: '',
  isDirty: false,
  isVirtual: false,
  isPinned: false,
  isPreview: false,
  isActive: false,
  tokens: [],
}

const shared = {
  displayName: 'bar.ts',
  index: 0,
  isDraggedTab: false,
  onDoubleClick: () => {},
  onContextMenu: () => {},
  onKeyDown: () => {},
  handleTabClose: () => {},
  handleTabPin: () => {},
}

describe('TabBarItem pill restyle', () => {
  it('active tab has rounded-full and bg-muted classes', () => {
    render(<TabBarItem buffer={editorBuffer} isActive={true} {...shared} />)
    const tab = screen.getByRole('tab')
    // toHaveClass does exact class-list matching — will not match bg-muted/80
    expect(tab).toHaveClass('rounded-full')
    expect(tab).toHaveClass('bg-muted')
  })

  it('inactive tab has rounded-full but not bg-muted', () => {
    render(<TabBarItem buffer={editorBuffer} isActive={false} {...shared} />)
    const tab = screen.getByRole('tab')
    expect(tab).toHaveClass('rounded-full')
    expect(tab).not.toHaveClass('bg-muted')
  })

  it('active tab does not have bg-muted/80 (old style removed)', () => {
    render(<TabBarItem buffer={editorBuffer} isActive={true} {...shared} />)
    const tab = screen.getByRole('tab')
    // Exact class-list check: 'bg-muted/80' must not be present
    expect(tab).not.toHaveClass('bg-muted/80')
  })

  it('close button has rounded-full class', () => {
    const { container } = render(
      <TabBarItem buffer={editorBuffer} isActive={true} {...shared} />
    )
    // The Tab <button> (role=tab) is buttons[0]; the close Button sibling is buttons[1]
    const closeBtn = container.querySelectorAll('button')[1] as HTMLElement
    expect(closeBtn).toBeDefined()
    expect(closeBtn).toHaveClass('rounded-full')
  })

  it('close button has hover:bg-foreground/10 class', () => {
    const { container } = render(
      <TabBarItem buffer={editorBuffer} isActive={true} {...shared} />
    )
    const closeBtn = container.querySelectorAll('button')[1] as HTMLElement
    expect(closeBtn).toBeDefined()
    expect(closeBtn).toHaveClass('hover:bg-foreground/10')
  })
})
```

---

- [ ] **Step 2: Run tests — verify they FAIL**

```bash
cd web && npx vitest run src/__tests__/features/tabs/components/tab-bar-item.test.tsx
```

Expected: all 5 tests fail. Typical output:
```
FAIL  src/__tests__/features/tabs/components/tab-bar-item.test.tsx
  ✗ active tab has rounded-full and bg-muted classes
  ✗ inactive tab has rounded-full but not bg-muted
  ...
```
If any test passes unexpectedly, re-read the current source before continuing.

---

- [ ] **Step 3: Restyle the `Tab` className in `tab-bar-item.tsx`**

In `web/src/features/tabs/components/tab-bar-item.tsx`, find line ~111:

```tsx
// BEFORE
className={cn("h-5 pl-2 pr-6", isActive && "bg-muted/80")}
```

Replace with:

```tsx
// AFTER
className={cn(
  "h-5 rounded-full",
  isActive ? "bg-muted pl-2 pr-5" : "bg-transparent pl-2 pr-2",
)}
```

> `pr-5` (20px) reserves space for the absolute-positioned close button (14px wide + 6px right offset). `bg-transparent` makes inactive tabs visually empty while keeping the hover target.

---

- [ ] **Step 4: Restyle the close/pin `Button` className in `tab-bar-item.tsx`**

Same file, find the `Button` for close/pin (~line 214). It currently looks like:

```tsx
className={cn(
  "-translate-y-1/2 absolute top-1/2 right-1 h-4 min-w-4 cursor-pointer select-none rounded-sm px-0 text-muted-foreground transition-opacity",
  "hover:text-foreground",
  buffer.isPinned || isActive ? "opacity-100" : "opacity-0 group-hover/tab:opacity-100",
)}
```

Replace with:

```tsx
className={cn(
  "-translate-y-1/2 absolute top-1/2 right-1 h-3.5 min-w-3.5 cursor-pointer select-none rounded-full p-0 text-muted-foreground transition-opacity",
  "hover:bg-foreground/10 hover:text-foreground",
  buffer.isPinned || isActive ? "opacity-60" : "opacity-0 group-hover/tab:opacity-100",
)}
```

Changes:
- `rounded-sm` → `rounded-full` (circle)
- `h-4 min-w-4` → `h-3.5 min-w-3.5` (14px — fits inside the pill)
- `px-0` → `p-0`
- Added `hover:bg-foreground/10` (circular highlight on hover)
- `opacity-100` → `opacity-60` for active/pinned at rest (subtle, not full opacity)

---

- [ ] **Step 5: Run tests — verify they PASS**

```bash
cd web && npx vitest run src/__tests__/features/tabs/components/tab-bar-item.test.tsx
```

Expected:
```
PASS  src/__tests__/features/tabs/components/tab-bar-item.test.tsx
  ✓ active tab has rounded-full and bg-muted classes
  ✓ inactive tab has rounded-full but not bg-muted
  ✓ active tab does not have bg-muted/80 (old style removed)
  ✓ close button has rounded-full class
  ✓ close button has hover:bg-foreground/10 class
```

---

- [ ] **Step 6: Run the full test suite — verify no regressions**

```bash
cd web && npm test
```

Expected: all previously passing tests still pass. Fix any failures before committing.

---

- [ ] **Step 7: Commit**

```bash
git add \
  web/src/features/tabs/components/tab-bar-item.tsx \
  web/src/__tests__/features/tabs/components/tab-bar-item.test.tsx
git commit -m "feat: restyle tab bar items as rounded-full pills

- Active tab: bg-muted pill (rounded-full, pr-5 for close button room)
- Inactive tab: transparent, rounded-full, pr-2
- Close button: 14×14 rounded-full ghost, hover:bg-foreground/10, opacity-60 at rest
- All shadcn/ui tokens — zero hardcoded colours

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```

---

## Task 2: Restyle nav and action buttons in `tab-bar.tsx` — circular ghosts

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar.tsx` (4 button classNames)

> `tab-bar.tsx` has ~15 store hooks making it impractical to unit-test in isolation. The change is four one-word substitutions (`rounded-md` → `rounded-full`, `min-w-5 px-1` → `w-5 p-0`). Visual correctness is confirmed by running the app after the commit.

---

- [ ] **Step 1: Update the Back (←) button**

In `web/src/features/tabs/components/tab-bar.tsx`, find the ArrowLeft `Button` (~line 728). Its className currently contains `rounded-md`:

```tsx
// BEFORE
className="h-5 min-w-5 shrink-0 rounded-md px-1 text-muted-foreground"
```

Replace with:

```tsx
// AFTER
className="h-5 w-5 shrink-0 rounded-full p-0 text-muted-foreground"
```

(`min-w-5` → `w-5` so width is fixed at 20px; `px-1` → `p-0` so the button is a true 20×20 circle.)

---

- [ ] **Step 2: Update the Forward (→) button**

Same file, find the ArrowRight `Button` (~line 740). Apply the identical change:

```tsx
// BEFORE
className="h-5 min-w-5 shrink-0 rounded-md px-1 text-muted-foreground"

// AFTER
className="h-5 w-5 shrink-0 rounded-full p-0 text-muted-foreground"
```

---

- [ ] **Step 3: Update the + (new tab) DropdownMenuTrigger**

Same file, find the `DropdownMenuTrigger` with the Plus icon (~line 787):

```tsx
// BEFORE
className="flex h-5 min-w-5 shrink-0 items-center justify-center rounded-md px-1 text-muted-foreground hover:bg-muted hover:text-foreground focus:outline-none"

// AFTER
className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full p-0 text-muted-foreground hover:bg-muted hover:text-foreground focus:outline-none"
```

---

- [ ] **Step 4: Update the Close Split (⊡) button**

Same file, find the `PanelLeftClose` `Button` (~line 803):

```tsx
// BEFORE
className="h-5 min-w-5 shrink-0 rounded-md px-1 text-muted-foreground"

// AFTER
className="h-5 w-5 shrink-0 rounded-full p-0 text-muted-foreground"
```

---

- [ ] **Step 5: Run the full test suite — verify no regressions**

```bash
cd web && npm test
```

Expected: all previously passing tests still pass.

---

- [ ] **Step 6: Commit**

```bash
git add web/src/features/tabs/components/tab-bar.tsx
git commit -m "feat: restyle tab bar nav/action buttons as circular ghosts

- Back, Forward, +, Close-Split buttons: rounded-full w-5 h-5 p-0
- True 20×20 circles, ghost at rest, bg-muted on hover (existing)
- Zero hardcoded colours

Co-Authored-By: Claude Sonnet 4.6 <noreply@anthropic.com>"
```
