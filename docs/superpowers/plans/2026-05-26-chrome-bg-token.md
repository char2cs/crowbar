# `--chrome-bg` Theme Token Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a single `--chrome-bg` CSS design token to the shadcn/ui theme so both the tab bar and sidebar share one theme-overridable transparent surface color.

**Architecture:** The token is defined in `index.css` with light (50% opacity) and dark (75% opacity) values. It is exposed as a Tailwind color via `@theme inline`. The tab bar and sidebar switch from their current hard-coded or runtime-variable backgrounds to `bg-chrome-bg`.

**Tech Stack:** Tailwind v4, CSS custom properties (oklch), Vitest, React Testing Library

---

## File Map

| File | Change |
|------|--------|
| `web/src/index.css` | Add `--chrome-bg` to `@theme inline`, `:root`, and `.dark` |
| `web/src/components/ui/sidebar.tsx` | `SidebarHeader` and `SidebarFooter` class strings |
| `web/src/__tests__/components/ui/sidebar.test.tsx` | New — render tests for the class change |
| `web/src/features/tabs/components/tab-bar.tsx` | Container div class string |

---

## Task 1: Add `--chrome-bg` to the theme

**Files:**
- Modify: `web/src/index.css`

- [ ] **Step 1: Add the Tailwind mapping inside `@theme inline`**

  Open `web/src/index.css`. The `@theme inline` block ends at the `--font-editor` line (~line 53). Add the new entry as the **last line** inside that block, right before the closing `}`:

  ```css
  /* existing last line */
  --font-editor: var(--editor-font-family);
  /* add: */
  --color-chrome-bg: var(--chrome-bg);
  ```

- [ ] **Step 2: Add the light-mode value to `:root`**

  Inside the `:root` block, at the end of the `/* custom tokens */` section (around line 103, after `--app-scrollbar-radius`):

  ```css
  --app-scrollbar-radius: 999px;
  /* add: */
  --chrome-bg: oklch(1 0 0 / 50%);
  ```

- [ ] **Step 3: Add the dark-mode value to `.dark`**

  Inside the `.dark` block, at the end of the `/* custom tokens — dark overrides */` section (after `--info: oklch(0.72 0.15 250);`):

  ```css
  --info: oklch(0.72 0.15 250);
  /* add: */
  --chrome-bg: oklch(0.148 0.004 228.8 / 75%);
  ```

- [ ] **Step 4: Verify the build passes**

  ```bash
  cd web && npm run build 2>&1 | tail -20
  ```

  Expected: exits 0. No "unknown utility" or CSS parse errors.

- [ ] **Step 5: Commit**

  ```bash
  git add web/src/index.css
  git commit -m "feat: add --chrome-bg theme token to shadcn/ui theme"
  ```

---

## Task 2: Update sidebar — TDD

`SidebarHeader` and `SidebarFooter` are small, dependency-free components. They are ideal candidates for render tests.

**Files:**
- Modify: `web/src/components/ui/sidebar.tsx` — lines 54 and 79
- Create: `web/src/__tests__/components/ui/sidebar.test.tsx`

- [ ] **Step 1: Write the failing tests**

  Create `web/src/__tests__/components/ui/sidebar.test.tsx`:

  ```tsx
  import { render } from '@testing-library/react'
  import { describe, it, expect } from 'vitest'
  import { SidebarHeader, SidebarFooter } from '@/components/ui/sidebar'

  describe('SidebarHeader', () => {
    it('uses bg-chrome-bg for its background', () => {
      const { container } = render(<SidebarHeader>test</SidebarHeader>)
      const el = container.firstChild as HTMLElement
      expect(el.className).toContain('bg-chrome-bg')
    })

    it('keeps backdrop-blur-sm for the frosted glass effect', () => {
      const { container } = render(<SidebarHeader>test</SidebarHeader>)
      const el = container.firstChild as HTMLElement
      expect(el.className).toContain('backdrop-blur-sm')
    })
  })

  describe('SidebarFooter', () => {
    it('uses bg-chrome-bg for its background in default (non-surface) mode', () => {
      const { container } = render(<SidebarFooter>test</SidebarFooter>)
      const el = container.firstChild as HTMLElement
      expect(el.className).toContain('bg-chrome-bg')
    })

    it('does not include bg-primary-bg in default mode', () => {
      const { container } = render(<SidebarFooter>test</SidebarFooter>)
      const el = container.firstChild as HTMLElement
      expect(el.className).not.toContain('bg-primary-bg')
    })
  })
  ```

