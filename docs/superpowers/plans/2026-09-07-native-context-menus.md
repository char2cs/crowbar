# Native Context Menus Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace every React-rendered right-click menu with the OS's real native menu (via Tauri's JS menu API), keeping the old rendered popup only as a `!isTauri()` dev-mode fallback.

**Architecture:** Add a `showNativeContextMenu(items, position)` helper in `web/src/components/ui/context-menu.tsx` built on `@tauri-apps/api/menu`, and make the existing `ContextMenu` export branch on `isTauri()` between it and the existing `ImperativeContextMenu` fallback. Because every live call site already renders `<ContextMenu items={...} position={...} isOpen={...} onClose={...} />` through that one name, this is a single-file swap — none of the 4 flat-item call sites (tabs, agent-chats tree, workspace/sidebar tree, file explorer) need to change. The Plate editor's block menu is migrated onto the same shared API (it currently talks to Radix directly). Dead code (the unused declarative `ContextMenuRoot`/... compound export) is deleted.

**Tech Stack:** React 19, `@tauri-apps/api` v2.11 (`menu`, `dpi` modules), `@base-ui/react` (Menu primitives, used for the fallback popup), Plate (`platejs`, `@platejs/selection`), Vitest + Testing Library.

**Spec:** `docs/superpowers/specs/2026-09-07-native-context-menus-design.md`

## Global Constraints

- No new Rust/Tauri command — menu construction happens entirely in JS via `@tauri-apps/api/menu`.
- Native menus carry NO per-item icons (only label + accelerator). Icons are dropped, not ported.
- The `!isTauri()` fallback (plain-browser `bun run dev`) keeps rendering today's in-app popup — it is not deleted, only demoted to a fallback path.
- `className` and `closeOnClick` on `ContextMenuItem` only affect the fallback rendering; the native path ignores them (no native styling hook, and native menus always close on activation).
- Every `Menu` built for the native path must be closed (`menu.close()`) after `popup()` resolves or rejects, to avoid leaking the underlying Rust-side resource handle on every right-click.
- Run `bun test`/`bun vitest run <path>` (not `bunx vitest`) and `bun tsc` (not `bunx tsc`) — `bunx` resolves a different package. `bun` is at `/Users/char2cs/.bun/bin/bun` if not on `PATH`.

---

### Task 1: Native menu mapping (`showNativeContextMenu`) + Tauri capability permission

**Files:**
- Modify: `desktop/src-tauri/capabilities/default.json`
- Modify: `web/src/components/ui/context-menu.tsx` (add imports + `toNativeMenuEntries`/`showNativeContextMenu`, additive only — nothing existing changes yet)
- Test: `web/src/__tests__/components/ui/context-menu-native.test.ts` (new)

**Interfaces:**
- Consumes: `ContextMenuItem` (existing interface, `web/src/components/ui/context-menu.tsx`) — fields used: `id`, `label`, `onClick`, `separator?`, `disabled?`, `shortcut?`, `items?`.
- Produces: `export async function showNativeContextMenu(items: ContextMenuItem[], position: { x: number; y: number }): Promise<void>` — Task 2 calls this directly.

- [ ] **Step 1: Add the `core:menu:default` permission**

Edit `desktop/src-tauri/capabilities/default.json` — add `"core:menu:default"` to the `permissions` array (after `"dialog:allow-open"`):

```json
{
  "$schema": "../gen/schemas/desktop-schema.json",
  "identifier": "default",
  "description": "Default app capabilities. `windows: [*]` because every Crowbar window is the same trusted bundle over the same crowbar:// protocol, differing only in which workspace it routes to — scoping to `main` denied the second window titlebar dragging, terminal creation, external links and dialogs. This capability is `local` only, so a remote-origin window would still get nothing from it; but adding a genuinely LOWER-trust local window means splitting capabilities, not narrowing this one, because it carries shell:allow-execute for the sidecar.",
  "windows": ["*"],
  "permissions": [
    "core:default",
    {
      "identifier": "shell:allow-execute",
      "allow": [{"name": "crowbar-api", "sidecar": true, "args": true}]
    },
    "shell:allow-spawn",
    "shell:allow-kill",
    "shell:allow-open",
    "log:default",
    "core:window:allow-start-dragging",
    "dialog:allow-open",
    "core:menu:default"
  ]
}
```

- [ ] **Step 2: Verify the JSON is still valid**

Run: `python3 -c "import json; json.load(open('desktop/src-tauri/capabilities/default.json')); print('ok')"`
Expected: `ok`

- [ ] **Step 3: Write the failing test for the native mapping**

Create `web/src/__tests__/components/ui/context-menu-native.test.ts`:

