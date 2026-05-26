# Settings Panel Cleanup — Design Spec

**Date:** 2026-05-25  
**Branch:** feature/agentic-ide-gordon  
**Status:** Approved

---

## Overview

The settings panel was carried over from Athas with minimal changes. Large portions are non-functional, use Athas-specific UI components instead of Shadcn/ui, and expose settings that don't apply to Crowbar's architecture. This spec covers the full cleanup: removing dead settings, fixing broken ones, replacing Athas-specific components with Shadcn/ui equivalents, porting the icon theme system, and fixing modal shell bugs.

---

## 1. Tabs — Structure Changes

### Remove entirely
- **General tab** (`general-settings.tsx`) — app version display, update checks, bug reporting, settings import/export. All Athas-specific infrastructure, not applicable to Crowbar.
- **Keybindings tab** (`keyboard-settings.tsx`) — keybinding customization is out of scope for now.

### Keep (no structural change)
- Editor, Files, Git, Terminal tabs remain in the sidebar.

### Final tab order
1. Appearance
2. Editor
3. Files
4. Git
5. Terminal

---

## 2. Appearance Tab

### 2a. Theming — Replace dual picker + toggle with two dropdowns

**Current behavior (remove):**
- A "Sync with System Theme" toggle
- When toggle OFF → single "Color Theme" dropdown
- When toggle ON → "Preferred Light Theme" + "Preferred Dark Theme" dropdowns

**New behavior:**
- **Color Theme** — single dropdown listing all themes registered in `themeRegistry`, not pre-filtered by light/dark
- **Theme Mode** — dropdown with three options: `Light`, `Dark`, `Sync with System`

**Settings keys:**
- `colorTheme: string` — theme ID (e.g. `"crowbar-dark"`)
- `themeMode: "light" | "dark" | "system"` — new field replacing `syncSystemTheme` + dual theme fields

**Application logic (settings-effects.ts):**
- When `themeMode === "system"`: use `window.matchMedia("(prefers-color-scheme: dark)")` to pick the actual applied mode; respond to media query changes
- When `themeMode === "light"` or `"dark"`: force that mode regardless of system
- Remove all logic for `syncSystemTheme`, `preferredLightTheme`, `preferredDarkTheme`

**Default values:**
- `colorTheme`: `"crowbar-dark"`
- `themeMode`: `"system"`

### 2b. Icon Themes — Full port from Athas

Crowbar's icon theme registry (`web/src/extensions/icon-themes/`) is currently a stub. Port the full implementation from Athas.

**Files to port / rewrite:**

| Source (Athas) | Destination (Crowbar) | Action |
|---|---|---|
| `icon-theme-registry.ts` | `web/src/extensions/icon-themes/icon-theme-registry.ts` | Replace stub with full implementation |
| `types.ts` | `web/src/extensions/icon-themes/types.ts` | Update to match full Athas type defs |
| `builtin/classic-theme.tsx` | `web/src/extensions/icon-themes/builtin/classic-theme.tsx` | Copy, adjust imports |
| `builtin/material-theme.tsx` | `web/src/extensions/icon-themes/builtin/material-theme.tsx` | Copy, adjust imports |
| `builtin/colorful-material-theme.tsx` | `web/src/extensions/icon-themes/builtin/colorful-material-theme.tsx` | Copy, adjust imports |
| `builtin/compact-theme.tsx` | `web/src/extensions/icon-themes/builtin/compact-theme.tsx` | Copy, adjust imports |
| `builtin/minimal-theme.tsx` | `web/src/extensions/icon-themes/builtin/minimal-theme.tsx` | Copy, adjust imports |
| `builtin/none-theme.tsx` | `web/src/extensions/icon-themes/builtin/none-theme.tsx` | Copy, adjust imports |
| `icon-theme-initializer.ts` | `web/src/extensions/icon-themes/icon-theme-initializer.ts` | Copy, adjust imports |

**Dependencies to add:**
- `@phosphor-icons/react` — if not already present (used by Classic, Compact, Minimal, None themes)
- `material-file-icons` — required by Material and Colorful Material themes

**Bootstrap integration:**
- Call `initializeIconThemes()` from the app bootstrap sequence (wherever `initializeThemes()` and similar calls happen)

**File explorer wiring:**
- Crowbar's file tree icon component must read the currently selected icon theme from `iconThemeStore` and call `theme.getFileIcon()` to render icons
- If the current theme or registry returns null/undefined, fall back to the Classic theme

**Settings UI:**
- Icon Theme dropdown must read from `iconThemeRegistry.getAllThemes()` and display `theme.name`
- Selection updates `iconThemeStore` immediately; no save/apply button needed

### 2c. UI Font Family — Replace with Shadcn/ui Select

The current font family picker uses an Athas-specific component. Replace with a Shadcn/ui `<Select>` component fed from `fontStore` (system fonts list). Behavior and setting key stay the same; only the UI component changes.