- [ ] **Step 2: Run the tests — confirm they fail**

  ```bash
  cd web && npx vitest run src/__tests__/components/ui/sidebar.test.tsx
  ```

  Expected: 4 failing tests. `SidebarHeader` and `SidebarFooter` still have `bg-primary-bg/95`.

- [ ] **Step 3: Update `SidebarFooter` in `sidebar.tsx`**

  File: `web/src/components/ui/sidebar.tsx`, line 54. Change:

  ```tsx
  // before
  "shrink-0 bg-primary-bg/95 px-2 py-2",
  // after
  "shrink-0 bg-chrome-bg px-2 py-2",
  ```

- [ ] **Step 4: Update `SidebarHeader` in `sidebar.tsx`**

  Same file, line 79. Change:

  ```tsx
  // before
  "sticky top-0 z-20 flex h-8 shrink-0 select-none items-center gap-1.5 bg-primary-bg/95 px-1.5 py-1 backdrop-blur-sm",
  // after
  "sticky top-0 z-20 flex h-8 shrink-0 select-none items-center gap-1.5 bg-chrome-bg px-1.5 py-1 backdrop-blur-sm",
  ```

- [ ] **Step 5: Run the tests — confirm they pass**

  ```bash
  cd web && npx vitest run src/__tests__/components/ui/sidebar.test.tsx
  ```

  Expected: 4 passing tests.

- [ ] **Step 6: Run the full suite — confirm no regressions**

  ```bash
  cd web && npm run test 2>&1 | tail -20
  ```

  Expected: all tests pass (exit 0).

- [ ] **Step 7: Commit**

  ```bash
  git add web/src/components/ui/sidebar.tsx web/src/__tests__/components/ui/sidebar.test.tsx
  git commit -m "feat: sidebar chrome surfaces use --chrome-bg token"
  ```

---

## Task 3: Update tab bar

`TabBar` pulls in DnD, multiple Zustand stores, and workspace context — too many layers to render cheaply in a test. The change is a single class swap on one div; the build and visual check are the right verification gates here.

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar.tsx` — line 720

- [ ] **Step 1: Update the container className**

  File: `web/src/features/tabs/components/tab-bar.tsx`, line 720. Change:

  ```tsx
  // before
  className="relative flex h-7 shrink-0 items-center gap-1 overflow-hidden bg-background px-1.5 py-0.5"
  // after
  className="relative flex h-7 shrink-0 items-center gap-1 overflow-hidden bg-chrome-bg backdrop-blur-sm px-1.5 py-0.5"
  ```

- [ ] **Step 2: Verify the build**

  ```bash
  cd web && npm run build 2>&1 | tail -20
  ```

  Expected: exits 0. No "bg-chrome-bg" utility warnings.

- [ ] **Step 3: Run the full test suite — confirm no regressions**

  ```bash
  cd web && npm run test 2>&1 | tail -20
  ```

  Expected: all tests pass.

- [ ] **Step 4: Commit**

  ```bash
  git add web/src/features/tabs/components/tab-bar.tsx
  git commit -m "feat: tab bar chrome surface uses --chrome-bg token with backdrop-blur-sm"
  ```

---

## Done — Theme Override Reminder

Any Crowbar theme can now override `--chrome-bg` anywhere a CSS scope is applied:

```css
/* example: a warm-tinted theme */
--chrome-bg: oklch(0.18 0.01 45 / 80%);
```

Both the tab bar and sidebar pick it up automatically — no component changes needed.