```ts
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ContextMenuItem } from '@/components/ui/context-menu'

const popupMock = vi.fn().mockResolvedValue(undefined)
const closeMock = vi.fn().mockResolvedValue(undefined)
const menuNewMock = vi.fn().mockResolvedValue({ popup: popupMock, close: closeMock })

vi.mock('@tauri-apps/api/menu', () => ({
  Menu: { new: (...args: unknown[]) => menuNewMock(...args) },
}))

describe('showNativeContextMenu', () => {
  beforeEach(() => {
    popupMock.mockClear()
    closeMock.mockClear()
    menuNewMock.mockClear()
    menuNewMock.mockResolvedValue({ popup: popupMock, close: closeMock })
  })

  afterEach(() => {
    vi.resetModules()
  })

  it('maps a flat item list to MenuItemOptions and pops up at the given position', async () => {
    const { showNativeContextMenu } = await import('@/components/ui/context-menu')
    const onClick = vi.fn()
    const items: ContextMenuItem[] = [
      { id: 'rename', label: 'Rename', onClick, shortcut: 'CmdOrCtrl+R' },
      { id: 'sep', label: '', separator: true, onClick: () => {} },
      { id: 'delete', label: 'Delete', onClick: vi.fn(), disabled: true },
    ]

    await showNativeContextMenu(items, { x: 120, y: 80 })

    expect(menuNewMock).toHaveBeenCalledTimes(1)
    const [{ items: nativeItems }] = menuNewMock.mock.calls[0] as [{ items: unknown[] }]
    expect(nativeItems).toEqual([
      { id: 'rename', text: 'Rename', enabled: true, accelerator: 'CmdOrCtrl+R', action: expect.any(Function) },
      { item: 'Separator' },
      { id: 'delete', text: 'Delete', enabled: false, accelerator: undefined, action: expect.any(Function) },
    ])

    const [renameEntry] = nativeItems as Array<{ action: (id: string) => void }>
    renameEntry.action('rename')
    expect(onClick).toHaveBeenCalledOnce()

    expect(popupMock).toHaveBeenCalledOnce()
    const [positionArg] = popupMock.mock.calls[0]
    expect(positionArg).toMatchObject({ x: 120, y: 80 })
  })

  it('maps nested items to a Submenu entry', async () => {
    const { showNativeContextMenu } = await import('@/components/ui/context-menu')
    const items: ContextMenuItem[] = [
      {
        id: 'turn-into',
        label: 'Turn into',
        onClick: () => {},
        items: [{ id: 'turn-into-h1', label: 'Heading 1', onClick: vi.fn() }],
      },
    ]

    await showNativeContextMenu(items, { x: 0, y: 0 })

    const [{ items: nativeItems }] = menuNewMock.mock.calls[0] as [{ items: unknown[] }]
    expect(nativeItems).toEqual([
      {
        text: 'Turn into',
        enabled: true,
        items: [
          { id: 'turn-into-h1', text: 'Heading 1', enabled: true, accelerator: undefined, action: expect.any(Function) },
        ],
      },
    ])
  })

  it('closes the menu after popup resolves', async () => {
    const { showNativeContextMenu } = await import('@/components/ui/context-menu')

    await showNativeContextMenu([{ id: 'a', label: 'A', onClick: vi.fn() }], { x: 0, y: 0 })

    expect(closeMock).toHaveBeenCalledOnce()
  })

  it('still closes the menu when popup rejects, and rethrows', async () => {
    popupMock.mockRejectedValueOnce(new Error('popup failed'))
    const { showNativeContextMenu } = await import('@/components/ui/context-menu')

    await expect(
      showNativeContextMenu([{ id: 'a', label: 'A', onClick: vi.fn() }], { x: 0, y: 0 }),
    ).rejects.toThrow('popup failed')
    expect(closeMock).toHaveBeenCalledOnce()
  })
})
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `/Users/char2cs/.bun/bin/bun vitest run src/__tests__/components/ui/context-menu-native.test.ts` (from `web/`)
Expected: FAIL — `showNativeContextMenu` is not exported from `@/components/ui/context-menu`.

- [ ] **Step 5: Implement the mapping + helper**

In `web/src/components/ui/context-menu.tsx`, add these imports at the top (alongside the existing ones):

```ts
import { Menu } from '@tauri-apps/api/menu'
import type { MenuItemOptions, SubmenuOptions, PredefinedMenuItemOptions } from '@tauri-apps/api/menu'
import { LogicalPosition } from '@tauri-apps/api/dpi'
```

Add this after the `ContextMenuItem`/`ContextMenuRootProps` interfaces (after line 34, before `function ImperativeContextMenu`):

```ts
// ── Native menu (Tauri) ──────────────────────────────────────────────────────

type NativeMenuEntry = MenuItemOptions | SubmenuOptions | PredefinedMenuItemOptions

function toNativeMenuEntries(items: ContextMenuItem[]): NativeMenuEntry[] {
  return items.map((item): NativeMenuEntry => {
    if (item.separator) {
      return { item: 'Separator' }
    }
    if (item.items && item.items.length > 0) {
      return {
        text: item.label,
        enabled: !item.disabled,
        items: toNativeMenuEntries(item.items),
      }
    }
    return {
      id: item.id,
      text: item.label,
      enabled: !item.disabled,
      accelerator: item.shortcut,
      action: () => item.onClick(),
    }
  })
}

/** Pops up the OS's own context menu. Always closes the underlying native
 * resource handle when the popup dismisses, whether an item was picked or
 * the popup was closed with no selection. */
