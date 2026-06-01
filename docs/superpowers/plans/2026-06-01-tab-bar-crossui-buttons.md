# Tab Bar CrossUI Button Consistency Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace ad-hoc button implementations in the tab bar with consistent CrossUI primitives, and activate the long-dead `tooltip`/`shortcut` props on `Button` so tooltips work across the entire app.

**Architecture:** The `Button` component in `components/ui/button.tsx` has stub `tooltip`/`shortcut`/`tooltipSide` props that are silently ignored. We implement them using `TooltipPrimitive` from `@radix-ui/react-tooltip` (already a dep) with `asChild` so no extra DOM node is added. Tab bar components are then cleaned up: manual size classes replaced with `size="icon-xs"`, dead props removed, and the raw `DropdownMenuTrigger` for the `+` button replaced with the `render={<Button/>}` pattern.

**Tech Stack:** React, `@radix-ui/react-tooltip`, `@base-ui/react/use-render`, Vitest + Testing Library, Tailwind CSS

---

### Task 1: Activate `tooltip`/`shortcut`/`tooltipSide` in `Button`

**Files:**
- Modify: `web/src/components/ui/button.tsx`
- Modify: `web/src/__tests__/components/ui/button.test.tsx`

#### Context

`button.tsx` uses `@base-ui/react/use-render` to render the button element. The `tooltip` prop is currently accepted and discarded. `@radix-ui/react-tooltip` is already installed (used by `tooltip.tsx`). The `Keybinding` component at `@/components/ui/keybinding` renders keyboard shortcut strings as styled badges.

The existing test `'accepts and ignores Crowbar compat props without error'` will need to be updated — it should now assert the tooltip is rendered, not just that no error is thrown.

#### Steps

- [ ] **Update the test file first**

Replace the existing "accepts and ignores Crowbar compat props" test and add tooltip-specific tests in `web/src/__tests__/components/ui/button.test.tsx`.

Add this import at the top of the file (after the existing imports):
```tsx
import { TooltipProvider } from '@/components/ui/tooltip'
```

Replace the existing test `'accepts and ignores Crowbar compat props without error'` with:
```tsx
it('renders tooltip content when tooltip prop is provided', async () => {
  const user = userEvent.setup()
  render(
    <TooltipProvider>
      <Button tooltip="Go Back">Click me</Button>
    </TooltipProvider>
  )
  await user.hover(screen.getByRole('button', { name: 'Click me' }))
  expect(await screen.findByText('Go Back')).toBeInTheDocument()
})

it('renders no tooltip when tooltip prop is absent', () => {
  render(<Button>Click me</Button>)
  expect(screen.getByRole('button', { name: 'Click me' })).toBeInTheDocument()
  // No tooltip wrapper rendered
  expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
})

it('still accepts compact and commandId props without error (compat)', () => {
  expect(() =>
    render(<Button compact commandId="some.command">label</Button>)
  ).not.toThrow()
})
```

Also add this import at the top:
```tsx
import userEvent from '@testing-library/user-event'
```

- [ ] **Run the new tests to confirm they fail**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx vitest run src/__tests__/components/ui/button.test.tsx
```

Expected: the tooltip test fails because `Button` doesn't render a tooltip yet.

- [ ] **Implement tooltip rendering in `button.tsx`**

Full replacement for `web/src/components/ui/button.tsx`:

```tsx
"use client";

import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import { mergeProps } from "@base-ui/react/merge-props";
import { useRender } from "@base-ui/react/use-render";
import { cva, type VariantProps } from "class-variance-authority";
import type * as React from "react";
import { cn } from "@/lib/utils";
import Keybinding from "@/components/ui/keybinding";
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
      },
    },
  },
);

const tooltipContentClass =
  "ui-text-sm pointer-events-none z-[99999] whitespace-nowrap rounded-lg border border-border/70 bg-card/95 px-2.5 py-1.5 text-foreground shadow-lg backdrop-blur-sm animate-in fade-in-0 zoom-in-95 data-[state=closed]:animate-out data-[state=closed]:fade-out-0 data-[state=closed]:zoom-out-95 data-[side=bottom]:slide-in-from-top-1 data-[side=left]:slide-in-from-right-1 data-[side=right]:slide-in-from-left-1 data-[side=top]:slide-in-from-bottom-1";

export interface ButtonProps extends useRender.ComponentProps<"button"> {
  variant?: VariantProps<typeof buttonVariants>["variant"];
  size?: VariantProps<typeof buttonVariants>["size"];
  loading?: boolean;
  /** Active state — adds bg-accent/20 highlight when true */
  active?: boolean;
  /** Compact mode (compat, no visual effect) */
  compact?: boolean;
  /** Tooltip text — renders a real tooltip on hover */
  tooltip?: string;
  /** Keyboard shortcut shown in the tooltip */
  shortcut?: string;
  /** Tooltip side preference */
  tooltipSide?: "top" | "right" | "bottom" | "left";
  /** Command ID for keybinding hints (compat, not rendered) */
  commandId?: string;
}

