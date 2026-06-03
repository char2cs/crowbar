# @coss/ui Adaptation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adapt `@coss/ui` components into Crowbar by replacing `button.tsx` with the `@coss/ui` version (grafting Crowbar prop extensions back on) and adding the missing CSS color tokens that new `@coss/ui` components depend on.

**Architecture:** Replace `button.tsx` wholesale with the `@coss/ui` internals (`useRender` + `mergeProps` from `@base-ui/react`) while preserving all Crowbar-specific props as accepted-but-silent parameters. `tabs.tsx`, `switch.tsx`, `input.tsx`, and `tooltip.tsx` are already well-adapted to base-ui and are **out of scope** — their current Crowbar-specific customisations are better than the @coss/ui defaults. `index.css` gains five new color theme aliases + their `:root`/`.dark` values so the untracked `@coss/ui` new-component library (accordion, alert, drawer, etc.) can render correctly.

**Tech Stack:** React 18, Tailwind CSS v4, `@base-ui/react` v1.5.0 (`useRender`, `mergeProps`), `class-variance-authority`, Vitest + React Testing Library.

---

## Scope notes

| File | Decision | Reason |
|---|---|---|
| `button.tsx` | **Replace** | Core visual uplift; Crowbar props are already no-ops |
| `tabs.tsx` | **Skip** | Callers use Radix `forceMount`/`hidden`; base-ui migration is a separate project |
| `switch.tsx` | **Skip** | Current version has better Crowbar customisations (size variants, `onChange`) |
| `input.tsx` | **Skip** | Current version has better Crowbar extensions (leftIcon, variant compat) |
| `tooltip.tsx` | **Skip** | Uses compound `<Tooltip content="..." />` API incompatible with base-ui tooltip |
| `index.css` | **Update** | Add five missing color tokens for new @coss/ui components |

---

## Files

| Action | Path |
|---|---|
| Modify | `web/src/components/ui/button.tsx` |
| Modify | `web/src/index.css` |
| Create | `web/src/__tests__/components/ui/button.test.tsx` |

---

## Task 1: Replace button.tsx with adapted @coss/ui version

**Files:**
- Modify: `web/src/components/ui/button.tsx`
- Create: `web/src/__tests__/components/ui/button.test.tsx`

### Step 1.1 — Write the failing tests

Create `web/src/__tests__/components/ui/button.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { describe, it, expect } from 'vitest'
import { Button } from '@/components/ui/button'

describe('Button', () => {
  it('renders a button element by default', () => {
    render(<Button>Click me</Button>)
    expect(screen.getByRole('button', { name: 'Click me' })).toBeInTheDocument()
  })

  it('applies ghost variant classes', () => {
    const { container } = render(<Button variant="ghost">Ghost</Button>)
    const btn = container.firstChild as HTMLElement
    expect(btn.className).toContain('hover:bg-accent')
  })

  it('accepts and ignores Crowbar compat props without error', () => {
    expect(() =>
      render(
        <Button
          tooltip="hint"
          compact
          active
          shortcut="mod+k"
          tooltipSide="bottom"
          commandId="some.command"
        >
          label
        </Button>
      )
    ).not.toThrow()
  })

  it('applies bg-accent/20 when active is true', () => {
    const { container } = render(<Button active>Active</Button>)
    const btn = container.firstChild as HTMLElement
    expect(btn.className).toContain('bg-accent/20')
  })

  it('shows spinner and sets data-loading when loading', () => {
    const { container } = render(<Button loading>Saving</Button>)
    const btn = container.firstChild as HTMLElement
    expect(btn).toHaveAttribute('data-loading', '')
    expect(container.querySelector('[data-slot="button-loading-indicator"]')).toBeInTheDocument()
  })

  it('is disabled when loading', () => {
    render(<Button loading>Saving</Button>)
    expect(screen.getByRole('button')).toBeDisabled()
  })

  it('renders accent variant without error (Crowbar alias → default style)', () => {
    expect(() => render(<Button variant="accent">Accent</Button>)).not.toThrow()
  })

  it('renders muted variant without error (Crowbar alias → ghost style)', () => {
    expect(() => render(<Button variant="muted">Muted</Button>)).not.toThrow()
  })

  it('renders danger variant without error (Crowbar alias → destructive style)', () => {
    expect(() => render(<Button variant="danger">Danger</Button>)).not.toThrow()
  })
})
```