export async function showNativeContextMenu(
  items: ContextMenuItem[],
  position: { x: number; y: number },
): Promise<void> {
  const menu = await Menu.new({ items: toNativeMenuEntries(items) })
  try {
    await menu.popup(new LogicalPosition(position.x, position.y))
  } finally {
    await menu.close()
  }
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `/Users/char2cs/.bun/bin/bun vitest run src/__tests__/components/ui/context-menu-native.test.ts` (from `web/`)
Expected: PASS (4 tests)

- [ ] **Step 7: Commit**

```bash
git add desktop/src-tauri/capabilities/default.json web/src/components/ui/context-menu.tsx web/src/__tests__/components/ui/context-menu-native.test.ts
git commit -m "$(cat <<'EOF'
feat(context-menu): add native menu mapping via @tauri-apps/api/menu

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `ContextMenuHost` — branch `ContextMenu` on `isTauri()`, delete the unused declarative API

**Files:**
- Modify: `web/src/components/ui/context-menu.tsx`
- Test: `web/src/__tests__/components/ui/context-menu-native-host.test.tsx` (new)
- Test (must stay green, unmodified): `web/src/__tests__/components/ui/context-menu.test.tsx`

**Interfaces:**
- Consumes: `showNativeContextMenu` (Task 1), `isTauri` from `@/lib/crowbar-bridge`.
- Produces: `ContextMenu` (same export name/props as today: `ContextMenuRootProps` — `isOpen`, `position`, `items`, `onClose`, `className?`, `footer?`) — every existing call site (`tab-context-menu.tsx`, `agent-chat-context-menu.tsx`, `row-context-menu.tsx`, `use-file-explorer-context-menu.tsx`) keeps importing `ContextMenu` from `@/components/ui/context-menu` with no changes.

- [ ] **Step 1: Write the failing test for the native host branch**

Create `web/src/__tests__/components/ui/context-menu-native-host.test.tsx`:

```tsx
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, waitFor } from '@testing-library/react'
import { ContextMenu } from '@/components/ui/context-menu'

const popupMock = vi.fn().mockResolvedValue(undefined)
const closeMock = vi.fn().mockResolvedValue(undefined)
const menuNewMock = vi.fn().mockResolvedValue({ popup: popupMock, close: closeMock })

vi.mock('@tauri-apps/api/menu', () => ({
  Menu: { new: (...args: unknown[]) => menuNewMock(...args) },
}))

type TauriWindow = Window & { __TAURI_INTERNALS__?: object }

beforeEach(() => {
  popupMock.mockClear()
  closeMock.mockClear()
  menuNewMock.mockClear()
  menuNewMock.mockResolvedValue({ popup: popupMock, close: closeMock })
  ;(window as TauriWindow).__TAURI_INTERNALS__ = {}
})

afterEach(() => {
  cleanup()
  delete (window as TauriWindow).__TAURI_INTERNALS__
  vi.restoreAllMocks()
})