export function Button({
  className,
  variant,
  size,
  render,
  children,
  loading = false,
  disabled: disabledProp,
  active,
  compact: _compact,
  tooltip,
  shortcut,
  tooltipSide = "top",
  commandId: _commandId,
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
    className: cn(buttonVariants({ className, size, variant }), active && "bg-accent/20"),
    "aria-disabled": loading || undefined,
    "data-loading": loading ? "" : undefined,
    "data-slot": "button",
    disabled: isDisabled,
    type: typeValue,
  };

  const buttonEl = useRender({
    defaultTagName: "button",
    props: mergeProps<"button">(defaultProps, props),
    render,
  });

  if (!tooltip) return buttonEl;

  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger asChild>{buttonEl}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content
          side={tooltipSide}
          sideOffset={6}
          collisionPadding={8}
          className={cn(tooltipContentClass, shortcut && "flex items-center gap-2")}
        >
          {tooltip}
          {shortcut && <Keybinding binding={shortcut} />}
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}
```

- [ ] **Run the tests to confirm they pass**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx vitest run src/__tests__/components/ui/button.test.tsx
```

Expected: all tests pass, including the new tooltip tests.

- [ ] **Commit**

```bash
git add web/src/components/ui/button.tsx web/src/__tests__/components/ui/button.test.tsx
git commit -m "feat(button): activate tooltip/shortcut props using TooltipPrimitive"
```

---

### Task 2: Clean up tab navigation buttons

**Files:**
- Modify: `web/src/features/tabs/components/tab-navigation-buttons.tsx`

#### Context

The sidebar toggle, back, and forward buttons all use `Button` with:
- Manual `className="h-6 w-6 shrink-0 rounded-full p-0 text-muted-foreground"` instead of a size variant
- Dead `compact` and `commandId` props
- `tooltip` and `tooltipSide` that now work after Task 1

Replace manual sizing with `size="icon-xs"` (gives `size-7 sm:size-6`, i.e. 28px/24px — matches the existing 24px `h-6 w-6`). Remove `rounded-full` (icon-xs uses `rounded-md` per the design decision). Keep `text-muted-foreground` and `shrink-0` in `className`.

- [ ] **Replace the file content**

Full replacement for `web/src/features/tabs/components/tab-navigation-buttons.tsx`:

```tsx
import {
  ArrowLeft,
  ArrowRight,
  SidebarSimple as PanelLeftClose,
} from "@phosphor-icons/react";
import React from "react";
import { Button } from "@/components/ui/button";
import { cn } from "@/utils/cn";

interface TabNavigationButtonsProps {
  isBottomPane: boolean;
  sidebarOpen: boolean;
  sidebarPosition: "left" | "right";
  isAtLeftEdge: boolean;
  canGoBack: boolean;
  canGoForward: boolean;
  onToggleSidebar: () => void;
  onJumpBack: () => void;
  onJumpForward: () => void;
}

const TabNavigationButtons = React.memo(function TabNavigationButtons({
  isBottomPane,
  sidebarOpen,
  sidebarPosition,
  canGoBack,
  canGoForward,
  onToggleSidebar,
  onJumpBack,
  onJumpForward,
}: TabNavigationButtonsProps) {
  return (
    <>
      {!isBottomPane && (
        <Button
          onClick={onToggleSidebar}
          variant="ghost"
          size="icon-xs"
          className={cn(
            "shrink-0 text-muted-foreground",
            sidebarPosition === "right" && "scale-x-[-1]",
          )}
          tooltip={sidebarOpen ? "Hide Sidebar" : "Show Sidebar"}
          tooltipSide="bottom"
          aria-label={sidebarOpen ? "Hide sidebar" : "Show sidebar"}
        >
          <PanelLeftClose size={14} />
        </Button>
      )}

      <div className="flex shrink-0 items-center gap-0.5">
        <Button
          onClick={onJumpBack}
          disabled={!canGoBack}
          variant="ghost"
          size="icon-xs"
          className="shrink-0 text-muted-foreground"
          tooltip="Go Back"
          tooltipSide="bottom"
          aria-label="Go back to previous location"
        >
          <ArrowLeft />
        </Button>
        <Button
          onClick={onJumpForward}
          disabled={!canGoForward}
          variant="ghost"
          size="icon-xs"
          className="shrink-0 text-muted-foreground"
          tooltip="Go Forward"
          tooltipSide="bottom"
          aria-label="Go forward to next location"
        >
          <ArrowRight />
        </Button>
      </div>
    </>
  );
});

export default TabNavigationButtons;
```

