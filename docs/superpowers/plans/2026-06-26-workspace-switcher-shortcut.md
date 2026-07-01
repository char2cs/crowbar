# Workspace Switcher Keyboard Shortcut Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `Cmd/Ctrl+.` as a globally-bound, live-editable keyboard shortcut that opens the workspace switcher dialog.

**Architecture:** Register a new `navigation.openWorkspaceSwitcher` command in the keymap registry; create a focused keyboard hook that reads its effective chord and fires a callback; call that hook from `ContextPill` to open the dialog. The Settings > Keybindings UI gains a "Navigation" section automatically because it groups by category from the registry.

**Tech Stack:** React, Zustand, Vitest + @testing-library/react, TypeScript

## Global Constraints

- All test files in `web/src/__tests__/` mirroring `web/src/` structure; use `@/` imports inside tests.
- Component files use kebab-case filenames; exported React components use PascalCase.
- Never `useXxxStore()` with no selector — always use a narrow selector.
- `mod` in chord strings means Cmd on Mac, Ctrl on Windows/Linux (handled by `eventMatchesChord`).

---

### Task 1: Extend the keymap registry with the new command

**Files:**
- Modify: `web/src/features/keymaps/types.ts`
- Modify: `web/src/features/keymaps/registry.ts`

**Interfaces:**
- Produces: `OPEN_WORKSPACE_SWITCHER = 'navigation.openWorkspaceSwitcher'` (exported string constant), available to the keyboard hook in Task 2 and settings in Task 4.
- Produces: `CommandCategory` now includes `'Navigation'`.

- [ ] **Step 1: Add `'Navigation'` to `CommandCategory`**

Open `web/src/features/keymaps/types.ts`. Change line 10:

```ts
export type CommandCategory = 'Panes' | 'Tabs' | 'Editor' | 'Navigation'
```

- [ ] **Step 2: Add the command constant and registry entry**

Open `web/src/features/keymaps/registry.ts`. After the existing `EDITOR_SAVE_ALL` constant line, add:

```ts
export const OPEN_WORKSPACE_SWITCHER = 'navigation.openWorkspaceSwitcher'
```

Then add a new entry at the end of the `COMMANDS` array (inside the `[...]`, after the `EDITOR_SAVE_ALL` entry):

```ts
  // --- Navigation (live-editable) ---
  {
    id: OPEN_WORKSPACE_SWITCHER,
    label: 'Open workspace switcher',
    category: 'Navigation',
    defaultChord: 'mod+.',
    liveEditable: true,
  },
```

- [ ] **Step 3: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

Expected: no errors (or only pre-existing errors unrelated to these files).

- [ ] **Step 4: Commit**

```bash
git add web/src/features/keymaps/types.ts web/src/features/keymaps/registry.ts
git commit -m "feat(keymaps): add navigation.openWorkspaceSwitcher command (mod+.)"
```

---

### Task 2: Create `useWorkspaceSwitcherKeyboard` hook

**Files:**
- Create: `web/src/features/keymaps/hooks/use-workspace-switcher-keyboard.ts`
- Create: `web/src/__tests__/features/keymaps/hooks/use-workspace-switcher-keyboard.test.ts`

**Interfaces:**
- Consumes: `OPEN_WORKSPACE_SWITCHER` from `@/features/keymaps/registry` (Task 1), `useEffectiveChordMap` from `@/features/keymaps/hooks/use-effective-keymap`, `eventMatchesChord` from `@/features/keymaps/utils/chord`.
- Produces: `useWorkspaceSwitcherKeyboard(onOpen: () => void): void` — called in `ContextPill` (Task 3).

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/features/keymaps/hooks/use-workspace-switcher-keyboard.test.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useWorkspaceSwitcherKeyboard } from '@/features/keymaps/hooks/use-workspace-switcher-keyboard'

vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  IS_MAC: false,
}))

function dispatchKeydown(init: KeyboardEventInit): KeyboardEvent {
  const event = new KeyboardEvent('keydown', { cancelable: true, bubbles: true, ...init })
  window.dispatchEvent(event)
  return event
}

