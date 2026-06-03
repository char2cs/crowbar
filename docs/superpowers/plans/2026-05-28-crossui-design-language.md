# Cross/UI Design Language Adoption — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adopt Cross/UI's font, color tokens, and component library across the Crowbar web app, splitting CSS into a UI theme file and a separate editor theme file, while preserving all Crowbar-specific component extensions.

**Architecture:** Three phases. Phase 1 splits `index.css` into `styles/theme.css` (Cross/UI tokens + Cal Sans fonts) and `styles/editor-theme.css` (JetBrains Mono + syntax tokens), leaving `index.css` as an import-only entry point. Phase 2 installs all Cross/UI components via the shadcn CLI then restores Crowbar-specific extensions on Group B components. Phase 3 greps for and replaces hardcoded color values throughout `web/src/`.

**Tech Stack:** React 19, Tailwind CSS v4, `@base-ui/react` v1.5, shadcn CLI (`npx shadcn@latest`), `components.json` with `@coss` registry already configured at `https://coss.com/ui/r/{name}.json`. Package manager: `npm`. Cal Sans font files are already at `web/public/fonts/`.

---

## Files

| Action | Path |
|---|---|
| Create | `web/src/styles/theme.css` |
| Create | `web/src/styles/editor-theme.css` |
| Modify | `web/src/index.css` |
| Overwrite (CLI) | `web/src/components/ui/*.tsx` (all Group A + B) |
| Restore | `web/src/components/ui/tabs.tsx` |
| Restore | `web/src/components/ui/tooltip.tsx` |
| Verify/restore | `web/src/components/ui/button.tsx` |
| Verify/restore | `web/src/components/ui/input.tsx` |
| Verify/restore | `web/src/components/ui/switch.tsx` |
| Sweep (many) | `web/src/**/*.{tsx,ts,css}` (hardcoded color removal) |

---

## Task 1: Create `web/src/styles/theme.css`

**Files:**
- Create: `web/src/styles/theme.css`

- [ ] **Step 1.1 — Create the styles directory and write theme.css**

```bash
mkdir -p web/src/styles
```

Write `web/src/styles/theme.css` with this exact content:

```css
/* Cal Sans UI — body/UI font (variable, wght 300–700) */
@font-face {
  font-family: 'CalSansUI';
  src: url('/fonts/CalSansUI.woff2') format('woff2');
  font-weight: 300 700;
  font-display: swap;
}

/* Cal Sans — heading font */
@font-face {
  font-family: 'CalSans';
  src: url('/fonts/CalSans-Regular.woff2') format('woff2');
  font-weight: 400 600;
  font-display: swap;
}

@theme inline {
  --font-sans: 'CalSansUI', sans-serif;
  --font-heading: 'CalSans', sans-serif;
  --color-background: var(--background);
  --color-foreground: var(--foreground);
  --color-card: var(--card);
  --color-card-foreground: var(--card-foreground);
  --color-popover: var(--popover);
  --color-popover-foreground: var(--popover-foreground);
  --color-primary: var(--primary);
  --color-primary-foreground: var(--primary-foreground);
  --color-secondary: var(--secondary);
  --color-secondary-foreground: var(--secondary-foreground);
  --color-muted: var(--muted);
  --color-muted-foreground: var(--muted-foreground);
  --color-accent: var(--accent);
  --color-accent-foreground: var(--accent-foreground);
  --color-destructive: var(--destructive);
  --color-destructive-foreground: var(--destructive-foreground);
  --color-border: var(--border);
  --color-input: var(--input);
  --color-ring: var(--ring);
  --color-chart-1: var(--chart-1);
  --color-chart-2: var(--chart-2);
  --color-chart-3: var(--chart-3);
  --color-chart-4: var(--chart-4);
  --color-chart-5: var(--chart-5);
  --color-sidebar: var(--sidebar);
  --color-sidebar-foreground: var(--sidebar-foreground);
  --color-sidebar-primary: var(--sidebar-primary);
  --color-sidebar-primary-foreground: var(--sidebar-primary-foreground);
  --color-sidebar-accent: var(--sidebar-accent);
  --color-sidebar-accent-foreground: var(--sidebar-accent-foreground);
  --color-sidebar-border: var(--sidebar-border);
  --color-sidebar-ring: var(--sidebar-ring);
  --color-success: var(--success);
  --color-success-foreground: var(--success-foreground);
  --color-warning: var(--warning);
  --color-warning-foreground: var(--warning-foreground);
  --color-info: var(--info);
  --color-info-foreground: var(--info-foreground);
  --color-code: var(--code);
  --color-code-foreground: var(--code-foreground);
  --color-code-highlight: var(--code-highlight);
  --color-chrome-bg: var(--chrome-bg);
  --font-editor: var(--editor-font-family);
  --radius-sm: calc(var(--radius) * 0.6);
  --radius-md: calc(var(--radius) * 0.8);
  --radius-lg: var(--radius);
  --radius-xl: calc(var(--radius) * 1.4);
  --radius-2xl: calc(var(--radius) * 1.8);
  --radius-3xl: calc(var(--radius) * 2.2);
  --radius-4xl: calc(var(--radius) * 2.6);
}

:root {
  /* ── Cross/UI light-mode tokens ── */
  --radius: .625rem;
  --background: var(--color-white);
  --foreground: var(--color-neutral-800);
  --card: var(--color-white);
  --card-foreground: var(--color-neutral-800);
  --popover: var(--color-white);
  --popover-foreground: var(--color-neutral-800);
  --primary: var(--color-neutral-800);
  --primary-foreground: var(--color-neutral-50);
  --secondary: #0000000a;
  --secondary-foreground: var(--color-neutral-800);
  --muted: #0000000a;
  --muted-foreground: #686868;
  --accent: #0000000a;
  --accent-foreground: var(--color-neutral-800);
  --destructive: var(--color-red-500);
  --destructive-foreground: var(--color-red-700);
  --info: var(--color-blue-500);
  --info-foreground: var(--color-blue-700);
  --success: var(--color-emerald-500);
  --success-foreground: var(--color-emerald-700);
  --warning: var(--color-amber-500);
  --warning-foreground: var(--color-amber-700);
  --border: #00000014;
  --input: #0000001a;
  --ring: var(--color-neutral-400);
  --chart-1: var(--color-orange-600);
  --chart-2: var(--color-teal-600);
  --chart-3: var(--color-cyan-900);
  --chart-4: var(--color-amber-400);
  --chart-5: var(--color-amber-500);
  --sidebar: var(--color-neutral-50);
  --sidebar-foreground: #262626;
  --sidebar-primary: var(--color-neutral-800);
  --sidebar-primary-foreground: var(--color-neutral-50);
  --sidebar-accent: #0000000a;
  --sidebar-accent-foreground: var(--color-neutral-800);
  --sidebar-border: #0000000f;
  --sidebar-ring: var(--color-neutral-400);
  --code: var(--color-white);
  --code-foreground: var(--foreground);
  --code-highlight: #0000000a;

  /* ── Crowbar UI-layer tokens (not in Cross/UI) ── */
  --chrome-bg: oklch(1 0 0 / 75%);
  --app-ui-scale: 1;
  --ui-text-xs: calc(0.6875rem * var(--app-ui-scale));
  --ui-text-sm: calc(0.75rem * var(--app-ui-scale));
  --ui-text-base: calc(0.875rem * var(--app-ui-scale));
  --app-scrollbar-size: 11px;
  --app-scrollbar-thumb: oklch(0.55 0 0 / 42%);
  --app-scrollbar-thumb-border: 3px solid transparent;
  --app-scrollbar-thumb-hover: oklch(0.55 0 0 / 58%);
  --app-scrollbar-track: transparent;
  --app-scrollbar-radius: 999px;
}

.dark {
  /* ── Cross/UI dark-mode tokens ── */
  --background: #141414;
  --foreground: var(--color-neutral-100);
  --card: var(--background);
  --card-foreground: var(--color-neutral-100);
  --popover: var(--background);
  --popover-foreground: var(--color-neutral-100);
  --primary: var(--color-neutral-100);
  --primary-foreground: var(--color-neutral-800);
  --secondary: #ffffff0a;
  --secondary-foreground: var(--color-neutral-100);
  --muted: #ffffff0a;
  --muted-foreground: #818181;
  --accent: #ffffff0a;
  --accent-foreground: var(--color-neutral-100);
  --destructive: #fb414a;
  --destructive-foreground: var(--color-red-400);
  --info: var(--color-blue-500);
  --info-foreground: var(--color-blue-400);
  --success: var(--color-emerald-500);
  --success-foreground: var(--color-emerald-400);
  --warning: var(--color-amber-500);
  --warning-foreground: var(--color-amber-400);
  --border: #ffffff0f;
  --input: #ffffff14;
  --ring: var(--color-neutral-500);
  --chart-1: var(--color-blue-700);
  --chart-2: var(--color-emerald-500);
  --chart-3: var(--color-amber-500);
  --chart-4: var(--color-purple-500);
  --chart-5: var(--color-rose-500);
  --sidebar: #111;
  --sidebar-foreground: #f5f5f5;
  --sidebar-primary: var(--color-neutral-100);
  --sidebar-primary-foreground: var(--color-neutral-800);
  --sidebar-accent: #ffffff0a;
  --sidebar-accent-foreground: var(--color-neutral-100);
  --sidebar-border: #ffffff0d;
  --sidebar-ring: var(--color-neutral-400);
  --code: var(--background);
  --code-foreground: var(--foreground);
  --code-highlight: #ffffff0a;

  /* ── Crowbar UI-layer dark overrides ── */
  --chrome-bg: oklch(0.148 0.004 228.8 / 85%);
}
```

