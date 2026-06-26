# Workspace Switcher Keyboard Shortcut

**Date:** 2026-06-26
**Branch:** hardening/production-readiness

## Goal

Add `Cmd+.` / `Ctrl+.` as a global keyboard shortcut to open the workspace switcher dialog (the context pill dropdown). The shortcut must be live-editable through Settings > Keybindings.

## Design

### 1. Registry — new command

Add to `features/keymaps/types.ts`:
- Extend `CommandCategory` union with `'Navigation'`

Add to `features/keymaps/registry.ts`:
- Export constant `OPEN_WORKSPACE_SWITCHER = 'navigation.openWorkspaceSwitcher'`
- Add entry to `COMMANDS`:
  ```ts
  { id: OPEN_WORKSPACE_SWITCHER, label: 'Open workspace switcher', category: 'Navigation', defaultChord: 'mod+.', liveEditable: true }
  ```

### 2. Keyboard hook — new file

Create `features/keymaps/hooks/use-workspace-switcher-keyboard.ts`:
- `useWorkspaceSwitcherKeyboard(onOpen: () => void): void`
- Reads the effective chord for `OPEN_WORKSPACE_SWITCHER` via `useEffectiveKeybinding()`
- Adds a global `keydown` listener (same pattern as `usePaneKeyboard`)
- Calls `onOpen()` when `eventMatchesChord(e, chord)` is true
- Cleans up on unmount / chord change

### 3. ContextPill — wire it in

In `components/layout/context-pill.tsx`:
- Call `useWorkspaceSwitcherKeyboard(() => setOpen(true))` inside the component body

### 4. Settings UI — expose the category

In `features/settings/components/tabs/keybindings-settings.tsx`:
- Add `'Navigation'` to `CATEGORY_ORDER` (before or after `'Panes'`)
- The command then appears automatically as its own section in Settings > Keybindings, with the same live rebind / conflict-detection / reset UX as all other commands

## Files changed

| File | Change |
|------|--------|
| `features/keymaps/types.ts` | Add `'Navigation'` to `CommandCategory` |
| `features/keymaps/registry.ts` | Add `OPEN_WORKSPACE_SWITCHER` constant + `COMMANDS` entry |
| `features/keymaps/hooks/use-workspace-switcher-keyboard.ts` | New hook |
| `components/layout/context-pill.tsx` | Call `useWorkspaceSwitcherKeyboard` |
| `features/settings/components/tabs/keybindings-settings.tsx` | Add `'Navigation'` to `CATEGORY_ORDER` |

## Behaviour

- Default chord: `mod+.` (Cmd+. on Mac, Ctrl+. on Windows/Linux)
- Pressing it anywhere in the app opens the workspace switcher dialog
- If the dialog is already open, the keydown fires `setOpen(true)` again — a no-op since it's already open
- The shortcut participates in conflict detection: if the user binds something else to `mod+.`, a warning badge appears on both commands
- Preset system: the `compact` preset does not override this command, so it keeps the default chord in both presets