- [ ] **Run the full test suite to confirm nothing broke**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx vitest run
```

Expected: all tests pass.

- [ ] **Commit**

```bash
git add web/src/features/tabs/components/tab-navigation-buttons.tsx
git commit -m "refactor(tabs): use Button size=icon-xs and remove dead props in nav buttons"
```

---

### Task 3: Fix the new tab `+` button

**Files:**
- Modify: `web/src/features/tabs/components/tab-new-button.tsx`

#### Context

The `+` button currently uses a raw `<DropdownMenuTrigger>` with hand-rolled hover classes (`hover:bg-muted hover:text-foreground focus:outline-none`). This is inconsistent with every other button — `Button variant="ghost"` uses `hover:bg-accent`. The fix is the `render={<Button/>}` pattern that `markdown-chat-toolbar.tsx` already uses for its Send/Stop buttons.

The `DropdownMenu` component from `@/components/ui/dropdown-menu` supports a `render` prop on `DropdownMenuTrigger` that accepts a React element — the trigger renders as that element, merging accessibility props via Base UI's render pattern.

The close-split `Button` below it already uses `Button` correctly; just drop `compact` from it.

- [ ] **Replace the file content**

Full replacement for `web/src/features/tabs/components/tab-new-button.tsx`:

```tsx
import {
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
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";

interface TabNewButtonProps {
  paneId: string;
  isBottomPane: boolean;
  disablePaneActions: boolean;
  isInSplit: boolean;
  onNewTerminal: () => void;
  onOpenUrl: () => void;
  onClosePane: () => void;
}

const TabNewButton = React.memo(function TabNewButton({
  isBottomPane,
  disablePaneActions,
  isInSplit,
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
        <DropdownMenuContent side="bottom" align="start" className="min-w-[140px]">
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

- [ ] **Run the full test suite**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx vitest run
```

Expected: all tests pass.

- [ ] **Commit**

```bash
git add web/src/features/tabs/components/tab-new-button.tsx
git commit -m "refactor(tabs): replace raw DropdownMenuTrigger with Button render pattern for + button"
```

---

### Task 4: Clean up tab bar item close/pin button

**Files:**
- Modify: `web/src/features/tabs/components/tab-bar-item.tsx`

#### Context

The close/pin button inside each tab item is intentionally tiny (16px × 16px, positioned absolutely). `size="icon-xs"` gives 28px/24px which is too large here — keep the manual `h-4 w-4` sizing. The changes are:
- Drop `compact` (no-op)
- `tooltip` and `shortcut` now work via Task 1 — `shortcut="mod+w"` will render as `⌘W` in the tooltip automatically via `Keybinding`
- Remove the now-redundant manual `hover:bg-accent hover:text-foreground` from `className` — `variant="ghost"` already provides these

- [ ] **Update the close/pin Button in `tab-bar-item.tsx`**

Find the `Button` at line ~161 in `web/src/features/tabs/components/tab-bar-item.tsx` and replace it:

Old:
```tsx
      <Button
        type="button"
        compact
        variant="ghost"
        data-no-dnd
        onClick={(e) => {
          e.stopPropagation();
          if (buffer.isPinned) {
            handleTabPin(buffer.id);
          } else {
            handleTabClose(buffer.id);
          }
        }}
        className={cn(
          "absolute inset-y-0 my-auto right-1.5 h-4 w-4 grid place-items-center cursor-pointer select-none rounded-full p-0 text-muted-foreground transition-opacity hover:bg-accent hover:text-foreground",
          buffer.isPinned || isActive ? "opacity-60" : "opacity-0 group-hover/tab:opacity-60",
        )}
        tooltip={buffer.isPinned ? "Unpin tab" : "Close"}
        shortcut={buffer.isPinned ? undefined : "mod+w"}
        tabIndex={-1}
        draggable={false}
      >
```

New:
```tsx
      <Button
        variant="ghost"
        data-no-dnd
        onClick={(e) => {
          e.stopPropagation();
          if (buffer.isPinned) {
            handleTabPin(buffer.id);
          } else {
            handleTabClose(buffer.id);
          }
        }}
        className={cn(
          "absolute inset-y-0 my-auto right-1.5 h-4 w-4 grid place-items-center cursor-pointer select-none rounded-full p-0 text-muted-foreground transition-opacity",
          buffer.isPinned || isActive ? "opacity-60" : "opacity-0 group-hover/tab:opacity-60",
        )}
        tooltip={buffer.isPinned ? "Unpin tab" : "Close"}
        tooltipSide="bottom"
        shortcut={buffer.isPinned ? undefined : "mod+w"}
        tabIndex={-1}
        draggable={false}
      >
```

- [ ] **Run the full test suite**

```bash
cd /Users/char2cs/.superconductor/worktrees/crowbar/sc-flux-phonon-3588/web && npx vitest run
```

Expected: all tests pass.

- [ ] **Commit**

```bash
git add web/src/features/tabs/components/tab-bar-item.tsx
git commit -m "refactor(tabs): clean up close/pin button — drop dead props, tooltip now works"
```