---

## Task 2: Create `web/src/styles/editor-theme.css`

**Files:**
- Create: `web/src/styles/editor-theme.css`

- [ ] **Step 2.1 — Write editor-theme.css**

Write `web/src/styles/editor-theme.css` with this exact content:

```css
@import "@fontsource-variable/jetbrains-mono";

:root {
  --editor-font-family: ui-monospace, 'JetBrains Mono Variable', monospace;

  /* Syntax highlighting — light mode */
  --syntax-keyword: oklch(0.45 0.18 310);
  --syntax-string: oklch(0.42 0.14 140);
  --syntax-number: oklch(0.50 0.15 45);
  --syntax-constant: oklch(0.48 0.12 215);
  --syntax-comment: oklch(0.52 0.01 255);
  --syntax-variable: oklch(0.45 0.15 15);
  --syntax-property: oklch(0.45 0.15 270);
  --syntax-type: oklch(0.52 0.16 95);
  --syntax-function: oklch(0.45 0.15 220);
  --syntax-operator: oklch(0.48 0.12 215);
  --syntax-punctuation: oklch(0.25 0 0);
  --syntax-tag: oklch(0.45 0.15 15);
  --syntax-attribute: oklch(0.45 0.18 310);
  --syntax-boolean: oklch(0.42 0.22 15);
  --syntax-null: oklch(0.42 0.22 15);
  --syntax-regex: oklch(0.48 0.12 215);
  --syntax-jsx: oklch(0.48 0.12 215);
  --syntax-jsx-attribute: var(--syntax-attribute);
  --syntax-markdown-heading: oklch(0.45 0.15 270);
  --syntax-markdown-bold: oklch(0.50 0.15 45);
  --syntax-markdown-italic: oklch(0.45 0.18 310);
  --syntax-markdown-strikethrough: oklch(0.45 0.15 15);
  --syntax-markdown-link: oklch(0.48 0.12 215);
  --syntax-markdown-link-text: oklch(0.45 0.15 270);
  --syntax-markdown-code: oklch(0.42 0.14 140);
  --syntax-markdown-list: oklch(0.45 0.18 310);
  --syntax-markdown-quote: oklch(0.52 0.01 255);
}

.dark {
  /* Syntax highlighting — dark mode */
  --syntax-keyword: oklch(0.68 0.18 310);
  --syntax-string: oklch(0.85 0.14 130);
  --syntax-number: oklch(0.75 0.15 45);
  --syntax-constant: oklch(0.85 0.09 215);
  --syntax-comment: oklch(0.62 0.01 255);
  --syntax-variable: oklch(0.70 0.15 15);
  --syntax-property: oklch(0.72 0.15 270);
  --syntax-type: oklch(0.87 0.16 95);
  --syntax-function: oklch(0.72 0.15 220);
  --syntax-operator: oklch(0.85 0.09 215);
  --syntax-punctuation: oklch(0.90 0 0);
  --syntax-tag: oklch(0.70 0.15 15);
  --syntax-attribute: oklch(0.68 0.18 310);
  --syntax-boolean: oklch(0.65 0.22 15);
  --syntax-null: oklch(0.65 0.22 15);
  --syntax-regex: oklch(0.85 0.09 215);
  --syntax-jsx: oklch(0.85 0.09 215);
  --syntax-jsx-attribute: var(--syntax-attribute);
  --syntax-markdown-heading: oklch(0.72 0.15 270);
  --syntax-markdown-bold: oklch(0.75 0.15 45);
  --syntax-markdown-italic: oklch(0.68 0.18 310);
  --syntax-markdown-strikethrough: oklch(0.70 0.15 15);
  --syntax-markdown-link: oklch(0.85 0.09 215);
  --syntax-markdown-link-text: oklch(0.72 0.15 270);
  --syntax-markdown-code: oklch(0.85 0.14 130);
  --syntax-markdown-list: oklch(0.68 0.18 310);
  --syntax-markdown-quote: oklch(0.56 0.01 255);
}
```