### 2d. Layout section — Remove dead settings

**Remove:**
- Sidebar Tabs Position — not applicable to Crowbar sidebar structure
- Title Bar Project Mode — Athas-specific
- Window Transparency — Athas-specific (Electron feature)
- Open Projects in New Window — Athas-specific

**Keep (and wire up):**
- Sidebar Position (Left / Right) — currently not applied to the Crowbar sidebar. Wire it up: update sidebar layout state when this setting changes.

---

## 3. Editor Tab

### Remove
- **Editor Engine** selector — Crowbar supports Monaco only. Remove the dropdown and the `editorEngine` setting key from the UI entirely (can keep in types for future, but hide from settings panel).

### Fix — Apply settings to Monaco instance
Currently, editor settings are stored in the settings store but not applied to the Monaco editor. Each setting must be propagated:

| Setting | Monaco API |
|---|---|
| Font Family | `editor.updateOptions({ fontFamily })` |
| Font Size | `editor.updateOptions({ fontSize })` |
| Line Height | `editor.updateOptions({ lineHeight })` |
| Tab Size | `editor.getModel()?.updateOptions({ tabSize })` |
| Word Wrap | `editor.updateOptions({ wordWrap: enabled ? "on" : "off" })` |
| Line Numbers | `editor.updateOptions({ lineNumbers: enabled ? "on" : "off" })` |
| Render Whitespace | `editor.updateOptions({ renderWhitespace })` |
| Indent Guides | `editor.updateOptions({ guides: { indentation: enabled } })` |
| Minimap | `editor.updateOptions({ minimap: { enabled } })` |
| Relative Line Numbers | `editor.updateOptions({ lineNumbers: enabled ? "relative" : "on" })` |
| Auto Completion | `editor.updateOptions({ quickSuggestions: enabled })` |
| Parameter Hints | `editor.updateOptions({ parameterHints: { enabled } })` |

**Implementation approach:** A settings effect (in `settings-effects.ts` or a dedicated `editor-effects.ts`) subscribes to relevant settings keys and calls `updateOptions` on all active Monaco editor instances. The editor registry (or wherever editor instances are stored) must be accessible from this effect.

---

## 4. Files Tab

Keep as-is. No changes to settings definitions or UI. Functional verification is out of scope for this cleanup.

---

## 5. Git Tab

Keep as-is. No changes. Functional verification is out of scope.

---

## 6. Terminal Tab

Keep as-is. No changes. Functional verification is out of scope.

---

## 7. Modal Shell — Bug Fixes

### Phantom scrollbar
- The settings dialog has an extra scrollbar appearing from a double-scroll container. Audit `settings-dialog.tsx` and the content wrapper: ensure only one element has `overflow-y: auto/scroll`. The outer dialog should be fixed-height with no overflow; the content pane should scroll.

### Weird scaling
- Audit the dialog size classes (`h-[74vh] max-h-[820px] w-[90vw] max-w-[1120px]`). Identify what causes the scaling issue — likely a `transform: scale()` or an `em`-based root font size being inherited. Fix to render at 1:1 scale.

### Shadcn/ui component audit
- For every custom Athas UI component used inside the settings panel, check if a Shadcn/ui equivalent exists:
  - Dropdowns/selects → `<Select>` from Shadcn
  - Toggles → `<Switch>` from Shadcn
  - Number inputs → `<Input type="number">` from Shadcn
  - Text inputs → `<Input>` from Shadcn
- Replace where drop-in equivalent exists. For complex custom components with no Shadcn counterpart, leave as-is.

---

## 8. Settings Types & Store Cleanup

- Remove `syncSystemTheme`, `preferredLightTheme`, `preferredDarkTheme` fields from settings type — replaced by `themeMode`
- Remove `editorEngine` from the settings UI (keep type definition, just don't render the control)
- Remove all settings belonging to deleted tabs (General, Keybindings) from `search-index.ts` so they don't appear in settings search
- Remove deleted settings from `default-settings.ts` default values where safe (or mark as deprecated)

---

## Non-Goals

- Making Git, Files, or Terminal settings functional — that's a separate effort
- Extension-contributed icon themes — not in scope; just the 6 built-ins
- Custom theme upload — keep or remove as-is, not part of this cleanup
- Command palette theme switcher — nice-to-have, out of scope

---

## Success Criteria

1. Settings panel opens with 5 tabs: Appearance, Editor, Files, Git, Terminal
2. Appearance tab shows: Color Theme (single dropdown), Theme Mode (Light/Dark/Sync), Icon Theme (all 6 options visible and applying to the file tree), UI Font Family (Shadcn Select), Sidebar Position (wired up)
3. Editor settings apply to the Monaco instance in real time
4. No phantom scrollbar in settings modal
5. No scaling anomalies in settings modal
6. No Athas-specific dropdown/toggle components visible in the settings panel