### Step 1.2 — Run tests to confirm they fail

```bash
cd web && npx vitest run src/__tests__/components/ui/button.test.tsx
```

Expected: several failures — `active`, `loading`, `accent`/`muted`/`danger` variants not present yet.

### Step 1.3 — Replace button.tsx

Replace the entire contents of `web/src/components/ui/button.tsx` with:

```tsx
"use client";

import { mergeProps } from "@base-ui/react/merge-props";
import { useRender } from "@base-ui/react/use-render";
import { cva, type VariantProps } from "class-variance-authority";
import type * as React from "react";
import { cn } from "@/lib/utils";
import { Spinner } from "@/components/ui/spinner";

export const buttonVariants = cva(
  "relative inline-flex shrink-0 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-lg border font-medium text-base outline-none transition-shadow before:pointer-events-none before:absolute before:inset-0 before:rounded-[calc(var(--radius-lg)-1px)] pointer-coarse:after:absolute pointer-coarse:after:size-full pointer-coarse:after:min-h-11 pointer-coarse:after:min-w-11 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-64 data-loading:select-none data-loading:text-transparent sm:text-sm [&_svg:not([class*='opacity-'])]:opacity-80 [&_svg:not([class*='size-'])]:size-4.5 sm:[&_svg:not([class*='size-'])]:size-4 [&_svg]:pointer-events-none [&_svg]:-mx-0.5 [&_svg]:shrink-0",
  {
    defaultVariants: {
      size: "default",
      variant: "default",
    },
    variants: {
      size: {
        default: "h-9 px-[calc(--spacing(3)-1px)] sm:h-8",
        icon: "size-9 sm:size-8",
        "icon-lg": "size-10 sm:size-9",
        "icon-sm": "size-8 sm:size-7",
        "icon-xl":
          "size-11 sm:size-10 [&_svg:not([class*='size-'])]:size-5 sm:[&_svg:not([class*='size-'])]:size-4.5",
        "icon-xs":
          "size-7 rounded-md before:rounded-[calc(var(--radius-md)-1px)] sm:size-6 not-in-data-[slot=input-group]:[&_svg:not([class*='size-'])]:size-4 sm:not-in-data-[slot=input-group]:[&_svg:not([class*='size-'])]:size-3.5",
        lg: "h-10 px-[calc(--spacing(3.5)-1px)] sm:h-9",
        sm: "h-8 gap-1.5 px-[calc(--spacing(2.5)-1px)] sm:h-7",
        xl: "h-11 px-[calc(--spacing(4)-1px)] text-lg sm:h-10 sm:text-base [&_svg:not([class*='size-'])]:size-5 sm:[&_svg:not([class*='size-'])]:size-4.5",
        xs: "h-7 gap-1 rounded-md px-[calc(--spacing(2)-1px)] text-sm before:rounded-[calc(var(--radius-md)-1px)] sm:h-6 sm:text-xs [&_svg:not([class*='size-'])]:size-4 sm:[&_svg:not([class*='size-'])]:size-3.5",
      },
      variant: {
        default:
          "not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)] border-primary bg-primary text-primary-foreground shadow-primary/24 shadow-xs hover:bg-primary/90 data-pressed:bg-primary/90 *:data-[slot=button-loading-indicator]:text-primary-foreground [:active,[data-pressed]]:inset-shadow-[0_1px_--theme(--color-black/8%)] [:disabled,:active,[data-pressed]]:shadow-none",
        destructive:
          "not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)] border-destructive bg-destructive text-white shadow-destructive/24 shadow-xs hover:bg-destructive/90 data-pressed:bg-destructive/90 *:data-[slot=button-loading-indicator]:text-white [:active,[data-pressed]]:inset-shadow-[0_1px_--theme(--color-black/8%)] [:disabled,:active,[data-pressed]]:shadow-none",
        "destructive-outline":
          "border-input bg-popover not-dark:bg-clip-padding text-destructive-foreground shadow-xs/5 not-disabled:not-active:not-data-pressed:before:shadow-[0_1px_--theme(--color-black/4%)] hover:border-destructive/32 hover:bg-destructive/4 data-pressed:border-destructive/32 data-pressed:bg-destructive/4 *:data-[slot=button-loading-indicator]:text-foreground dark:bg-input/32 dark:not-disabled:before:shadow-[0_-1px_--theme(--color-white/2%)] dark:not-disabled:not-active:not-data-pressed:before:shadow-[0_-1px_--theme(--color-white/6%)] [:disabled,:active,[data-pressed]]:shadow-none",
        ghost:
          "border-transparent text-foreground hover:bg-accent data-pressed:bg-accent *:data-[slot=button-loading-indicator]:text-foreground",
        link: "border-transparent text-foreground underline-offset-4 hover:underline data-pressed:underline *:data-[slot=button-loading-indicator]:text-foreground",
        outline:
          "border-input bg-popover not-dark:bg-clip-padding text-foreground shadow-xs/5 not-disabled:not-active:not-data-pressed:before:shadow-[0_1px_--theme(--color-black/4%)] hover:bg-accent/50 data-pressed:bg-accent/50 *:data-[slot=button-loading-indicator]:text-foreground dark:bg-input/32 dark:data-pressed:bg-input/64 dark:hover:bg-input/64 dark:not-disabled:before:shadow-[0_-1px_--theme(--color-white/2%)] dark:not-disabled:not-active:not-data-pressed:before:shadow-[0_-1px_--theme(--color-white/6%)] [:disabled,:active,[data-pressed]]:shadow-none",
        secondary:
          "border-transparent bg-secondary text-secondary-foreground hover:bg-secondary/90 data-pressed:bg-secondary/90 *:data-[slot=button-loading-indicator]:text-secondary-foreground [:active,[data-pressed]]:bg-secondary/80",
        // Crowbar aliases — map to nearest @coss/ui variant
        accent:
          "not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)] border-primary bg-primary text-primary-foreground shadow-primary/24 shadow-xs hover:bg-primary/90 data-pressed:bg-primary/90 [:active,[data-pressed]]:inset-shadow-[0_1px_--theme(--color-black/8%)] [:disabled,:active,[data-pressed]]:shadow-none",
        muted:
          "border-transparent text-foreground hover:bg-accent data-pressed:bg-accent",
        danger:
          "not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)] border-destructive bg-destructive text-white shadow-destructive/24 shadow-xs hover:bg-destructive/90 data-pressed:bg-destructive/90 [:active,[data-pressed]]:inset-shadow-[0_1px_--theme(--color-black/8%)] [:disabled,:active,[data-pressed]]:shadow-none",
      },
    },
  }
);

export interface ButtonProps extends useRender.ComponentProps<"button"> {
  variant?: VariantProps<typeof buttonVariants>["variant"];
  size?: VariantProps<typeof buttonVariants>["size"];
  loading?: boolean;
  /** Crowbar compat — accepted but silent (no tooltip infrastructure in @coss/ui button) */
  compact?: boolean;
  tooltip?: string;
  shortcut?: string;
  tooltipSide?: "top" | "right" | "bottom" | "left";
  commandId?: string;
  /** Adds bg-accent/20 highlight when true */
  active?: boolean;
}

export function Button({
  className,
  variant,
  size,
  render,
  children,
  loading = false,
  disabled: disabledProp,
  compact: _compact,
  tooltip: _tooltip,
  shortcut: _shortcut,
  tooltipSide: _tooltipSide,
  commandId: _commandId,
  active,
  ...props
}: ButtonProps): React.ReactElement {
  const isDisabled: boolean = Boolean(loading || disabledProp);
  const typeValue: React.ButtonHTMLAttributes<HTMLButtonElement>["type"] =
    render ? undefined : "button";

  const defaultProps = {
    children: (
      <>
        {children}
        {loading && (
          <Spinner
            className="pointer-events-none absolute"
            data-slot="button-loading-indicator"
          />
        )}
      </>
    ),
    className: cn(
      buttonVariants({ className, size, variant }),
      active && "bg-accent/20",
    ),
    "aria-disabled": loading || undefined,
    "data-loading": loading ? "" : undefined,
    "data-slot": "button",
    disabled: isDisabled,
    type: typeValue,
  };

  return useRender({
    defaultTagName: "button",
    props: mergeProps<"button">(defaultProps, props),
    render,
  });
}
```