---

## Task 3: Update `web/src/index.css`

**Files:**
- Modify: `web/src/index.css`

- [ ] **Step 3.1 — Replace index.css entirely**

Replace the full content of `web/src/index.css` with:

```css
@import "tailwindcss";
@import "tw-animate-css";
@import "./styles/theme.css";
@import "./styles/editor-theme.css";

@custom-variant dark (&:is(.dark *));

@layer base {
  * {
    @apply border-border outline-ring/50;
  }
  body {
    @apply bg-chrome-bg text-foreground;
  }
  html {
    @apply font-sans;
  }
}

@utility ui-font {
  font-family: var(--app-font-family, var(--font-sans));
}

@utility ui-text-xs {
  font-size: var(--ui-text-xs);
}

@utility ui-text-sm {
  font-size: var(--ui-text-sm);
}

@utility ui-text-base {
  font-size: var(--ui-text-base);
}
```

- [ ] **Step 3.2 — Run the full test suite**

```bash
cd web && npm test -- --run
```

Expected: no failures. If any test fails referencing a missing CSS var, check that the var name exists in `theme.css` or `editor-theme.css`.

- [ ] **Step 3.3 — Commit Phase 1**

```bash
git add web/src/index.css web/src/styles/
git commit -m "feat: split theme into theme.css + editor-theme.css, adopt Cross/UI tokens and Cal Sans font"
```

---

## Task 4: Install Cross/UI components via CLI

**Files:**
- Overwrite: all files in `web/src/components/ui/` that exist in Cross/UI

- [ ] **Step 4.1 — Run the Cross/UI CLI installer**

From the repo root:

```bash
cd web && npx shadcn@latest add @coss/ui --overwrite
```

When prompted "Are you sure you want to overwrite?" answer **yes**. The `components.json` already has the `@coss` registry at `https://coss.com/ui/r/{name}.json` so no additional config is needed.

- [ ] **Step 4.2 — Check what changed**

```bash
git diff --name-only web/src/components/ui/
```

Note which files changed. The Group C files (Crowbar-specific, no Cross/UI counterpart) should NOT appear in this list: `pane.tsx`, `sidebar-tree.tsx`, `tree-row.tsx`, `number-input.tsx`, `search.tsx`, `keybinding.tsx`, `primitive-dialog-service.tsx`, `dropdown.tsx`, `toast.tsx`.

If any Group C file was overwritten, restore it immediately:

```bash
git checkout HEAD -- web/src/components/ui/pane.tsx
git checkout HEAD -- web/src/components/ui/sidebar-tree.tsx
git checkout HEAD -- web/src/components/ui/tree-row.tsx
git checkout HEAD -- web/src/components/ui/number-input.tsx
git checkout HEAD -- web/src/components/ui/search.tsx
git checkout HEAD -- web/src/components/ui/keybinding.tsx
git checkout HEAD -- web/src/components/ui/primitive-dialog-service.tsx
git checkout HEAD -- web/src/components/ui/dropdown.tsx
git checkout HEAD -- web/src/components/ui/toast.tsx
```

- [ ] **Step 4.3 — Run tests to see baseline failures**

```bash
cd web && npm test -- --run 2>&1 | tail -40
```

Note which tests fail — these are the ones that need Crowbar extensions restored (Tasks 5–8).