describe('ContextMenu — native path (isTauri() true)', () => {
  it('renders nothing and pops the native menu with the right items and position', async () => {
    const onClose = vi.fn()
    const onClick = vi.fn()
    const { container } = render(
      <ContextMenu
        isOpen
        position={{ x: 42, y: 7 }}
        items={[{ id: 'a', label: 'A', onClick }]}
        onClose={onClose}
      />,
    )

    expect(container).toBeEmptyDOMElement()

    await waitFor(() => expect(menuNewMock).toHaveBeenCalledOnce())
    const [{ items: nativeItems }] = menuNewMock.mock.calls[0] as [{ items: Array<{ action: (id: string) => void }> }]
    expect(nativeItems[0].action).toBeInstanceOf(Function)
    nativeItems[0].action('a')
    expect(onClick).toHaveBeenCalledOnce()

    await waitFor(() => expect(onClose).toHaveBeenCalledOnce())
  })

  it('does nothing when isOpen is false', async () => {
    const onClose = vi.fn()
    render(
      <ContextMenu
        isOpen={false}
        position={{ x: 0, y: 0 }}
        items={[{ id: 'a', label: 'A', onClick: vi.fn() }]}
        onClose={onClose}
      />,
    )

    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(menuNewMock).not.toHaveBeenCalled()
    expect(onClose).not.toHaveBeenCalled()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `/Users/char2cs/.bun/bin/bun vitest run src/__tests__/components/ui/context-menu-native-host.test.tsx` (from `web/`)
Expected: FAIL — today `ContextMenu` always renders `ImperativeContextMenu`'s popup regardless of `isTauri()`, so `container` is not empty and `menuNewMock` is never called.

- [ ] **Step 3: Add the `isTauri` import and rewrite the `ContextMenu` export**

Add this import to `web/src/components/ui/context-menu.tsx` (alongside the existing ones):

```ts
import { isTauri } from '@/lib/crowbar-bridge'
```

Replace (around line 157-159):

```ts
// Re-export the imperative ContextMenu under the same name.
// The base-ui declarative version is exported as ContextMenuRoot for the Crowbar UI library.
export { ImperativeContextMenu as ContextMenu }
```

with:

```ts
// `ContextMenu` is native-first: in Tauri it pops the OS's own menu and
// renders nothing; outside Tauri (plain-browser `bun run dev`) it falls back
// to ImperativeContextMenu, the rendered popup this app used everywhere
// before native menus existed.
function ContextMenuHost({ isOpen, position, items, onClose, className, footer }: ContextMenuRootProps) {
  const onCloseRef = useRef(onClose)
  useEffect(() => {
    onCloseRef.current = onClose
  }, [onClose])

  useEffect(() => {
    if (!isTauri() || !isOpen) return
    let cancelled = false
    void (async () => {
      try {
        await showNativeContextMenu(items, position)
      } catch (error) {
        console.error('Failed to show native context menu:', error)
      } finally {
        if (!cancelled) onCloseRef.current()
      }
    })()
    return () => {
      cancelled = true
    }
    // items/position are read once, at the moment isOpen flips true — an
    // already-open native popup can't be updated mid-display anyway.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [isOpen])

  if (isTauri()) return null

  return (
    <ImperativeContextMenu
      isOpen={isOpen}
      position={position}
      items={items}
      onClose={onClose}
      className={className}
      footer={footer}
    />
  )
}

export { ContextMenuHost as ContextMenu }
```

- [ ] **Step 4: Run the new test to verify it passes**

Run: `/Users/char2cs/.bun/bin/bun vitest run src/__tests__/components/ui/context-menu-native-host.test.tsx` (from `web/`)
Expected: PASS (2 tests)

- [ ] **Step 5: Confirm the fallback test suite is still green**

Run: `/Users/char2cs/.bun/bin/bun vitest run src/__tests__/components/ui/context-menu.test.tsx` (from `web/`)
Expected: PASS (3 tests, unmodified) — jsdom has no `__TAURI_INTERNALS__` global by default, so these exercise the unchanged fallback path.

- [ ] **Step 6: Delete the unused declarative compound API**

In `web/src/components/ui/context-menu.tsx`, delete:
- The `import { ContextMenu as ContextMenuPrimitive } from '@base-ui/react/context-menu'` line.
- Everything from `function ContextMenuRoot(...)` through the end of the file (the whole declarative family: `ContextMenuRoot`, `ContextMenuPortal`, `ContextMenuTrigger`, `ContextMenuContent`, `ContextMenuGroup`, `ContextMenuLabel`, `ContextMenuItem` (the component — NOT the `ContextMenuItem` **interface**, which stays), `ContextMenuSub`, `ContextMenuSubTrigger`, `ContextMenuSubContent`, `ContextMenuCheckboxItem`, `ContextMenuRadioGroup`, `ContextMenuRadioItem`, `ContextMenuSeparator`, `ContextMenuShortcut`, and the trailing `export { ContextMenuRoot, ... }` block).
- The now-unused `CheckIcon` import from `lucide-react` (keep `ChevronRightIcon` — Task 3 needs it for the submenu fallback).

- [ ] **Step 7: Confirm nothing else imports the deleted names**

Run: `grep -rn "ContextMenuRoot\|ContextMenuTrigger\b\|ContextMenuContent\b\|ContextMenuGroup\b\|ContextMenuCheckboxItem\|ContextMenuRadioGroup\|ContextMenuRadioItem\|ContextMenuShortcut\b" web/src --include='*.tsx' --include='*.ts' | grep -v __tests__` (from repo root)
Expected: no output (the only prior hit was a comment in `block-context-menu.tsx`, rewritten in Task 4).

- [ ] **Step 8: Typecheck and run the full context-menu test file set**

Run (from `web/`): `/Users/char2cs/.bun/bin/bun tsc --noEmit`
Expected: no errors.

Run: `/Users/char2cs/.bun/bin/bun vitest run src/__tests__/components/ui/context-menu.test.tsx src/__tests__/components/ui/context-menu-native.test.ts src/__tests__/components/ui/context-menu-native-host.test.tsx`
Expected: PASS (9 tests total)

- [ ] **Step 9: Commit**

```bash
git add web/src/components/ui/context-menu.tsx web/src/__tests__/components/ui/context-menu-native-host.test.tsx
git commit -m "$(cat <<'EOF'
feat(context-menu): make ContextMenu native-first, drop unused declarative API

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: Submenu support in the fallback popup

**Why:** Task 4 migrates the Plate block menu's "Turn into" / "Align" submenus onto this shared `ContextMenu`. Today's `ImperativeContextMenu` fallback declares `ContextMenuItem.items` in its type but never renders it — right now nothing uses nested items, so that gap is invisible. It must actually work before Task 4 gives it real submenus, or the `!isTauri()` fallback silently drops "Turn into" and "Align" entirely.

**Files:**
- Modify: `web/src/components/ui/context-menu.tsx`
- Test: `web/src/__tests__/components/ui/context-menu.test.tsx` (extend)

**Interfaces:**
- Consumes: `ContextMenuItem.items` (existing field).
- Produces: no new exports — `ImperativeContextMenu` now honors nested `items` by rendering a submenu.

- [ ] **Step 1: Write the failing test**

Add to `web/src/__tests__/components/ui/context-menu.test.tsx`:

```tsx
describe('ContextMenu submenus', () => {
  it('opens a submenu on hover and fires the nested item onClick', async () => {
    const onNested = vi.fn()
    const { findByText } = render(
      <ContextMenu
        isOpen={true}
        position={{ x: 0, y: 0 }}
        items={[
          {
            id: 'turn-into',
            label: 'Turn into',
            onClick: () => {},
            items: [{ id: 'h1', label: 'Heading 1', onClick: onNested }],
          },
        ]}
        onClose={vi.fn()}
      />,
    )

    const trigger = await findByText('Turn into')
    fireEvent.pointerEnter(trigger)
    const nestedItem = await findByText('Heading 1')
    fireEvent.click(nestedItem)

    expect(onNested).toHaveBeenCalledOnce()
  })
})
```

Add `import { fireEvent } from '@testing-library/react'`'s `fireEvent` to the existing top-of-file import if not already present (it already imports `render, fireEvent` — reuse it).

- [ ] **Step 2: Run the test to verify it fails**

Run: `/Users/char2cs/.bun/bin/bun vitest run src/__tests__/components/ui/context-menu.test.tsx` (from `web/`)
Expected: FAIL — "Heading 1" never appears; `item.items` is not rendered today.

- [ ] **Step 3: Extract a recursive item renderer and add submenu rendering**

In `web/src/components/ui/context-menu.tsx`, replace the `{items.map((item) => ...)}` block inside `ImperativeContextMenu` (the JSX that currently maps `separator` vs. a plain `MenuPrimitive.Item`) with a call to a new recursive helper, and define that helper above `ImperativeContextMenu`:

```tsx
const menuItemClass =
  "flex min-h-8 cursor-default select-none items-center gap-2 rounded-sm px-2 py-1 text-base text-foreground outline-none data-disabled:pointer-events-none data-highlighted:bg-accent data-highlighted:text-accent-foreground data-disabled:opacity-64 sm:min-h-7 sm:text-sm [&>svg:not([class*='opacity-'])]:opacity-80 [&>svg:not([class*='size-'])]:size-4.5 sm:[&>svg:not([class*='size-'])]:size-4 [&>svg]:pointer-events-none [&>svg]:-mx-0.5 [&>svg]:shrink-0"

const popupClass =
  "relative flex not-[class*='w-']:min-w-[180px] origin-(--transform-origin) rounded-lg border bg-popover not-dark:bg-clip-padding shadow-lg/5 outline-none before:pointer-events-none before:absolute before:inset-0 before:rounded-[calc(var(--radius-lg)-1px)] before:shadow-[0_1px_--theme(--color-black/4%)] focus:outline-none dark:before:shadow-[0_-1px_--theme(--color-white/6%)]"

function renderMenuItems(items: ContextMenuItem[], onCloseRef: React.RefObject<() => void>) {
  return items.map((item) => {
    if (item.separator) {
      return <MenuPrimitive.Separator key={item.id} className="mx-2 my-1 h-px bg-border" />
    }
    if (item.items && item.items.length > 0) {
      return (
        <MenuPrimitive.SubmenuRoot key={item.id}>
          <MenuPrimitive.SubmenuTrigger disabled={item.disabled} className={cn(menuItemClass, item.className)}>
            <span className="flex-1">{item.label}</span>
            <ChevronRightIcon className="ml-auto size-4 opacity-60" />
          </MenuPrimitive.SubmenuTrigger>
          <MenuPrimitive.Portal>
            <MenuPrimitive.Positioner side="right" align="start" sideOffset={-4} className="z-[10040]">
              <MenuPrimitive.Popup className={popupClass}>
                <div className="max-h-(--available-height) w-full overflow-y-auto p-1">
                  {renderMenuItems(item.items, onCloseRef)}
                </div>
              </MenuPrimitive.Popup>
            </MenuPrimitive.Positioner>
          </MenuPrimitive.Portal>
        </MenuPrimitive.SubmenuRoot>
      )
    }
    return (
      <MenuPrimitive.Item
        key={item.id}
        disabled={item.disabled}
        className={cn(menuItemClass, item.className)}
        onClick={() => {
          item.onClick()
          if (item.closeOnClick !== false) onCloseRef.current()
        }}
      >
        {item.icon}
        <span className="flex-1">{item.label}</span>
        {item.shortcut && (
          <kbd className="ms-auto font-medium font-sans text-muted-foreground/72 text-xs tracking-widest">
            {item.shortcut}
          </kbd>
        )}
      </MenuPrimitive.Item>
    )
  })
}
```

Then, inside `ImperativeContextMenu`'s JSX, replace:

```tsx
            <div className="max-h-(--available-height) w-full overflow-y-auto p-1">
              {items.map((item) =>
                item.separator ? (
                  <MenuPrimitive.Separator key={item.id} className="mx-2 my-1 h-px bg-border" />
                ) : (
                  <MenuPrimitive.Item
                    key={item.id}
                    disabled={item.disabled}
                    className={cn(
                      "flex min-h-8 cursor-default select-none items-center gap-2 rounded-sm px-2 py-1 text-base text-foreground outline-none data-disabled:pointer-events-none data-highlighted:bg-accent data-highlighted:text-accent-foreground data-disabled:opacity-64 sm:min-h-7 sm:text-sm [&>svg:not([class*='opacity-'])]:opacity-80 [&>svg:not([class*='size-'])]:size-4.5 sm:[&>svg:not([class*='size-'])]:size-4 [&>svg]:pointer-events-none [&>svg]:-mx-0.5 [&>svg]:shrink-0",
                      item.className,
                    )}
                    onClick={() => {
                      item.onClick()
                      if (item.closeOnClick !== false) onCloseRef.current()
                    }}
                  >
                    {item.icon}
                    <span className="flex-1">{item.label}</span>
                    {item.shortcut && (
                      <kbd className="ms-auto font-medium font-sans text-muted-foreground/72 text-xs tracking-widest">
                        {item.shortcut}
                      </kbd>
                    )}
                  </MenuPrimitive.Item>
                ),
              )}
              {footer}
            </div>
```

with:

```tsx
            <div className="max-h-(--available-height) w-full overflow-y-auto p-1">
              {renderMenuItems(items, onCloseRef)}
              {footer}
            </div>
```

And update the `MenuPrimitive.Popup className` prop (just above that `<div>`) to reuse the extracted constant:

```tsx
          <MenuPrimitive.Popup className={cn(popupClass, className)}>
```

(replacing the inline `cn("relative flex ... dark:before:shadow-[0_-1px_--theme(--color-white/6%)]", className)` call with `cn(popupClass, className)`.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `/Users/char2cs/.bun/bin/bun vitest run src/__tests__/components/ui/context-menu.test.tsx` (from `web/`)
Expected: PASS (4 tests)

- [ ] **Step 5: Typecheck**

Run (from `web/`): `/Users/char2cs/.bun/bin/bun tsc --noEmit`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/ui/context-menu.tsx web/src/__tests__/components/ui/context-menu.test.tsx
git commit -m "$(cat <<'EOF'
feat(context-menu): render submenus in the fallback popup

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Migrate the Plate block-context-menu to the shared `ContextMenu` API

**Files:**
- Modify: `web/src/components/ui/block-context-menu.tsx`
- Test: `web/src/__tests__/components/ui/block-context-menu.test.tsx` (new)

**Interfaces:**
- Consumes: `ContextMenu`, `useContextMenu`, `ContextMenuItem` from `@/components/ui/context-menu` (Tasks 1-3).
- Produces: `BlockContextMenu` (same export, same `{ children }` prop) — `block-menu-kit.tsx` needs no changes.

- [ ] **Step 1: Write the failing test**

Create `web/src/__tests__/components/ui/block-context-menu.test.tsx`:

```tsx
import { afterEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen } from '@testing-library/react'
import { Plate, usePlateEditor } from 'platejs/react'
import { BlockSelectionPlugin } from '@platejs/selection/react'
import { BlockMenuKit } from '@/components/editor/plugins/block-menu-kit'

type TouchNavigator = Navigator & { maxTouchPoints: number }

afterEach(() => {
  cleanup()
  ;(navigator as TouchNavigator).maxTouchPoints = 0
})

function Harness() {
  const editor = usePlateEditor({
    plugins: BlockMenuKit,
    value: [
      { type: 'p', id: 'block-1', children: [{ text: 'first' }] },
      { type: 'p', id: 'block-2', children: [{ text: 'second' }] },
    ],
  })
  return (
    <Plate editor={editor}>
      <div data-slate-editor="false">content</div>
    </Plate>
  )
}

function selectFirstBlock() {
  const wrapper = document.querySelector('[data-slate-editor="false"]')!
  fireEvent.contextMenu(wrapper)
}

const menuItemLabels = () =>
  Array.from(document.querySelectorAll('[role="menuitem"]')).map((el) => el.textContent)

describe('BlockContextMenu', () => {
  it('shows Delete / Duplicate / Turn into / Indent / Outdent / Align on right-click', () => {
    render(<Harness />)

    selectFirstBlock()

    expect(menuItemLabels()).toEqual(['Delete', 'Duplicate', 'Turn into', 'Indent', 'Outdent', 'Align'])
  })

  it('renders no menu at all on a touch device', () => {
    ;(navigator as TouchNavigator).maxTouchPoints = 1
    render(<Harness />)
    fireEvent(window, new Event('resize'))

    selectFirstBlock()

    expect(menuItemLabels()).toEqual([])
    expect(screen.getByText('content')).toBeInTheDocument()
  })

  it('does not open when the click target is the slate editor itself', () => {
    render(<Harness />)
    const editorNode = document.createElement('div')
    editorNode.dataset.slateEditor = 'true'
    document.body.appendChild(editorNode)

    fireEvent.contextMenu(editorNode)

    expect(menuItemLabels()).toEqual([])
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `/Users/char2cs/.bun/bin/bun vitest run src/__tests__/components/ui/block-context-menu.test.tsx` (from `web/`)
Expected: FAIL — today's `BlockContextMenu` renders through `@radix-ui/react-context-menu` and doesn't expose `[role="menuitem"]` the same way once the shared `ContextMenu` is in play (or, if the labels happen to match, the touch-device assertion or the `data-slate-editor="true"` guard still exercises the pre-migration code path). Confirm the failure reflects the OLD implementation being exercised, not a typo in the test, before moving on.

- [ ] **Step 3: Rewrite `block-context-menu.tsx`**

Replace the full contents of `web/src/components/ui/block-context-menu.tsx` with:

```tsx
// Adapted from the Plate registry (`https://platejs.org/r/block-menu-kit.json`
// -> `block-context-menu`) — the per-block right-click menu.
//
// HAND-ADAPTED: this app's right-click menus are the OS's own native menu
// (see `@/components/ui/context-menu`'s `ContextMenu`/`useContextMenu`), not a
// React-rendered popup — a browser has to fake a menu; this app doesn't need
// to. Upstream renders through `@radix-ui/react-context-menu` directly; that
// import is gone here in favour of the shared `ContextMenu` API every other
// right-click menu in this app uses.
'use client'

import * as React from 'react'

import { BLOCK_CONTEXT_MENU_ID, BlockMenuPlugin, BlockSelectionPlugin } from '@platejs/selection/react'
import { KEYS } from 'platejs'
import { useEditorPlugin, useEditorReadOnly } from 'platejs/react'

import { useIsTouchDevice } from '@/hooks/use-is-touch-device'
import { setBlockType } from '@/components/editor/transforms'
import { ContextMenu, useContextMenu, type ContextMenuItem } from '@/components/ui/context-menu'

export function BlockContextMenu({ children }: { children: React.ReactNode }) {
  const { api, editor } = useEditorPlugin(BlockMenuPlugin)
  const isTouch = useIsTouchDevice()
  const readOnly = useEditorReadOnly()
  const menu = useContextMenu()
  const { openAt, close } = menu

  const handleTurnInto = React.useCallback(
    (type: string) => {
      editor
        .getApi(BlockSelectionPlugin)
        .blockSelection.getNodes()
        .forEach(([, path]) => {
          setBlockType(editor, type, { at: path })
        })
    },
    [editor],
  )

  const handleAlign = React.useCallback(
    (align: 'center' | 'left' | 'right') => {
      editor.getTransforms(BlockSelectionPlugin).blockSelection.setNodes({ align })
    },
    [editor],
  )

  // Closes both trackers together: this component's own popup state, and
  // Plate's `openId` — which `BlockSelectionPlugin` reads internally to decide
  // whether a block menu is currently open for a selected block.
  const handleClose = React.useCallback(() => {
    close()
    api.blockMenu.hide()
    editor.getApi(BlockSelectionPlugin).blockSelection.focus()
  }, [close, api, editor])

  if (isTouch) {
    return children
  }

  const items: ContextMenuItem[] = [
    {
      id: 'delete',
      label: 'Delete',
      onClick: () => {
        editor.getTransforms(BlockSelectionPlugin).blockSelection.removeNodes()
        editor.tf.focus()
      },
    },
    {
      id: 'duplicate',
      label: 'Duplicate',
      onClick: () => editor.getTransforms(BlockSelectionPlugin).blockSelection.duplicate(),
    },
    {
      id: 'turn-into',
      label: 'Turn into',
      onClick: () => {},
      items: [
        { id: 'turn-into-paragraph', label: 'Paragraph', onClick: () => handleTurnInto(KEYS.p) },
        { id: 'turn-into-h1', label: 'Heading 1', onClick: () => handleTurnInto(KEYS.h1) },
        { id: 'turn-into-h2', label: 'Heading 2', onClick: () => handleTurnInto(KEYS.h2) },
        { id: 'turn-into-h3', label: 'Heading 3', onClick: () => handleTurnInto(KEYS.h3) },
        { id: 'turn-into-blockquote', label: 'Blockquote', onClick: () => handleTurnInto(KEYS.blockquote) },
      ],
    },
    { id: 'sep-1', label: '', separator: true, onClick: () => {} },
    {
      id: 'indent',
      label: 'Indent',
      onClick: () => editor.getTransforms(BlockSelectionPlugin).blockSelection.setIndent(1),
    },
    {
      id: 'outdent',
      label: 'Outdent',
      onClick: () => editor.getTransforms(BlockSelectionPlugin).blockSelection.setIndent(-1),
    },
    {
      id: 'align',
      label: 'Align',
      onClick: () => {},
      items: [
        { id: 'align-left', label: 'Left', onClick: () => handleAlign('left') },
        { id: 'align-center', label: 'Center', onClick: () => handleAlign('center') },
        { id: 'align-right', label: 'Right', onClick: () => handleAlign('right') },
      ],
    },
  ]

  return (
    <div
      className="w-full"
      onContextMenu={(event) => {
        const dataset = (event.target as HTMLElement).dataset
        const disabled =
          dataset?.slateEditor === 'true' || readOnly || dataset?.plateOpenContextMenu === 'false'

        if (disabled) return event.preventDefault()

        event.preventDefault()
        const position = { x: event.clientX, y: event.clientY }
        setTimeout(() => {
          api.blockMenu.show(BLOCK_CONTEXT_MENU_ID, position)
          openAt(position)
        }, 0)
      }}
    >
      {children}
      <ContextMenu isOpen={menu.isOpen} position={menu.position} items={items} onClose={handleClose} />
    </div>
  )
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `/Users/char2cs/.bun/bin/bun vitest run src/__tests__/components/ui/block-context-menu.test.tsx` (from `web/`)
Expected: PASS (3 tests). If the harness's `Plate`/`usePlateEditor` setup needs adjustment (e.g. `BlockMenuKit`'s `BlockSelectionPlugin` requiring the editor to actually mount `<PlateContent>` before `getApi(BlockSelectionPlugin).blockSelection` is meaningful), fix the test harness — not `block-context-menu.tsx` — to match how `BlockSelectionPlugin` is actually driven elsewhere in this codebase (`src/components/editor/plugins/block-selection-kit.tsx`, `src/components/ui/block-selection.tsx`).

- [ ] **Step 5: Typecheck**

Run (from `web/`): `/Users/char2cs/.bun/bin/bun tsc --noEmit`
Expected: no errors.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/ui/block-context-menu.tsx web/src/__tests__/components/ui/block-context-menu.test.tsx
git commit -m "$(cat <<'EOF'
feat(editor): migrate the block context menu off @radix-ui/react-context-menu

Co-Authored-By: Claude Sonnet 5 <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Regression verification — unmodified surfaces, typecheck, full suite

**Files:** none modified (verification only — fixes here only if a regression is actually found).

**Interfaces:** none new.

- [ ] **Step 1: Run every test file for the 4 surfaces that received no code changes**

Run (from `web/`):
```bash
/Users/char2cs/.bun/bin/bun vitest run \
  src/__tests__/features/tabs \
  src/__tests__/features/agent/tree \
  src/__tests__/components/layout/row-context-menu.test.tsx \
  src/__tests__/components/layout/row-menu-model.test.ts \
  src/__tests__/features/agent/tree/lib/chat-menu-model.test.ts \
  src/__tests__/features/file-explorer/file-explorer/hooks/use-file-explorer-context-menu.test.tsx
```
Expected: PASS, same pass count as on `develop` before this branch — these files import `ContextMenu`/`useContextMenu` with an unchanged contract, so they should require zero edits. If any fail, the failure is a real regression in `ContextMenuHost` (Task 2) or the fallback renderer (Task 3) — fix there, not in these files.

- [ ] **Step 2: Grep for any remaining reference to a deleted export**

Run (from repo root): `grep -rn "ContextMenuRoot\|ContextMenuPortal\b\|ContextMenuTrigger\b\|ContextMenuContent\b\|ContextMenuGroup\b\|ContextMenuLabel\b\|ContextMenuSub\b\|ContextMenuSubTrigger\|ContextMenuSubContent\|ContextMenuCheckboxItem\|ContextMenuRadioGroup\|ContextMenuRadioItem\|ContextMenuSeparator\b\|ContextMenuShortcut\b" web/src --include='*.tsx' --include='*.ts'`
Expected: no output.

- [ ] **Step 3: Full typecheck**

Run (from `web/`): `/Users/char2cs/.bun/bin/bun tsc --noEmit`
Expected: no errors.

- [ ] **Step 4: Full test suite, once**

Run (from `web/`): `/Users/char2cs/.bun/bin/bun vitest run`
Expected: PASS, no unrelated failures. (Per project convention, this full-suite run happens once here — not after every task.)

- [ ] **Step 5: Lint**

Run (from `web/`): `/Users/char2cs/.bun/bin/bun run lint` (check `web/package.json` `scripts` for the exact lint command name if this doesn't match)
Expected: no errors in files touched by this plan.

No commit for this task — it's verification-only. If Step 1 or 4 finds a real regression, fix it as part of the task whose file caused it (Task 2, 3, or 4), amend that task's commit scope with a new commit there, then re-run this task's steps.

---

### Task 6: Manual verification in `make dev-desktop` + browser fallback

**Files:** none modified (unless a live bug is found, in which case fix it in the relevant task's file and note the fix here).

- [ ] **Step 1: Launch the real desktop app**

Run (from repo root, foreground or background as convenient): `make dev-desktop`
Wait for the Tauri window to open with a live workspace loaded.

- [ ] **Step 2: Tab bar — native menu**

Right-click an open tab. Expected: a real OS-drawn context menu appears (not the app's rounded-corner in-app style) with Pin/Unpin, (Rename, if a terminal tab), a separator, Split Right/Split Down (if applicable), Copy Path, Copy Relative Path, Reveal in Finder, Open in Terminal, Reload, a separator, Close, Close Others, Close to Right, Close All. Click "Copy Path" — expected: no error, path lands on the clipboard (paste somewhere to confirm).

- [ ] **Step 3: File explorer — native menu**

Right-click a file and a folder in the file explorer. Expected: native OS menu, correct items per the locked/unlocked and file/dir branches (see spec's per-surface notes). Click "New File" on a folder — expected: inline rename input appears in the tree, no crash.

- [ ] **Step 4: Chats tree — native menu**

Right-click a chat row in the agent chats tree. Expected: native OS menu with "New thread" / "Group into a folder" / "Delete chat" (labels per `chat-menu-model.ts`). Click "New thread" — expected: a new chat is created under the right-clicked one.

- [ ] **Step 5: Workspace/sidebar tree — native menu**

Right-click a row in the workspace sidebar tree. Expected: native OS menu with Group/Lock/Unlock/Remove per selection state.

- [ ] **Step 6: Editor block menu — native menu with a working submenu**

Open a markdown/chat document with at least two paragraph blocks. Right-click a block (not touching the text caret's own native selection area). Expected: native OS menu with Delete, Duplicate, "Turn into ▸" (hovering opens a native submenu: Paragraph/Heading 1/Heading 2/Heading 3/Blockquote), Indent, Outdent, "Align ▸" (Left/Center/Right). Click "Turn into" → "Heading 1" — expected: the block becomes an H1. Right-click the editor's own text caret area (not a block) — expected: no custom menu opens (the `data-slate-editor="true"` guard).

- [ ] **Step 7: Plain-browser fallback**

Stop `make dev-desktop`. Run (from `web/`): `/Users/char2cs/.bun/bin/bun run dev`, open the printed `localhost` URL in a normal browser tab. Right-click a tab, a file-explorer row, a chat row, a sidebar row, and an editor block. Expected: the OLD in-app rounded popup renders for every one of them (native menus are unavailable outside Tauri) — including a working hover submenu for "Turn into" / "Align" on the block menu (Task 3's fallback submenu support).

- [ ] **Step 8: Record the result**

If every check in Steps 2-7 passed, this task is done — no commit needed (nothing changed). If a live bug was found and fixed, commit that fix against the task whose file it belongs to (Task 2/3/4), then re-run the specific step here that had failed to confirm the fix.