### Step 1.4 — Run tests to confirm they pass

```bash
cd web && npx vitest run src/__tests__/components/ui/button.test.tsx
```

Expected: all 9 tests PASS.

### Step 1.5 — Run full test suite to check for regressions

```bash
cd web && npx vitest run
```

Expected: no new failures beyond any pre-existing ones. If tests fail due to changed class names in button (e.g. a test asserting `bg-primary` is still present — that class is still there), investigate and fix the assertion to match the new class shape.

### Step 1.6 — Commit

```bash
git add web/src/components/ui/button.tsx web/src/__tests__/components/ui/button.test.tsx
git commit -m "feat: adapt button.tsx to @coss/ui — new visual design, Crowbar props preserved"
```

---

## Task 2: Add missing color tokens to index.css

The new `@coss/ui` components (alert, meter, empty, etc.) use `text-success`, `bg-success`, `text-warning-foreground`, `text-destructive-foreground`, `text-info-foreground` Tailwind classes. Without theme aliases and actual color values, these classes silently produce no color output.

**Files:**
- Modify: `web/src/index.css`

### Step 2.1 — Add theme aliases to `@theme inline`

In `web/src/index.css`, append these five lines inside the `@theme inline { }` block, after the existing `--color-chrome-bg` line:

```css
    --color-success: var(--success);
    --color-success-foreground: var(--success-foreground);
    --color-warning-foreground: var(--warning-foreground);
    --color-info-foreground: var(--info-foreground);
    --color-destructive-foreground: var(--destructive-foreground);
```