- [ ] **Step 4.4 — Commit the CLI-installed components**

```bash
git add web/src/components/ui/
git commit -m "feat: install Cross/UI components via shadcn CLI"
```

---

## Task 5: Restore `tabs.tsx` — standalone Tab component

The Cross/UI `tabs.tsx` uses `@base-ui/react` tabs and will have dropped the standalone `Tab` component and `TabsItem` type used throughout the editor tab bars.

**Files:**
- Modify: `web/src/components/ui/tabs.tsx`

- [ ] **Step 5.1 — Read the new tabs.tsx from the CLI install**

```bash
cat web/src/components/ui/tabs.tsx
```

Verify that `Tab` is no longer exported. If it still is, skip the remaining steps in this task.

- [ ] **Step 5.2 — Append the Crowbar Tab component and TabsItem type**

Add this block to the **end** of `web/src/components/ui/tabs.tsx`, before any existing export statement:

```tsx
/** Standalone tab button used by Crowbar feature modules (does not require Radix/base-ui value prop) */
export interface TabProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  isActive?: boolean
  isDragged?: boolean
  action?: React.ReactNode
  variant?: string
  size?: "xs" | "sm" | "md" | "lg"
  labelPosition?: "start" | "center" | "end"
  maxWidth?: number
}

const Tab = React.forwardRef<HTMLButtonElement, TabProps>(
  (
    {
      className,
      isActive,
      isDragged: _isDragged,
      action,
      variant: _variant,
      size: _size,
      labelPosition: _labelPosition,
      maxWidth: _maxWidth,
      children,
      ...props
    },
    ref,
  ) => (
    <button
      ref={ref}
      className={cn(
        "relative inline-flex shrink-0 cursor-pointer items-center whitespace-nowrap rounded-lg border font-medium text-sm outline-none transition-colors",
        "focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background",
        "disabled:pointer-events-none disabled:opacity-64",
        isActive
          ? "border-border bg-background text-foreground"
          : "border-transparent text-muted-foreground hover:bg-accent hover:text-foreground",
        className,
      )}
      {...props}
    >
      {children}
      {action}
    </button>
  ),
)
Tab.displayName = "Tab"

/** Crowbar tab item descriptor */
export interface TabsItem {
  id: string
  icon?: React.ReactNode
  label?: string
  isActive?: boolean
  onClick?: () => void
  role?: string
  ariaLabel?: string
  className?: string
  tabIndex?: number
  title?: string
  tooltip?: {
    content: string
    shortcut?: string
    side?: "top" | "right" | "bottom" | "left"
    className?: string
  }
}
```

Add `Tab` to the export list at the bottom of the file. The `cn` import and `React` import must already be present — if not, add them.

- [ ] **Step 5.3 — Run tests**

```bash
cd web && npm test -- --run src/__tests__ 2>&1 | grep -E "FAIL|PASS|Error" | head -30
```

Expected: tests that use `Tab` now pass.

- [ ] **Step 5.4 — Commit**

```bash
git add web/src/components/ui/tabs.tsx
git commit -m "feat: restore Tab standalone component on Cross/UI tabs.tsx"
```

---

## Task 6: Restore `tooltip.tsx` — compound API + Radix named exports

Cross/UI's tooltip likely uses `@base-ui/react/tooltip`. Crowbar's `tooltip.tsx` exposes: a default compound `<TooltipCompound content="..." />` plus `Tooltip`, `TooltipTrigger`, `TooltipContent`, `TooltipPortal`, `TooltipProvider` as named exports (Radix-based, used in many call sites).

**Files:**
- Modify: `web/src/components/ui/tooltip.tsx`

- [ ] **Step 6.1 — Read the CLI-installed tooltip.tsx**

```bash
cat web/src/components/ui/tooltip.tsx
```

Check whether it exports: `TooltipProvider`, `Tooltip`, `TooltipTrigger`, `TooltipContent`, `TooltipPortal`, and a default compound export.

- [ ] **Step 6.2 — If any Crowbar export is missing, replace the file entirely**

If the Cross/UI version dropped the compound default or the Radix-based named exports, restore this exact content to `web/src/components/ui/tooltip.tsx`:

```tsx
import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import { cva } from "class-variance-authority";
import type React from "react";
import Keybinding from "@/components/ui/keybinding";
import { cn } from "@/utils/cn";

interface TooltipProps {
  content: string;
  children: React.ReactNode;
  side?: "top" | "bottom" | "left" | "right";
  shortcut?: string;
  className?: string;
  triggerClassName?: string;
}

const tooltipContentVariants = cva(
  "ui-text-sm pointer-events-none z-[99999] whitespace-nowrap rounded-lg border border-border/70 bg-card/95 px-2.5 py-1.5 text-foreground shadow-lg backdrop-blur-sm animate-in fade-in-0 zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[side=bottom]:slide-in-from-top-1 data-[side=left]:slide-in-from-right-1 data-[side=right]:slide-in-from-left-1 data-[side=top]:slide-in-from-bottom-1",
);

export function TooltipProvider({ children }: { children: React.ReactNode }) {
  return (
    <TooltipPrimitive.Provider delayDuration={150} skipDelayDuration={100} disableHoverableContent>
      {children}
    </TooltipPrimitive.Provider>
  );
}

export const Tooltip = TooltipPrimitive.Root
export const TooltipTrigger = TooltipPrimitive.Trigger
export const TooltipContent = TooltipPrimitive.Content
export const TooltipPortal = TooltipPrimitive.Portal

export default function TooltipCompound({
  content,
  children,
  side = "top",
  shortcut,
  className,
  triggerClassName,
}: TooltipProps) {
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger asChild>
        <span className={cn("inline-flex items-center", triggerClassName)}>{children}</span>
      </TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content
          side={side}
          sideOffset={6}
          collisionPadding={8}
          className={cn(tooltipContentVariants(), shortcut && "flex items-center gap-2", className)}
        >
          {content}
          {shortcut && <Keybinding binding={shortcut} />}
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}
```

- [ ] **Step 6.3 — Run tests**

```bash
cd web && npm test -- --run 2>&1 | grep -E "FAIL|PASS" | head -20
```

- [ ] **Step 6.4 — Commit**

```bash
git add web/src/components/ui/tooltip.tsx
git commit -m "feat: restore compound tooltip API on Cross/UI tooltip.tsx"
```

---

## Task 7: Verify button, input, switch — restore if needed

These three were already adapted before the CLI install. Check whether the CLI overwrote them with a version that lost Crowbar extensions.

**Files:**
- Verify/restore: `web/src/components/ui/button.tsx`
- Verify/restore: `web/src/components/ui/input.tsx`
- Verify/restore: `web/src/components/ui/switch.tsx`

- [ ] **Step 7.1 — Check button.tsx**

```bash
grep -n "active\|loading\|commandId\|compact\|tooltip" web/src/components/ui/button.tsx | head -20
```

Expected: lines showing `active`, `loading`, `commandId`, `compact`, `tooltip` props. If missing, restore from git:

```bash
git show HEAD~1:web/src/components/ui/button.tsx > web/src/components/ui/button.tsx
```

(Adjust `HEAD~1` to whichever commit last had the adapted button — use `git log --oneline web/src/components/ui/button.tsx` to find it.)

- [ ] **Step 7.2 — Check input.tsx**

```bash
grep -n "leftIcon\|containerClassName\|LeftIcon" web/src/components/ui/input.tsx | head -10
```

Expected: lines showing `leftIcon` prop. If missing, restore from git the same way as Step 7.1.

- [ ] **Step 7.3 — Check switch.tsx**

```bash
grep -n "onChange\|size.*sm\|data-size" web/src/components/ui/switch.tsx | head -10
```

Expected: lines showing `onChange` and `size` handling. If missing, restore from git.

- [ ] **Step 7.4 — Run tests**

```bash
cd web && npm test -- --run 2>&1 | grep -E "FAIL|PASS" | head -20
```

- [ ] **Step 7.5 — Commit if any restores were done**

```bash
git add web/src/components/ui/button.tsx web/src/components/ui/input.tsx web/src/components/ui/switch.tsx
git commit -m "feat: restore Crowbar extensions on button/input/switch after CLI install"
```

---

## Task 8: Verify remaining Group B components compile cleanly