describe('useWorkspaceSwitcherKeyboard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls onOpen and prevents default on Ctrl+.', () => {
    const onOpen = vi.fn()
    renderHook(() => useWorkspaceSwitcherKeyboard(onOpen))

    const event = dispatchKeydown({ key: '.', ctrlKey: true })

    expect(onOpen).toHaveBeenCalledTimes(1)
    expect(event.defaultPrevented).toBe(true)
  })

  it('does not call onOpen on plain dot', () => {
    const onOpen = vi.fn()
    renderHook(() => useWorkspaceSwitcherKeyboard(onOpen))

    dispatchKeydown({ key: '.' })

    expect(onOpen).not.toHaveBeenCalled()
  })

  it('does not call onOpen on a different mod chord', () => {
    const onOpen = vi.fn()
    renderHook(() => useWorkspaceSwitcherKeyboard(onOpen))

    dispatchKeydown({ key: ',', ctrlKey: true })

    expect(onOpen).not.toHaveBeenCalled()
  })

  it('removes the listener on unmount', () => {
    const onOpen = vi.fn()
    const { unmount } = renderHook(() => useWorkspaceSwitcherKeyboard(onOpen))
    unmount()

    dispatchKeydown({ key: '.', ctrlKey: true })

    expect(onOpen).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd web && npx vitest run src/__tests__/features/keymaps/hooks/use-workspace-switcher-keyboard.test.ts 2>&1 | tail -20
```

Expected: FAIL — "Cannot find module '@/features/keymaps/hooks/use-workspace-switcher-keyboard'"

- [ ] **Step 3: Create the hook**

Create `web/src/features/keymaps/hooks/use-workspace-switcher-keyboard.ts`:

```ts
import { useEffect } from 'react'
import { useEffectiveChordMap } from '@/features/keymaps/hooks/use-effective-keymap'
import { eventMatchesChord } from '@/features/keymaps/utils/chord'
import { OPEN_WORKSPACE_SWITCHER } from '@/features/keymaps/registry'

export function useWorkspaceSwitcherKeyboard(onOpen: () => void): void {
  const chordMap = useEffectiveChordMap()

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const chord = chordMap[OPEN_WORKSPACE_SWITCHER]
      if (!chord || !eventMatchesChord(e, chord)) return
      e.preventDefault()
      onOpen()
    }

    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [chordMap, onOpen])
}
```

- [ ] **Step 4: Run the tests and verify they pass**

```bash
cd web && npx vitest run src/__tests__/features/keymaps/hooks/use-workspace-switcher-keyboard.test.ts 2>&1 | tail -20
```

Expected: all 4 tests pass.

- [ ] **Step 5: Commit**

```bash
git add web/src/features/keymaps/hooks/use-workspace-switcher-keyboard.ts web/src/__tests__/features/keymaps/hooks/use-workspace-switcher-keyboard.test.ts
git commit -m "feat(keymaps): useWorkspaceSwitcherKeyboard hook"
```

---

### Task 3: Wire the hook into `ContextPill`

**Files:**
- Modify: `web/src/components/layout/context-pill.tsx`
- Modify: `web/src/__tests__/components/layout/context-pill.test.tsx`

**Interfaces:**
- Consumes: `useWorkspaceSwitcherKeyboard` from `@/features/keymaps/hooks/use-workspace-switcher-keyboard` (Task 2).

- [ ] **Step 1: Write the failing test**

Open `web/src/__tests__/components/layout/context-pill.test.tsx`. Add this test inside the existing `describe('ContextPill', ...)` block (after the last existing `it(...)` case):

```ts
  it('opens the workspace switcher on Ctrl+. keydown', async () => {
    mockPathname = '/ide/p1/r1/ws1'
    render(<ContextPill />)

    fireEvent.keyDown(window, { key: '.', ctrlKey: true })

    expect(await screen.findByPlaceholderText('Switch workspace…')).toBeInTheDocument()
  })
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd web && npx vitest run src/__tests__/components/layout/context-pill.test.tsx 2>&1 | tail -20
```

Expected: the new test fails — the dialog does not open.

- [ ] **Step 3: Wire the hook into `ContextPill`**

Open `web/src/components/layout/context-pill.tsx`. Add the import after the existing imports:

```ts
import { useWorkspaceSwitcherKeyboard } from '@/features/keymaps/hooks/use-workspace-switcher-keyboard'
```

Then inside the `ContextPill` function body, right after `const [open, setOpen] = useState(false)`, add:

```ts
  useWorkspaceSwitcherKeyboard(() => setOpen(true))
```

- [ ] **Step 4: Run the tests and verify they all pass**

```bash
cd web && npx vitest run src/__tests__/components/layout/context-pill.test.tsx 2>&1 | tail -20
```

Expected: all 5 tests pass (4 existing + 1 new).

- [ ] **Step 5: Commit**

```bash
git add web/src/components/layout/context-pill.tsx web/src/__tests__/components/layout/context-pill.test.tsx
git commit -m "feat(context-pill): open workspace switcher on mod+. shortcut"
```

---

### Task 4: Expose the Navigation category in Settings > Keybindings

**Files:**
- Modify: `web/src/features/settings/components/tabs/keybindings-settings.tsx`

**Interfaces:**
- Consumes: `OPEN_WORKSPACE_SWITCHER` command registered in Task 1 — no code import needed; it flows through `COMMANDS` automatically.

- [ ] **Step 1: Add `'Navigation'` to `CATEGORY_ORDER`**

Open `web/src/features/settings/components/tabs/keybindings-settings.tsx`. Change line 29:

```ts
const CATEGORY_ORDER: CommandCategory[] = ['Navigation', 'Panes', 'Tabs', 'Editor']
```

(Navigation first so the workspace shortcut sits at the top of the list.)

- [ ] **Step 2: Verify TypeScript compiles**

```bash
cd web && npx tsc --noEmit 2>&1 | head -30
```

Expected: no new errors.

- [ ] **Step 3: Run the full test suite**

```bash
cd web && npx vitest run 2>&1 | tail -30
```

Expected: all tests pass.

- [ ] **Step 4: Commit**

```bash
git add web/src/features/settings/components/tabs/keybindings-settings.tsx
git commit -m "feat(settings): show Navigation keybindings section"
```