The `@theme inline` block should end like this after the edit:

```css
    --color-warning: var(--warning);
    --color-info: var(--info);
    --font-editor: var(--editor-font-family);
    --color-chrome-bg: var(--chrome-bg);
    --color-success: var(--success);
    --color-success-foreground: var(--success-foreground);
    --color-warning-foreground: var(--warning-foreground);
    --color-info-foreground: var(--info-foreground);
    --color-destructive-foreground: var(--destructive-foreground);
}
```

### Step 2.2 — Add color values to `:root`

Append these lines to the `:root { }` block in `web/src/index.css`, after the existing `--chrome-bg` line:

```css
    --success: oklch(0.65 0.16 145);
    --success-foreground: oklch(0.98 0.02 145);
    --warning-foreground: oklch(0.98 0.02 85);
    --info-foreground: oklch(0.98 0.02 250);
    --destructive-foreground: oklch(0.98 0.02 27);
```

### Step 2.3 — Add dark overrides to `.dark`

Append these lines to the `.dark { }` block in `web/src/index.css`, after the existing `--chrome-bg` dark override:

```css
    --success: oklch(0.72 0.18 145);
    --success-foreground: oklch(0.15 0.03 145);
    --warning-foreground: oklch(0.15 0.03 85);
    --info-foreground: oklch(0.15 0.03 250);
    --destructive-foreground: oklch(0.15 0.03 27);
```

### Step 2.4 — Commit

```bash
git add web/src/index.css
git commit -m "feat: add success/warning/info/destructive-foreground color tokens for @coss/ui components"
```

---

## Task 3: Run full test suite and fix any remaining failures

**Files:**
- Any test files that fail

### Step 3.1 — Run the full suite

```bash
cd web && npx vitest run
```

### Step 3.2 — If the tab-bar-item test fails

The test at `web/src/__tests__/features/tabs/components/tab-bar-item.test.tsx` may fail if it asserts specific button class names that changed. Check the failure message.

If the test asserts `bg-primary/15` or similar on an active tab, the tab-bar-item uses the `Tab` component (not `Button`), so button changes should not affect it. If it somehow fails, read the test and update the class assertion to match the actual rendered output.

### Step 3.3 — Commit fixes if needed

```bash
git add <changed test files>
git commit -m "test: update assertions for @coss/ui button class changes"
```

---

## Self-review notes

- **Spec coverage:** button ✓, index.css ✓, tabs/switch/input/tooltip deferred with rationale ✓, new components (already untracked in repo, depend on color tokens added in Task 2) ✓
- **No placeholders:** all code blocks are complete
- **Type consistency:** `ButtonProps` extends `useRender.ComponentProps<"button">` throughout; Spinner import path matches existing project structure