For each of these: `select.tsx`, `dialog.tsx`, `dropdown-menu.tsx`, `context-menu.tsx`, `textarea.tsx`, `label.tsx`, `resizable.tsx`, `sidebar.tsx`, `sonner.tsx` — confirm that TypeScript compiles and tests pass. The CLI-installed versions should be compatible, but if any Crowbar call site imports a prop that the CLI version dropped, add it back as an accepted-but-ignored parameter.

**Files:**
- Verify: `web/src/components/ui/{select,dialog,dropdown-menu,context-menu,textarea,label,resizable,sidebar,sonner}.tsx`

- [ ] **Step 8.1 — TypeScript check**

```bash
cd web && npm run typecheck 2>&1 | grep -v "node_modules" | head -50
```

Expected: zero errors. If TypeScript reports errors in `components/ui/` files about missing props, read the specific file and add the missing prop as an accepted-but-ignored parameter following the same pattern as button.tsx (destructure with `_` prefix, don't use).

- [ ] **Step 8.2 — Run full test suite**

```bash
cd web && npm test -- --run
```

Expected: all tests pass.

- [ ] **Step 8.3 — Commit any compat fixes**

```bash
git add web/src/components/ui/
git commit -m "feat: fix Crowbar call-site compat on remaining Group B components"
```

---

## Task 9: Phase 3 — App sweep for hardcoded colors

Remove all raw hex / oklch / hsl values from files outside the two theme files.

**Files:**
- Sweep: `web/src/**/*.{tsx,ts,css}` (excluding theme files and out-of-scope editor CSS)

- [ ] **Step 9.1 — Run the audit grep**

```bash
grep -rn 'oklch\|#[0-9a-fA-F]\{3,8\}\b\|hsl(' web/src \
  --include='*.tsx' --include='*.ts' --include='*.css' \
  | grep -v 'styles/theme.css\|styles/editor-theme.css\|token-theme.css\|monaco-editor.css\|overlay-card.css\|terminal.css\|completion-dropdown.css\|hover-tooltip.css\|markdown/styles.css\|node_modules'
```

- [ ] **Step 9.2 — Fix each match**

For each file+line in the output, open the file and replace the hardcoded value with the appropriate CSS variable. Common mappings:

| Raw value | Replace with |
|---|---|
| Any shade of `oklch(... 165 ...)` (teal/green) | `var(--primary)` or `var(--accent)` |
| White `#fff` / `oklch(1 0 0)` | `var(--background)` or `var(--foreground)` depending on use |
| Muted grey in dark contexts | `var(--muted-foreground)` |
| Border-like transparencies | `var(--border)` |

When in doubt about the right token: look at what the element is (background, text, border) and match it to the Cross/UI token vocabulary.

- [ ] **Step 9.3 — Re-run audit to confirm zero matches**

```bash
grep -rn 'oklch\|#[0-9a-fA-F]\{3,8\}\b\|hsl(' web/src \
  --include='*.tsx' --include='*.ts' --include='*.css' \
  | grep -v 'styles/theme.css\|styles/editor-theme.css\|token-theme.css\|monaco-editor.css\|overlay-card.css\|terminal.css\|completion-dropdown.css\|hover-tooltip.css\|markdown/styles.css\|node_modules'
```

Expected: no output.

- [ ] **Step 9.4 — Run full tests**

```bash
cd web && npm test -- --run
```

Expected: all tests pass.

- [ ] **Step 9.5 — Commit**

```bash
git add web/src/
git commit -m "fix: replace all hardcoded colors with CSS variable references (Phase 3 sweep)"
```

---

## Self-review notes

- **Spec coverage:** Phase 1 (theme split) ✓ Tasks 1–3. Phase 2 (component migration) ✓ Tasks 4–8. Phase 3 (sweep) ✓ Task 9. Group C (keep as-is) handled in Step 4.2. Font files already at `web/public/fonts/` ✓.
- **No placeholders:** All code blocks are complete. All commands are exact.
- **Type consistency:** `Tab`, `TabProps`, `TabsItem` in Task 5 match current `tab-bar-item.tsx` and `terminal-tab-bar-item.tsx` consumers. `TooltipCompound` default export in Task 6 matches current import pattern in feature components.
- **Scope check:** Out-of-scope editor CSS files are excluded from the Phase 3 grep in Step 9.1. `--editor-font-family` lives in `editor-theme.css`, never `theme.css`. One-way dependency enforced.
