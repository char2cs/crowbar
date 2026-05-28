# @coss/ui Adaptation Design

**Date:** 2026-05-28  
**Branch:** enhancement/design-language  
**Status:** Approved

## Goal

Adopt `@coss/ui` as Crowbar's UI component foundation — bringing in its updated visual design and component internals — without breaking any existing Crowbar prop APIs or call sites.

## Context

- `@base-ui/react` v1.5.0 is already installed and already exposes `useRender` and `mergeProps`, the APIs `@coss/ui` components use internally. No version upgrade needed.
- Running `shadcn add @coss/ui` directly is destructive — it overwrites Crowbar's customized components without merging. The correct approach is manual adaptation.
- Several `@coss/ui` components are already present in the repo as untracked files (accordion, alert, drawer, sheet, etc.) from a prior install attempt.

## Approach

Install `@coss/ui` into a **temp directory** to read source cleanly, then adapt each conflicting component by hand.

## Components

### Conflicting — adapt, don't replace

| File | What @coss/ui changes | What Crowbar grafts back |
|---|---|---|
| `button.tsx` | `useRender` + `mergeProps` internals; new variant/size classes; adds `loading` prop | `tooltip`, `compact`, `active`, `commandId` props (pass-through or no-ops) |
| `tabs.tsx` | Standard shadcn exports only | Re-add `Tab` standalone component used across editor tab bars |
| `switch.tsx` | Named export only | Add `export default Switch` alias |
| `input.tsx` | Named export only | Add `export default Input` alias |
| `tooltip.tsx` | Named export only | Add `export default Tooltip` alias |

### New — install as-is

Already present as untracked files. No adaptation needed:
- `accordion.tsx`, `alert.tsx`, `alert-dialog.tsx`, `autocomplete.tsx`
- `breadcrumb.tsx`, `calendar.tsx`, `checkbox-group.tsx`, `combobox.tsx`
- `drawer.tsx`, `empty.tsx`, `field.tsx`, `fieldset.tsx`, `form.tsx`
- `frame.tsx`, `group.tsx`, `kbd.tsx`, `menu.tsx`, `meter.tsx`
- `number-field.tsx`, `otp-field.tsx`, `pagination.tsx`, `popover.tsx`
- `preview-card.tsx`, `radio-group.tsx`, `sheet.tsx`, `table.tsx`
- `toolbar.tsx`

### index.css

Add missing color tokens needed by new components. Skip keyframe animations — `tw-animate-css` already covers them.

Tokens to add to `@theme inline`:
```css
--color-success: var(--success);
--color-success-foreground: var(--success-foreground);
--color-warning-foreground: var(--warning-foreground);
--color-info-foreground: var(--info-foreground);
--color-destructive-foreground: var(--destructive-foreground);
```

Color values to add to `:root` and `.dark`:
- `--success`, `--success-foreground`, `--warning-foreground`, `--info-foreground`, `--destructive-foreground`

## Constraints

- All existing call sites continue to work without changes — no `grep`-and-replace campaigns on consumers.
- The `Tab` standalone component must remain exported from `tabs.tsx` — it is used in `tab-bar-item.tsx` and `terminal-tab-bar-item.tsx`.
- Default export aliases on `switch`, `input`, `tooltip` are temporary shims; a follow-up pass can migrate call sites to named imports.

## Testing

Run the full Vitest suite after each component adaptation. Fix any assertion failures caused by changed class names before moving to the next component.
